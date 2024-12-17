package main

import (
	"context"
	"fmt"
	"log"
	"time"

	buildv1alpha1 "github.com/forge-build/forge/api/v1alpha1"
	"github.com/forge-build/forge/util"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

func main() {
	// Create a controller-runtime client
	ctx := context.Background()
	k8sClient, err := getKubernetesClient()
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}
	example := createBuildObject()
	err = util.EnsureCredentialsSecret(ctx, k8sClient, example, util.SSHCredentials{
		Username:   "xxxxxxxxxxxxxxxxxxxxxxxxxx",
		PrivateKey: "mohamed",
		PublicKey:  "mohamed",
		Host:       "ddddd",
	}, "aws")
	if err != nil {
		log.Fatalf("Failed to create secret: %v", err)
	}
}

// getKubernetesClient initializes and returns a controller-runtime client
func getKubernetesClient() (client.Client, error) {
	// Load the Kubernetes configuration
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// Create a new controller-runtime client
	k8sClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return k8sClient, nil
}

// createSecret creates a Kubernetes Secret object
func createSecret(k8sClient client.Client, namespace, secretName string, data map[string][]byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Define the secret object
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	// Create the secret in the cluster
	err := k8sClient.Create(ctx, secret)
	if err != nil {
		return fmt.Errorf("failed to create secret: %w", err)
	}

	return nil
}

func createBuildObject() *buildv1alpha1.Build {
	return &buildv1alpha1.Build{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Build",
			APIVersion: "infrastructure.forge.build/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-build",
			Namespace: "default",
		},
		Spec: buildv1alpha1.BuildSpec{
			Paused: false,
			Connector: buildv1alpha1.ConnectorSpec{
				Type: "ssh",
				Credentials: &corev1.LocalObjectReference{
					Name: "ssh-credentials",
				},
			},
			InfrastructureRef: &corev1.ObjectReference{
				Kind:       "AWSBuild",
				APIVersion: "infrastructure.forge.build/v1alpha1",
				Name:       "my-aws-build",
			},
		},
		Status: buildv1alpha1.BuildStatus{
			InfrastructureReady: true,
			Connected:           true,
			ProvisionersReady:   false,
			Phase:               string(buildv1alpha1.BuildPhaseBuilding),
			Ready:               false,
		},
	}
}
