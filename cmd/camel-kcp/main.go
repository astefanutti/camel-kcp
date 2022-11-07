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
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	goruntime "runtime"
	"strconv"
	"time"

	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
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
	"github.com/apache/camel-k/pkg/event"
	"github.com/apache/camel-k/pkg/platform"
	"github.com/apache/camel-k/pkg/util/defaults"
	logutil "github.com/apache/camel-k/pkg/util/log"

	"github.com/apache/camel-kcp/pkg/controller/apibinding"
)

var scheme = runtime.NewScheme()

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

func printVersion() {
	logger.Info(fmt.Sprintf("Go Version: %s", goruntime.Version()))
	logger.Info(fmt.Sprintf("Go OS/Arch: %s/%s", goruntime.GOOS, goruntime.GOARCH))
	logger.Info(fmt.Sprintf("Buildah Version: %v", defaults.BuildahVersion))
	logger.Info(fmt.Sprintf("Kaniko Version: %v", defaults.KanikoVersion))
	logger.Info(fmt.Sprintf("Camel K Operator Version: %v", defaults.Version))
	logger.Info(fmt.Sprintf("Camel K Default Runtime Version: %v", defaults.DefaultRuntimeVersion))
	logger.Info(fmt.Sprintf("Camel K Git Commit: %v", defaults.GitCommit))
}

func main() {
	printVersion()

	rand.Seed(time.Now().UTC().UnixNano())

	_, err := maxprocs.Set(maxprocs.Logger(func(f string, a ...interface{}) { logger.Info(fmt.Sprintf(f, a)) }))
	exitOnError(err, "failed to set GOMAXPROCS from cgroups")

	ctx := ctrl.SetupSignalHandler()
	cfg := ctrl.GetConfigOrDie()

	// Register types to scheme
	exitOnError(clientgoscheme.AddToScheme(scheme), "failed registering types to scheme")
	exitOnError(apis.AddToScheme(scheme), "failed registering types to scheme")
	exitOnError(apisv1alpha1.AddToScheme(scheme), "failed registering types to scheme")

	// Clients
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	exitOnError(err, "failed to create discovery client")

	if !kcpAPIsGroupPresent(discoveryClient) {
		exitOnError(errors.New("apis.kcp.dev group is not present"), "")
	}

	logger.Info("Looking up virtual workspace URL")
	apiExportCfg, err := restConfigForAPIExport(ctx, cfg, options.apiExportName)
	exitOnError(err, "error looking up virtual workspace URL")

	logger.Info("Using virtual workspace URL", "url", apiExportCfg.Host)

	// Cache options
	hasIntegrationLabel, err := labels.NewRequirement(v1.IntegrationLabel, selection.Exists, []string{})
	exitOnError(err, "cannot create Integration label selector")
	selector := labels.NewSelector().Add(*hasIntegrationLabel)
	selectors := cache.SelectorsByObject{
		&corev1.Pod{}:        {Label: selector},
		&appsv1.Deployment{}: {Label: selector},
		&batchv1.Job{}:       {Label: selector},
		&servingv1.Service{}: {Label: selector},
		&batchv1.CronJob{}:   {Label: selector},
	}

	// FIXME: cluster-aware event sink
	broadcaster := record.NewBroadcaster()
	defer broadcaster.Shutdown()
	broadcaster = event.NewSinkLessBroadcaster(broadcaster)

	// Manager options
	mgrOptions := ctrl.Options{
		LeaderElection:                options.enableLeaderElection,
		LeaderElectionID:              options.leaderElectionID,
		LeaderElectionConfig:          cfg,
		LeaderElectionResourceLock:    resourcelock.LeasesResourceLock,
		LeaderElectionReleaseOnCancel: true,
		HealthProbeBindAddress:        ":" + strconv.Itoa(options.healthProbePort),
		MetricsBindAddress:            ":" + strconv.Itoa(options.metricsPort),
		Scheme:                        scheme,
		EventBroadcaster:              broadcaster,
		NewCache: func(config *rest.Config, options cache.Options) (cache.Cache, error) {
			options.SelectorsByObject = selectors
			return kcp.NewClusterAwareCache(config, options)
		},
	}

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

	mgr, err := kcp.NewClusterAwareManager(apiExportCfg, mgrOptions)
	exitOnError(err, "")

	// Probes and controllers
	logger.Info("Configuring the manager")
	exitOnError(mgr.AddHealthzCheck("healthz", healthz.Ping), "Unable to add health check")
	exitOnError(mgr.AddReadyzCheck("readyz", healthz.Ping), "Unable to add ready check")

	c, err := NewClient(apiExportCfg, scheme, mgr.GetClient())
	exitOnError(err, "failed to create client")

	exitOnError(controller.AddToManager(mgr, c), "")

	// FIXME: workspace initializer
	// logger.Info("Installing operator resources")
	// installCtx, installCancel := context.WithTimeout(ctx, 1*time.Minute)
	// defer installCancel()
	// install.OperatorStartupOptionalTools(installCtx, c, "", operatorNamespace, logger)
	// exitOnError(findOrCreateIntegrationPlatform(installCtx, c, operatorNamespace), "failed to create integration platform")
	exitOnError(apibinding.Add(mgr, c), "")

	logger.Info("Starting the manager")
	exitOnError(mgr.Start(ctx), "manager exited non-zero")
}

// +kubebuilder:rbac:groups="apis.kcp.dev",resources=apiexports,verbs=get;list;watch
// +kubebuilder:rbac:groups="apis.kcp.dev",resources=apiexports/content,verbs=get;list;watch;create;update;patch;delete

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
	exitOnError(err, "failed to get server groups")

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

// getOperatorImage returns the image currently used by the running operator if present (when running out of cluster, it may be absent).
// nolint: unused
func getOperatorImage(ctx context.Context, c client.Reader) (string, error) {
	ns := platform.GetOperatorNamespace()
	name := platform.GetOperatorPodName()
	if ns == "" || name == "" {
		return "", nil
	}

	pod := corev1.Pod{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &pod); err != nil && apierrors.IsNotFound(err) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	if len(pod.Spec.Containers) == 0 {
		return "", fmt.Errorf("no containers found in operator pod")
	}
	return pod.Spec.Containers[0].Image, nil
}

func exitOnError(err error, msg string) {
	if err != nil {
		logger.Error(err, msg)
		os.Exit(1)
	}
}
