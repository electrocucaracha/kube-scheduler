/*
Copyright 2019
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

// Package app implements a Server object for running the scheduler.
package app

import (
	"context"
	"fmt"
	"os"

	schedulerserverconfig "github.com/electrocucaracha/kube-scheduler/cmd/app/config"
	"github.com/electrocucaracha/kube-scheduler/cmd/app/options"
	"github.com/electrocucaracha/kube-scheduler/pkg/scheduler"

	"github.com/spf13/cobra"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/component-base/version"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog"
	utilflag "k8s.io/kubernetes/pkg/util/flag"
)

// NewSchedulerCommand creates a *cobra.Command object with default parameters
func NewSchedulerCommand() *cobra.Command {
	opts, err := options.NewOptions()
	if err != nil {
		klog.Fatalf("unable to initialize command options: %v", err)
	}

	cmd := &cobra.Command{
		Use: "scheduler",
		Long: `Custom scheduler is created to demostrated how to
create an additional scheduler for kubernetes`,
		Run: func(cmd *cobra.Command, args []string) {
			if err := runCommand(cmd, args, opts); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
		},
	}

	usageFmt := "Usage:\n  %s\n"

	cmd.SetUsageFunc(func(cmd *cobra.Command) error {
		fmt.Fprintf(cmd.OutOrStderr(), usageFmt, cmd.UseLine())
		return nil
	})
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n"+usageFmt, cmd.Long, cmd.UseLine())
	})
	cmd.MarkFlagFilename("config", "yaml", "yml", "json")

	return cmd
}

// runCommand runs the scheduler.
func runCommand(cmd *cobra.Command, args []string, opts *options.Options) error {
	verflag.PrintAndExitIfRequested()
	utilflag.PrintFlags(cmd.Flags())

	if errs := opts.Validate(); len(errs) > 0 {
		fmt.Fprintf(os.Stderr, "%v\n", utilerrors.NewAggregate(errs))
		os.Exit(1)
	}

	c, err := opts.Config()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}

	stopCh := make(chan struct{})

	return Run(*c, stopCh)
}

// Run executes the scheduler based on the given configuration. It only return on error or when stopCh is closed.
func Run(cc schedulerserverconfig.Config, stopCh <-chan struct{}) error {
	// To help debugging, immediately log version
	klog.V(1).Infof("Starting Kubernetes Scheduler version %+v", version.Get())

	sched, err := scheduler.New(cc.Client,
		cc.ComponentConfig.AlgorithmSource,
		stopCh)
	if err != nil {
		return err
	}

	// Prepare a reusable runCommand function.
	run := func(ctx context.Context) {
		sched.Run()
		<-ctx.Done()
	}

	ctx, cancel := context.WithCancel(context.TODO()) // TODO once Run() accepts a context, it should be used here
	defer cancel()

	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	// Leader election is disabled, so runCommand inline until done.
	run(ctx)

	return fmt.Errorf("finished without leader elect")
}
