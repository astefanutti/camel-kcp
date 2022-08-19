/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	// +kubebuilder:scaffold:imports

	apisv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"

	logutil "github.com/apache/camel-k/pkg/util/log"
)

var logger = logutil.Log.WithName("kcp")

var options struct {
	// The name of the APIExport
	apiExportName string
	// The port of the metrics endpoint
	metricsPort int
	// The port of the health probe endpoint
	healthProbePort int
	// Enable leader election
	enableLeaderElection bool
	// Leader election id
	leaderElectionID string
}

func init() {
	flagSet := flag.CommandLine

	flagSet.StringVar(&options.apiExportName, "api-export-name", "", "The name of the APIExport.")
	flagSet.IntVar(&options.metricsPort, "metrics-port", 8080, "The port of the metrics endpoint.")
	flagSet.IntVar(&options.healthProbePort, "health-probe-port", 8081, "The port of the health probe endpoint.")
	flagSet.BoolVar(&options.enableLeaderElection, "leader-election", false, "Enable leader election.")
	flagSet.StringVar(&options.leaderElectionID, "leader-election-id", "", "Use the given ID as the leader election Lease name")

	opts := ctrlzap.Options{
		EncoderConfigOptions: []ctrlzap.EncoderConfigOption{
			func(c *zapcore.EncoderConfig) {
				c.ConsoleSeparator = " "
			},
		},
		ZapOpts: []zap.Option{
			zap.AddCaller(),
		},
	}
	opts.BindFlags(flag.CommandLine)
	klog.InitFlags(flag.CommandLine)
	flag.Parse()

	log.SetLogger(ctrlzap.New(ctrlzap.UseFlagOptions(&opts)))
	klog.SetLogger(logger.AsLogger())
}

func main() {
	ctx := ctrl.SetupSignalHandler()
	cfg := ctrl.GetConfigOrDie()
	newManager := manager.New

	if kcpAPIsGroupPresent(cfg) {
		logger.Info("Looking up virtual workspace URL")
		apiExportCfg, err := restConfigForAPIExport(ctx, cfg, options.apiExportName)
		if err != nil {
			logger.Error(err, "error looking up virtual workspace URL")
		}

		logger.Info("Using virtual workspace URL", "url", cfg.Host)
		newManager = kcp.NewClusterAwareManager
		cfg = apiExportCfg
	} else {
		logger.Info("The apis.kcp.dev group is not present - creating standard manager")
	}

	// TODO: reuse upstream operator run function
	Run(ctx, cfg, newManager, int32(options.healthProbePort), int32(options.metricsPort), options.enableLeaderElection, "")
}

// +kubebuilder:rbac:groups="apis.kcp.dev",resources=apiexports,verbs=get;list;watch

// restConfigForAPIExport returns a *rest.Config properly configured to communicate with the endpoint for the
// APIExport's virtual workspace.
func restConfigForAPIExport(ctx context.Context, cfg *rest.Config, apiExportName string) (*rest.Config, error) {
	scheme := runtime.NewScheme()
	if err := apisv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("error adding apis.kcp.dev/v1alpha1 to scheme: %w", err)
	}

	apiExportClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("error creating APIExport client: %w", err)
	}

	var apiExport apisv1alpha1.APIExport

	if apiExportName != "" {
		if err := apiExportClient.Get(ctx, types.NamespacedName{Name: apiExportName}, &apiExport); err != nil {
			return nil, fmt.Errorf("error getting APIExport %q: %w", apiExportName, err)
		}
	} else {
		logger.Info("api-export-name is empty - listing")
		exports := &apisv1alpha1.APIExportList{}
		if err := apiExportClient.List(ctx, exports); err != nil {
			return nil, fmt.Errorf("error listing APIExports: %w", err)
		}
		if len(exports.Items) == 0 {
			return nil, fmt.Errorf("no APIExport found")
		}
		if len(exports.Items) > 1 {
			return nil, fmt.Errorf("more than one APIExport found")
		}
		apiExport = exports.Items[0]
	}

	if len(apiExport.Status.VirtualWorkspaces) < 1 {
		return nil, fmt.Errorf("APIExport %q status.virtualWorkspaces is empty", apiExportName)
	}

	cfg = rest.CopyConfig(cfg)
	// TODO: sharding support
	cfg.Host = apiExport.Status.VirtualWorkspaces[0].URL

	return cfg, nil
}

func kcpAPIsGroupPresent(restConfig *rest.Config) bool {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		logger.Error(err, "failed to create discovery client")
		os.Exit(1)
	}
	apiGroupList, err := discoveryClient.ServerGroups()
	if err != nil {
		logger.Error(err, "failed to get server groups")
		os.Exit(1)
	}

	for _, group := range apiGroupList.Groups {
		if group.Name == apisv1alpha1.SchemeGroupVersion.Group {
			for _, version := range group.Versions {
				if version.Version == apisv1alpha1.SchemeGroupVersion.Version {
					return true
				}
			}
		}
	}
	return false
}
