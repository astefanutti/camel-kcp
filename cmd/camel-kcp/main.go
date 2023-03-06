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
	goruntime "runtime"
	"time"

	"go.uber.org/automaxprocs/maxprocs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"golang.org/x/sync/errgroup"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"
	retrywatch "k8s.io/client-go/tools/watch"
	configv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/klog/v2"
	"k8s.io/utils/pointer"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlcfg "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/kcp"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apisv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"
	"github.com/kcp-dev/kcp/pkg/apis/third_party/conditions/util/conditions"

	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/apache/camel-k/pkg/apis"
	v1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	camelk "github.com/apache/camel-k/pkg/controller"
	"github.com/apache/camel-k/pkg/event"
	"github.com/apache/camel-k/pkg/util/defaults"
	logutil "github.com/apache/camel-k/pkg/util/log"

	"github.com/apache/camel-kcp/pkg/client"
	"github.com/apache/camel-kcp/pkg/config"
	"github.com/apache/camel-kcp/pkg/controller"
	"github.com/apache/camel-kcp/pkg/platform"
)

var scheme = runtime.NewScheme()

var logger = logutil.Log.WithName("kcp")

var options struct {
	// The path of the configuration file
	configFilePath string
}

func init() {
	flagSet := flag.CommandLine

	flag.StringVar(&options.configFilePath, "config", "",
		"The controller will load its initial configuration from this file. "+
			"Omit this flag to use the default configuration values. "+
			"Command-line flags override configuration from this file.")

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
	opts.BindFlags(flagSet)
	klog.InitFlags(flagSet)
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

	ctx := ctrl.SetupSignalHandler()
	cfg := ctrl.GetConfigOrDie()

	// Scheme
	exitOnError(clientgoscheme.AddToScheme(scheme), "failed registering types to scheme")
	exitOnError(apis.AddToScheme(scheme), "failed registering types to scheme")
	exitOnError(apisv1alpha1.AddToScheme(scheme), "failed registering types to scheme")

	// Configuration
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
	broadcaster = event.NewSinkLessBroadcaster(broadcaster)

	// FIXME: enable leader election
	mgrOptions := ctrl.Options{
		LeaderElectionConfig:          cfg,
		LeaderElectionReleaseOnCancel: true,
		Scheme:                        scheme,
		EventBroadcaster:              broadcaster,
		NewCache: func(config *rest.Config, options cache.Options) (cache.Cache, error) {
			options.SelectorsByObject = selectors
			return kcp.NewClusterAwareCache(config, options)
		},
	}

	svcCfg := &config.ServiceConfiguration{
		ControllerManagerConfigurationSpec: ctrlcfg.ControllerManagerConfigurationSpec{
			Health: ctrlcfg.ControllerHealth{
				HealthProbeBindAddress: ":8081",
			},
			Metrics: ctrlcfg.ControllerMetrics{
				BindAddress: ":8080",
			},
			LeaderElection: &configv1alpha1.LeaderElectionConfiguration{
				LeaderElect:  pointer.Bool(false),
				ResourceLock: resourcelock.LeasesResourceLock,
			},
		},
		Service: config.ServiceConfigurationSpec{
			APIExports: config.APIExports{
				CamelK: config.CamelKAPIExport{
					LocalAPIExportReference: config.LocalAPIExportReference{
						APIExportName: "camel-k",
					},
				},
				Kaoto: config.KaotoAPIExport{
					LocalAPIExportReference: config.LocalAPIExportReference{
						APIExportName: "kaoto",
					},
				},
			},
		},
	}

	if options.configFilePath != "" {
		mgrOptions, err = mgrOptions.AndFrom(ctrl.ConfigFile().AtPath(options.configFilePath).OfKind(svcCfg))
		exitOnError(err, "error loading controller configuration")
	}

	// Environment
	_, err = maxprocs.Set(maxprocs.Logger(func(f string, a ...interface{}) { logger.Info(fmt.Sprintf(f, a)) }))
	exitOnError(err, "failed to set GOMAXPROCS from cgroups")

	if ip := svcCfg.Service.APIExports.CamelK.OnAPIBinding.DefaultPlatform; ip != nil && ip.Namespace != "" {
		exitOnError(os.Setenv(platform.OperatorNamespaceEnvVariable, ip.Namespace), "")
	} else {
		exitOnError(os.Setenv(platform.OperatorNamespaceEnvVariable, platform.DefaultNamespaceName), "")
	}

	// Bootstrap
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(cfg)
	exitOnError(err, "failed to create discovery client")

	if !kcpAPIsGroupPresent(discoveryClient) {
		exitOnError(fmt.Errorf("%s group is not present", apisv1alpha1.SchemeGroupVersion.Group), "")
	}

	group, groupCtx := errgroup.WithContext(ctx)

	// TODO: revisit if/when controller-runtime supports multiple clusters / clients
	group.Go(startCamelKManager(groupCtx, cfg, svcCfg, mgrOptions))
	group.Go(startKaotoManager(groupCtx, cfg, svcCfg, broadcaster))

	exitOnError(group.Wait(), "managers exited non-zero")
}

