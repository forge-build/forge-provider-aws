/*
Copyright 2024 The Forge contributors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package awsbuild

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/forge-build/forge-provider-aws/pkg/cloud"
	"github.com/forge-build/forge-provider-aws/pkg/cloud/scope"
	awserrors "github.com/forge-build/forge-provider-aws/pkg/cloud/services/errors"
	"github.com/forge-build/forge-provider-aws/pkg/cloud/services/images"
	"github.com/forge-build/forge-provider-aws/pkg/cloud/services/instances"
	"github.com/forge-build/forge-provider-aws/pkg/cloud/services/networks"
	"github.com/forge-build/forge-provider-aws/pkg/cloud/services/securitygroup"
	"github.com/forge-build/forge-provider-aws/pkg/cloud/services/subnet"
	buildv1 "github.com/forge-build/forge/api/v1alpha1"
	"github.com/forge-build/forge/pkg/ssh"
	forgeutil "github.com/forge-build/forge/util"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/forge-build/forge/util/annotations"
	"github.com/forge-build/forge/util/predicates"
	"github.com/pkg/errors"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"

	infrav1 "github.com/forge-build/forge-provider-aws/pkg/api/v1alpha1"
)

const ControllerName = "awsbuild-controller"

// AWSBuildReconciler reconciles a AWSBuild object
type AWSBuildReconciler struct {
	client.Client
	log      logr.Logger
	recorder record.EventRecorder
}

// Add creates a new AWSBuild controller and adds it to the Manager.
func Add(ctx context.Context, mgr ctrl.Manager, numWorkers int, log *zap.SugaredLogger) error {
	// Create the reconciler instance
	reconciler := &AWSBuildReconciler{
		Client:   mgr.GetClient(),
		recorder: mgr.GetEventRecorderFor(ControllerName),
	}

	// Set up the controller with custom predicates
	_, err := builder.ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: numWorkers,
		}).
		For(&infrav1.AWSBuild{}).
		WithEventFilter(predicates.ResourceNotPaused(ctrl.LoggerFrom(ctx))).
		Watches(&buildv1.Build{},
			handler.EnqueueRequestsFromMapFunc(forgeutil.BuildToInfrastructureMapFunc(ctx, infrav1.GroupVersion.WithKind(infrav1.AWSBuildKind), mgr.GetClient(), &infrav1.AWSBuild{})),
			builder.WithPredicates(predicates.BuildUnpaused(ctrl.LoggerFrom(ctx)))).
		Build(reconciler)

	return err
}

// +kubebuilder:rbac:groups=infrastructure.forge.build,resources=awsbuilds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.forge.build,resources=awsbuilds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.forge.build,resources=awsbuilds/finalizers,verbs=update
// +kubebuilder:rbac:groups=forge.build,resources=builds,verbs=get;list;watch;patch

func (r *AWSBuildReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	r.log = log.FromContext(ctx)
	r.log.Info("Reconciling")
	awsBuild := &infrav1.AWSBuild{}
	err := r.Get(ctx, req.NamespacedName, awsBuild)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("awsBuild resource not found or already deleted")
			return ctrl.Result{}, nil
		}

		r.log.Error(err, "Unable to fetch awsBuild resource")
		return ctrl.Result{}, err
	}

	// Fetch the Build.
	build, err := forgeutil.GetOwnerBuild(ctx, r.Client, awsBuild.ObjectMeta)
	if err != nil {
		r.log.Error(err, "Failed to get owner build")
		return ctrl.Result{}, err
	}
	if build == nil {
		r.log.Info("Build Controller has not yet set OwnerRef")
		return ctrl.Result{}, nil
	}

	if annotations.IsPaused(build, awsBuild) {
		r.log.Info("awsBuild of linked Build is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	buildScope, err := scope.NewAWSBuildScope(ctx, scope.AWSBuildScopeParams{
		Client:   r.Client,
		Build:    build,
		AWSBuild: awsBuild,
	})
	if err != nil {
		return ctrl.Result{}, errors.Errorf("failed to create scope: %+v", err)
	}

	// Always close the scope when exiting this function so we can persist any awsBuild changes.
	defer func() {
		if err := buildScope.Close(); err != nil && reterr == nil {
			reterr = err
		}
	}()

	// Handle deleted clusters
	if !awsBuild.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(awsBuild, infrav1.BuildFinalizer) {
		return r.reconcileDelete(ctx, buildScope)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, buildScope)
}

func (r *AWSBuildReconciler) recordEvent(awsBuild *infrav1.AWSBuild, eventType, reason, message string) {
	r.log.Info(message)
	r.recorder.Event(awsBuild, eventType, reason, message)
}

func (r *AWSBuildReconciler) reconcileDelete(ctx context.Context, buildScope *scope.AWSBuildScope) (ctrl.Result, error) {
	r.log.Info("Reconciling Delete AWSBuild")

	reconcilers := []cloud.Reconciler{
		instances.New(buildScope),
		securitygroup.New(buildScope),
		subnet.New(buildScope),
		networks.New(buildScope),
	}

	for _, reconcile := range reconcilers {
		if err := reconcile.Delete(ctx); err != nil {
			if awserrors.IsInstanceNotTerminated(err) {
				r.log.Info("Instance is not terminated yet")
				return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
			}
			r.log.Error(err, "Reconcile error")
			r.recordEvent(buildScope.AWSBuild, "Warning", "Cleaning Up Failed", fmt.Sprintf("Reconcile error - %v ", err))
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(buildScope.AWSBuild, infrav1.BuildFinalizer)
	r.recordEvent(buildScope.AWSBuild, "Normal", "Reconciled", fmt.Sprintf("%s is reconciled successfully ", buildScope.Name()))
	buildScope.SetCleanedUP()
	return ctrl.Result{}, nil
}

func (r *AWSBuildReconciler) reconcileNormal(ctx context.Context, buildScope *scope.AWSBuildScope) (ctrl.Result, error) {
	reconcilers := []cloud.Reconciler{
		networks.New(buildScope),
		subnet.New(buildScope),
		securitygroup.New(buildScope),
		instances.New(buildScope),
		images.New(buildScope),
	}

	// get ssh key
	sshKey, err := r.GetSSHKey(ctx, buildScope)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "unable to get an ssh-key")
	}
	buildScope.SetSSHKey(sshKey)

	if !buildScope.IsReady() {
		for _, reconciler := range reconcilers {
			if err := reconciler.Reconcile(ctx); err != nil {
				r.log.Error(err, "Reconcile error")
				r.recordEvent(buildScope.AWSBuild, "Warning", "Building Failed", fmt.Sprintf("Reconcile error - %v ", err))
				return ctrl.Result{}, err
			}
		}
		controllerutil.AddFinalizer(buildScope.AWSBuild, infrav1.BuildFinalizer)
		if err := buildScope.PatchObject(); err != nil {
			return ctrl.Result{}, err
		}
	}

	r.log.Info("Reconciling AWSBuild")

	if buildScope.IsReady() && !buildScope.IsCleanedUP() {
		return r.reconcileDelete(ctx, buildScope)
	}

	if buildScope.GetInstanceID() == nil {
		r.recordEvent(buildScope.AWSBuild, "Normal", "AWSBuildReconciler", "AWSBuild does not started the build yet ")

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	r.recordEvent(buildScope.AWSBuild, "Normal", "InstanceCreated", fmt.Sprintf("Machine is created, Got an instance ID - %s ", *buildScope.GetInstanceID()))

	buildScope.SetMachineReady()

	if buildScope.AWSBuild.Status.ArtifactRef == nil {
		r.recordEvent(buildScope.AWSBuild, "Normal", "WaitBuilding", "Artifact is not available yet ")

		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	r.recordEvent(buildScope.AWSBuild, "Normal", "ImageReady", fmt.Sprintf("Got an available Artifact - %s ", *buildScope.AWSBuild.Status.ArtifactRef))

	buildScope.SetReady()
	r.recordEvent(buildScope.AWSBuild, "Normal", "Reconciled", "AWS Build is reconciled successfully ")

	return ctrl.Result{}, nil
}

func (r *AWSBuildReconciler) GetSSHKey(ctx context.Context, buildScope *scope.AWSBuildScope) (key scope.SSHKey, err error) {
	if buildScope.AWSBuild.Spec.SSHCredentialsRef != nil {
		secret, err := forgeutil.GetSecretFromSecretReference(ctx, r.Client, *buildScope.AWSBuild.Spec.SSHCredentialsRef)
		if err != nil {
			return scope.SSHKey{}, errors.Wrap(err, "unable to get ssh credentials secret")
		}

		_, _, privKey, pubKey := ssh.GetCredentialsFromSecret(secret)

		return scopeSSHKey(buildScope.AWSBuild.Spec.Username, privKey, pubKey), nil
	}

	if buildScope.AWSBuild.Spec.GenerateSSHKey {
		sshKey, err := ssh.NewKeyPair()
		if err != nil {
			return key, errors.Wrap(err, "cannot generate ssh key")
		}

		err = forgeutil.EnsureCredentialsSecret(ctx, r.Client, buildScope.Build, forgeutil.SSHCredentials{
			Username:   buildScope.AWSBuild.Spec.Username,
			PrivateKey: string(sshKey.PrivateKey),
			PublicKey:  string(sshKey.PublicKey),
		}, "aws")
		if err != nil {
			return scope.SSHKey{}, err
		}
		// Update the CredentialsRef in the AWSBuild spec
		buildScope.AWSBuild.Spec.SSHCredentialsRef = &corev1.SecretReference{
			Name:      fmt.Sprintf("%s-ssh-credentials", buildScope.Build.Name),
			Namespace: buildScope.Build.Namespace,
		}

		return scopeSSHKey(buildScope.AWSBuild.Spec.Username, string(sshKey.PrivateKey), string(sshKey.PublicKey)), nil
	}

	return scope.SSHKey{}, errors.New("no ssh key provided, consider using spec.generateSSHKey or provide a private key")
}

func scopeSSHKey(username, privateKey, pubKey string) scope.SSHKey {
	sshPublicKey := strings.TrimSuffix(pubKey, "\n")

	return scope.SSHKey{
		MetadataSSHKeys: fmt.Sprintf("%s:%s %s", username, sshPublicKey, username),
		PrivateKey:      privateKey,
		PublicKey:       pubKey,
	}
}
