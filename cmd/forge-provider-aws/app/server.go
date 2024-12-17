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

package app

import (
	"flag"
	"fmt"

	"github.com/go-logr/zapr"
	"github.com/spf13/cobra"
	"go.uber.org/zap"

	infrastructurev1alpha1 "github.com/forge-build/forge-provider-aws/pkg/api/v1alpha1"
	buildv1 "github.com/forge-build/forge/api/v1alpha1"

	"github.com/forge-build/forge-provider-aws/cmd/forge-provider-aws/app/options"
	forgelog "github.com/forge-build/forge-provider-aws/pkg/log"

	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

const (
	controllerName = "forge-provider-aws"
)

func NewControllerManagerCommand() *cobra.Command {
	opts := &options.ControllerManagerRunOptions{}

	// Create a FlagSet and add your flags to it
	fs := flag.NewFlagSet(controllerName, flag.ExitOnError)
	opts.AddFlags(fs)

	// Create a Cobra command
	cmd := &cobra.Command{
		Use:   controllerName,
		Short: "Controller manager for Forge AWS Provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse the flags from the FlagSet
			fs.Parse(args)
			return runControllerManager(opts)
		},
	}

	// Add the FlagSet to the Cobra command
	cmd.Flags().AddGoFlagSet(fs)

	return cmd
}

func runControllerManager(opts *options.ControllerManagerRunOptions) error {
	// Initialize logger
	rawLog := forgelog.New(opts.Debug, opts.LogFormat)
	log := rawLog.Sugar()
	log = log.With("controller-name", controllerName)
	ctrlruntimelog.SetLogger(zapr.NewLogger(rawLog.WithOptions(zap.AddCallerSkip(1))))

	// Setting up kubernetes Configuration
	cfg, err := ctrlruntime.GetConfig()
	if err != nil {
		log.Fatalw("Failed to get kubeconfig", zap.Error(err))
	}
	electionName := controllerName
	if opts.WorkerName != "" {
		electionName += "-" + opts.WorkerName
	}

	// Create a new Manager
	mgr, err := manager.New(cfg, manager.Options{
		Metrics:          metricsserver.Options{BindAddress: opts.MetricsBindAddress},
		LeaderElection:   opts.EnableLeaderElection,
		LeaderElectionID: electionName,
	})
	if err != nil {
		log.Fatalw("Failed to create the manager", zap.Error(err))
	}

	if err := infrastructurev1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Fatalw("Failed to register scheme", zap.Stringer("api", infrastructurev1alpha1.GroupVersion), zap.Error(err))
	}

	if err := buildv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Fatalw("Failed to register scheme", zap.Stringer("api", buildv1.GroupVersion), zap.Error(err))
	}
	rootCtx := signals.SetupSignalHandler()

	ctrlCtx := &options.ControllerContext{
		Ctx:        rootCtx,
		RunOptions: opts,
		Mgr:        mgr,
		Log:        log,
	}
	if err := createAllControllers(ctrlCtx); err != nil {
		log.Fatalw("Could not create all controllers", zap.Error(err))
	}

	log.Info(fmt.Sprintf("Starting the %s Controller Manager", controllerName))
	if err := mgr.Start(rootCtx); err != nil {
		log.Fatalw("problem running manager", zap.Error(err))
	}
	return nil
}