func startCamelKManager(ctx context.Context, cfg *rest.Config, svcCfg *config.ServiceConfiguration, mgrOptions manager.Options) func() error {
	return func() error {
		logger.Info("Looking up Camel K virtual workspace URL")
		apiExportCfg, err := restConfigForAPIExport(ctx, cfg, svcCfg.Service.APIExports.CamelK.APIExportName)
		if err != nil {
			return err
		}
		logger.Info("Using Camel K virtual workspace URL", "url", apiExportCfg.Host)

		// Set the operator container image if it runs in-container
		// FIXME: find a way to retrieve the image
		// platform.OperatorImage, err = getOperatorImage(ctx, c)
		// exitOnError(err, "cannot get operator container image")

		logger.Info("Configuring the Camel K manager")
		mgr, err := kcp.NewClusterAwareManager(apiExportCfg, mgrOptions)
		if err != nil {
			return err
		}
		err = mgr.AddHealthzCheck("healthz", healthz.Ping)
		if err != nil {
			return err
		}
		err = mgr.AddReadyzCheck("readyz", healthz.Ping)
		if err != nil {
			return err
		}
		c, err := client.NewClient(apiExportCfg, scheme, mgr.GetClient())
		if err != nil {
			return err
		}
		err = camelk.AddToManager(ctx, mgr, c)
		if err != nil {
			return err
		}
		err = controller.AddCamelKController(mgr, c, svcCfg)
		if err != nil {
			return err
		}
		logger.Info("Starting the Camel K manager")
		return mgr.Start(ctx)
	}
}

func startKaotoManager(ctx context.Context, cfg *rest.Config, svcCfg *config.ServiceConfiguration, broadcaster record.EventBroadcaster) func() error {
	return func() error {
		logger.Info("Looking up Kaoto virtual workspace URL")
		apiExportCfg, err := restConfigForAPIExport(ctx, cfg, svcCfg.Service.APIExports.Kaoto.APIExportName)
		if err != nil {
			return err
		}
		logger.Info("Using Kaoto virtual workspace URL", "url", apiExportCfg.Host)

		logger.Info("Configuring Kaoto the manager")
		mgr, err := kcp.NewClusterAwareManager(apiExportCfg, ctrl.Options{
			LeaderElection:     false,
			MetricsBindAddress: "0",
			Scheme:             scheme,
			EventBroadcaster:   broadcaster,
		})
		if err != nil {
			return err
		}
		c, err := client.NewClient(apiExportCfg, scheme, mgr.GetClient())
		if err != nil {
			return err
		}
		err = controller.AddKaotoController(mgr, c, svcCfg)
		if err != nil {
			return err
		}
		err = controller.AddKaotoIngressController(mgr, c, svcCfg)
		if err != nil {
			return err
		}
		logger.Info("Starting the Kaoto manager")
		return mgr.Start(ctx)
	}
}

// +kubebuilder:rbac:groups="apis.kcp.io",resources=apiexports,verbs=get;list;watch
// +kubebuilder:rbac:groups="apis.kcp.io",resources=apiexports/content,verbs=get;list;watch;create;update;patch;delete

