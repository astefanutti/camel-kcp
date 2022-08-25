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
	"math/rand"
	"os"
	"reflect"
	"strconv"
	"time"

	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	apisv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"

	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/apache/camel-k/pkg/apis"
	v1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	"github.com/apache/camel-k/pkg/controller"
	"github.com/apache/camel-k/pkg/platform"
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
	printVersion()

	rand.Seed(time.Now().UTC().UnixNano())

	_, err := maxprocs.Set(maxprocs.Logger(func(f string, a ...interface{}) { logger.Info(fmt.Sprintf(f, a)) }))
	exitOnError(err, "failed to set GOMAXPROCS from cgroups")

	ctx := ctrl.SetupSignalHandler()
	cfg := ctrl.GetConfigOrDie()

	// Common manager options
	mgrOptions := ctrl.Options{
		LeaderElection:                options.enableLeaderElection,
		LeaderElectionID:              options.leaderElectionID,
		LeaderElectionConfig:          cfg,
		LeaderElectionResourceLock:    resourcelock.LeasesResourceLock,
		LeaderElectionReleaseOnCancel: true,
		HealthProbeBindAddress:        ":" + strconv.Itoa(options.healthProbePort),
		MetricsBindAddress:            ":" + strconv.Itoa(options.metricsPort),
	}

	// Clients
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	exitOnError(err, "failed to create discovery client")

	kcpEnabled := kcpAPIsGroupPresent(discoveryClient)

	var apiExportCfg *rest.Config
	if kcpEnabled {
		logger.Info("Looking up virtual workspace URL")
		apiExportCfg, err = restConfigForAPIExport(ctx, cfg, options.apiExportName)
		exitOnError(err, "error looking up virtual workspace URL")

		logger.Info("Using virtual workspace URL", "url", apiExportCfg.Host)

		discoveryClient, err = discovery.NewDiscoveryClientForConfig(apiExportCfg)
		exitOnError(err, "failed to create discovery client for APIExport virtual workspace")
	} else {
		logger.Info("The apis.kcp.dev group is not present - creating standard manager")
	}

	// Cache options
	hasIntegrationLabel, err := labels.NewRequirement(v1.IntegrationLabel, selection.Exists, []string{})
	exitOnError(err, "cannot create Integration label selector")
	selector := labels.NewSelector().Add(*hasIntegrationLabel)
	selectors := cache.SelectorsByObject{
		&corev1.Pod{}:        {Label: selector},
		&appsv1.Deployment{}: {Label: selector},
		&batchv1.Job{}:       {Label: selector},
		&servingv1.Service{}: {Label: selector},
	}
	if ok, err := isAPIResourceInstalled(discoveryClient, batchv1.SchemeGroupVersion.String(), reflect.TypeOf(batchv1.CronJob{}).Name()); ok && err == nil {
		selectors[&batchv1.CronJob{}] = struct {
			Label labels.Selector
			Field fields.Selector
		}{
			Label: selector,
		}
	}
	if kcpEnabled {
		mgrOptions.NewCache = func(config *rest.Config, opts cache.Options) (cache.Cache, error) {
			return kcp.NewClusterAwareCache(config, cache.Options{
				SelectorsByObject: selectors,
			})
		}
	} else {
		mgrOptions.NewCache = cache.BuilderWithOptions(
			cache.Options{
				SelectorsByObject: selectors,
			},
		)
	}

	// We do not rely on the event broadcaster managed by controller runtime,
	// so that we can check the operator has been granted permission to create
	// Events. This is required for the operator to be installable by standard
	// admin users, that are not granted create permission on Events by default.
	broadcaster := record.NewBroadcaster()
	defer broadcaster.Shutdown()

	// if ok, err := kubernetes.CheckPermission(ctx, c, corev1.GroupName, "events", "", "", "create"); err != nil || !ok {
	// 	// Do not sink Events to the server as they'll be rejected
	// 	broadcaster = event.NewSinkLessBroadcaster(broadcaster)
	// 	exitOnError(err, "cannot check permissions for creating Events")
	// 	logger.Info("Event broadcasting is disabled because of missing permissions to create Events")
	// }

	operatorNamespace := platform.GetOperatorNamespace()
	if operatorNamespace == "" {
		mgrOptions.LeaderElection = false
		logger.Info("unable to determine namespace for leader election")
	}

	// Set the operator container image if it runs in-container
	// FIXME: find a way to retrieve the operator image
	// platform.OperatorImage, err = getOperatorImage(ctx, c)
	// exitOnError(err, "cannot get operator container image")

	if !mgrOptions.LeaderElection {
		logger.Info("Leader election is disabled!")
	}

	var mgr ctrl.Manager
	if kcpEnabled {
		mgr, err = kcp.NewClusterAwareManager(apiExportCfg, mgrOptions)
		exitOnError(err, "")
	} else {
		mgr, err = ctrl.NewManager(cfg, mgrOptions)
		exitOnError(err, "")
	}

	// exitOnError(
	// 	mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, "status.phase",
	// 		func(obj ctrl.Object) []string {
	// 			pod, _ := obj.(*corev1.Pod)
	// 			return []string{string(pod.Status.Phase)}
	// 		}),
	// 	"unable to set up field indexer for status.phase: %v",
	// )

	logger.Info("Configuring manager")
	exitOnError(mgr.AddHealthzCheck("health-probe", healthz.Ping), "Unable add liveness check")
	exitOnError(apis.AddToScheme(mgr.GetScheme()), "")
	exitOnError(controller.AddToManager(mgr), "")

	// FIXME: workspace initializer
	// logger.Info("Installing operator resources")
	// installCtx, installCancel := context.WithTimeout(ctx, 1*time.Minute)
	// defer installCancel()
	// install.OperatorStartupOptionalTools(installCtx, c, "", operatorNamespace, logger)
	// exitOnError(findOrCreateIntegrationPlatform(installCtx, c, operatorNamespace), "failed to create integration platform")

	logger.Info("Starting the manager")
	exitOnError(mgr.Start(ctx), "manager exited non-zero")
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

func kcpAPIsGroupPresent(discoveryClient discovery.DiscoveryInterface) bool {
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
