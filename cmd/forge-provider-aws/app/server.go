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

	"github.com/spf13/cobra"

	infrastructurev1alpha1 "github.com/forge-build/forge-provider-aws/pkg/api/v1alpha1"
	buildv1 "github.com/forge-build/forge/pkg/api/v1alpha1"

	"github.com/forge-build/forge-provider-aws/cmd/forge-provider-aws/app/options"
	forgelog "github.com/forge-build/forge/pkg/log"

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
	logRaw, err := forgelog.NewZapLogger(opts.LogLevel, opts.LogFormat)
	if err != nil {
		return nil
	}
	log := logRaw.WithName(controllerName)
	ctrlruntimelog.SetLogger(log)

	// Setting up kubernetes Configuration
	cfg, err := ctrlruntime.GetConfig()
	if err != nil {
		log.Error(err, "Failed to get kubeconfig")
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
		log.Error(err, "Failed to create the manager")
	}

	if err := infrastructurev1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "Failed to register scheme")
	}

	if err := buildv1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "Failed to register scheme")
	}
	rootCtx := signals.SetupSignalHandler()

	ctrlCtx := &options.ControllerContext{
		Ctx:        rootCtx,
		RunOptions: opts,
		Mgr:        mgr,
		Log:        logRaw,
	}
	if err := createAllControllers(ctrlCtx); err != nil {
		log.Error(err, "Could not create all controllers")
	}

	log.Info(fmt.Sprintf("Starting the %s Controller Manager", controllerName))
	if err := mgr.Start(rootCtx); err != nil {
		log.Error(err, "problem running manager")
	}
	return nil
}