// restConfigForAPIExport returns a *rest.Config properly configured to communicate with the endpoint for the
// APIExport's virtual workspace. It blocks until the controller APIExport VirtualWorkspaceURLsReady condition
// becomes truthy, which happens when the APIExport is bound for the first time.
func restConfigForAPIExport(ctx context.Context, cfg *rest.Config, apiExportName string) (*rest.Config, error) {
	apiExportClient, err := ctrlclient.NewWithWatch(cfg, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("error creating APIExport client: %w", err)
	}

	list := &apisv1alpha1.APIExportList{}
	selector := fields.OneTermEqualSelector("metadata.name", apiExportName)
	err = apiExportClient.List(ctx, list, ctrlclient.MatchingFieldsSelector{Selector: selector})
	if err != nil {
		return nil, fmt.Errorf("error watching for APIExport: %w", err)
	}
	if len(list.Items) > 0 && isAPIExportReady(&list.Items[0]) {
		cfg = rest.CopyConfig(cfg)
		// TODO: sharding support
		//nolint:staticcheck // SA1019 VirtualWorkspaces is deprecated but not removed yet
		cfg.Host = list.Items[0].Status.VirtualWorkspaces[0].URL
		return cfg, nil
	}

	rw, err := retrywatch.NewRetryWatcher(list.ResourceVersion, watcher(apiExportClient.Watch).FilteredBy(selector))
	if err != nil {
		return nil, fmt.Errorf("error creating retry watcher for APIExport: %w", err)
	}
	defer rw.Stop()

	logger.Info("Watching for APIExport to become ready", "name", apiExportName)

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case e := <-rw.ResultChan():
			switch e.Type {
			case watch.Error:
				return nil, fmt.Errorf("error watching for APIExport: %w", apierrors.FromObject(e.Object))

			case watch.Added, watch.Modified:
				apiExport, ok := e.Object.(*apisv1alpha1.APIExport)
				if !ok {
					return nil, fmt.Errorf("unexpected event object: %v", e.Object)
				}
				if !isAPIExportReady(apiExport) {
					continue
				}
				cfg = rest.CopyConfig(cfg)
				// TODO: sharding support
				//nolint:staticcheck // SA1019 VirtualWorkspaces is deprecated but not removed yet
				cfg.Host = apiExport.Status.VirtualWorkspaces[0].URL
				return cfg, nil
			}
		}
	}
}

func isAPIExportReady(apiExport *apisv1alpha1.APIExport) bool {
	if !conditions.IsTrue(apiExport, apisv1alpha1.APIExportVirtualWorkspaceURLsReady) {
		logger.Info("APIExport virtual workspace URLs are not ready", "APIExport", apiExport.Name)
		return false
	}

	//nolint:staticcheck // SA1019 VirtualWorkspaces is deprecated but not removed yet
	if len(apiExport.Status.VirtualWorkspaces) == 0 {
		logger.Info("APIExport does not have any virtual workspace URLs", "APIExport", apiExport.Name)
		return false
	}

	return true
}

func kcpAPIsGroupPresent(discoveryClient discovery.ServerGroupsInterface) bool {
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
func getOperatorImage(ctx context.Context, c ctrlclient.Reader) (string, error) {
	ns := platform.GetOperatorNamespace()
	name := platform.GetOperatorPodName()
	if ns == "" || name == "" {
		return "", nil
	}

	pod := corev1.Pod{}
	if err := c.Get(ctx, ctrlclient.ObjectKey{Namespace: ns, Name: name}, &pod); err != nil && apierrors.IsNotFound(err) {
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

type watcher func(ctx context.Context, obj ctrlclient.ObjectList, opts ...ctrlclient.ListOption) (watch.Interface, error)

func (w watcher) Watch(options metav1.ListOptions) (watch.Interface, error) {
	return w(context.TODO(), &apisv1alpha1.APIExportList{}, &ctrlclient.ListOptions{Raw: &options})
}

func (w watcher) FilteredBy(selector fields.Selector) watcher {
	return func(ctx context.Context, obj ctrlclient.ObjectList, opts ...ctrlclient.ListOption) (watch.Interface, error) {
		return w(ctx, obj, append(opts, ctrlclient.MatchingFieldsSelector{Selector: selector})...)
	}
}
