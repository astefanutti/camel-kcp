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
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"go.uber.org/automaxprocs/maxprocs"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordination "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	servingv1 "knative.dev/serving/pkg/apis/serving/v1"

	"github.com/apache/camel-k/pkg/apis"
	v1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	"github.com/apache/camel-k/pkg/client"
	"github.com/apache/camel-k/pkg/controller"
	"github.com/apache/camel-k/pkg/event"
	"github.com/apache/camel-k/pkg/install"
	"github.com/apache/camel-k/pkg/platform"
	"github.com/apache/camel-k/pkg/util/defaults"
	"github.com/apache/camel-k/pkg/util/kubernetes"
)

func printVersion() {
	logger.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	logger.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	logger.Info(fmt.Sprintf("Buildah Version: %v", defaults.BuildahVersion))
	logger.Info(fmt.Sprintf("Kaniko Version: %v", defaults.KanikoVersion))
	logger.Info(fmt.Sprintf("Camel K Operator Version: %v", defaults.Version))
	logger.Info(fmt.Sprintf("Camel K Default Runtime Version: %v", defaults.DefaultRuntimeVersion))
	logger.Info(fmt.Sprintf("Camel K Git Commit: %v", defaults.GitCommit))
}

// Run starts the Camel K operator.
func Run(ctx context.Context, cfg *rest.Config, newManager func(*rest.Config, manager.Options) (manager.Manager, error), healthPort, monitoringPort int32, leaderElection bool, leaderElectionID string) {
	rand.Seed(time.Now().UTC().UnixNano())

	_, err := maxprocs.Set(maxprocs.Logger(func(f string, a ...interface{}) { logger.Info(fmt.Sprintf(f, a)) }))
	exitOnError(err, "failed to set GOMAXPROCS from cgroups")

	printVersion()

	watchNamespace, err := getWatchNamespace()
	exitOnError(err, "failed to get watch namespace")

	// Increase maximum burst that is used by client-side throttling,
	// to prevent the requests made to apply the bundled Kamelets
	// from being throttled.
	cfg.QPS = 20
	cfg.Burst = 200
	c, err := client.NewClientWithConfig(false, cfg)
	exitOnError(err, "cannot initialize client")

	// We do not rely on the event broadcaster managed by controller runtime,
	// so that we can check the operator has been granted permission to create
	// Events. This is required for the operator to be installable by standard
	// admin users, that are not granted create permission on Events by default.
	broadcaster := record.NewBroadcaster()
	defer broadcaster.Shutdown()

	if ok, err := kubernetes.CheckPermission(ctx, c, corev1.GroupName, "events", watchNamespace, "", "create"); err != nil || !ok {
		// Do not sink Events to the server as they'll be rejected
		broadcaster = event.NewSinkLessBroadcaster(broadcaster)
		exitOnError(err, "cannot check permissions for creating Events")
		logger.Info("Event broadcasting is disabled because of missing permissions to create Events")
	}

	operatorNamespace := platform.GetOperatorNamespace()
	if operatorNamespace == "" {
		// Fallback to using the watch namespace when the operator is not in-cluster.
		// It does not support local (off-cluster) operator watching resources globally,
		// in which case it's not possible to determine a namespace.
		operatorNamespace = watchNamespace
		if operatorNamespace == "" {
			leaderElection = false
			logger.Info("unable to determine namespace for leader election")
		}
	}

	// Set the operator container image if it runs in-container
	platform.OperatorImage, err = getOperatorImage(ctx, c)
	exitOnError(err, "cannot get operator container image")

	if ok, err := kubernetes.CheckPermission(ctx, c, coordination.GroupName, "leases", operatorNamespace, "", "create"); err != nil || !ok {
		leaderElection = false
		exitOnError(err, "cannot check permissions for creating Leases")
		logger.Info("The operator is not granted permissions to create Leases")
	}

	if !leaderElection {
		logger.Info("Leader election is disabled!")
	}

	hasIntegrationLabel, err := labels.NewRequirement(v1.IntegrationLabel, selection.Exists, []string{})
	exitOnError(err, "cannot create Integration label selector")
	selector := labels.NewSelector().Add(*hasIntegrationLabel)

	selectors := cache.SelectorsByObject{
		&corev1.Pod{}:        {Label: selector},
		&appsv1.Deployment{}: {Label: selector},
		&batchv1.Job{}:       {Label: selector},
		&servingv1.Service{}: {Label: selector},
	}

	if ok, err := kubernetes.IsAPIResourceInstalled(c, batchv1.SchemeGroupVersion.String(), reflect.TypeOf(batchv1.CronJob{}).Name()); ok && err == nil {
		selectors[&batchv1.CronJob{}] = struct {
			Label labels.Selector
			Field fields.Selector
		}{
			Label: selector,
		}
	}

	mgr, err := newManager(c.GetConfig(), manager.Options{
		Namespace:                     watchNamespace,
		EventBroadcaster:              broadcaster,
		LeaderElection:                leaderElection,
		LeaderElectionNamespace:       operatorNamespace,
		LeaderElectionID:              leaderElectionID,
		LeaderElectionResourceLock:    resourcelock.LeasesResourceLock,
		LeaderElectionReleaseOnCancel: true,
		HealthProbeBindAddress:        ":" + strconv.Itoa(int(healthPort)),
		MetricsBindAddress:            ":" + strconv.Itoa(int(monitoringPort)),
		NewCache: cache.BuilderWithOptions(
			cache.Options{
				SelectorsByObject: selectors,
			},
		),
	})
	exitOnError(err, "")

	exitOnError(
		mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, "status.phase",
			func(obj ctrl.Object) []string {
				pod, _ := obj.(*corev1.Pod)
				return []string{string(pod.Status.Phase)}
			}),
		"unable to set up field indexer for status.phase: %v",
	)

	logger.Info("Configuring manager")
	exitOnError(mgr.AddHealthzCheck("health-probe", healthz.Ping), "Unable add liveness check")
	exitOnError(apis.AddToScheme(mgr.GetScheme()), "")
	exitOnError(controller.AddToManager(mgr), "")

	logger.Info("Installing operator resources")
	installCtx, installCancel := context.WithTimeout(ctx, 1*time.Minute)
	defer installCancel()
	install.OperatorStartupOptionalTools(installCtx, c, watchNamespace, operatorNamespace, logger)
	exitOnError(findOrCreateIntegrationPlatform(installCtx, c, operatorNamespace), "failed to create integration platform")

	logger.Info("Starting the manager")
	exitOnError(mgr.Start(ctx), "manager exited non-zero")
}

// findOrCreateIntegrationPlatform create default integration platform in operator namespace if not already exists.
func findOrCreateIntegrationPlatform(ctx context.Context, c client.Client, operatorNamespace string) error {
	var platformName string
	if defaults.OperatorID() != "" {
		platformName = defaults.OperatorID()
	} else {
		platformName = platform.DefaultPlatformName
	}

	if pl, err := kubernetes.GetIntegrationPlatform(ctx, c, platformName, operatorNamespace); pl == nil || k8serrors.IsNotFound(err) {
		defaultPlatform := v1.NewIntegrationPlatform(operatorNamespace, platformName)
		if defaultPlatform.Labels == nil {
			defaultPlatform.Labels = make(map[string]string)
		}
		defaultPlatform.Labels["camel.apache.org/platform.generated"] = "true"

		if _, err := c.CamelV1().IntegrationPlatforms(operatorNamespace).Create(ctx, &defaultPlatform, metav1.CreateOptions{}); err != nil {
			return err
		}

		// Make sure that IntegrationPlatform installed in operator namespace can be seen by others
		if err := install.IntegrationPlatformViewerRole(ctx, c, operatorNamespace); err != nil && !k8serrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "Error while installing global IntegrationPlatform viewer role")
		}
	} else {
		return err
	}

	return nil
}

// getWatchNamespace returns the Namespace the operator should be watching for changes.
func getWatchNamespace() (string, error) {
	ns, found := os.LookupEnv(platform.OperatorWatchNamespaceEnvVariable)
	if !found {
		return "", fmt.Errorf("%s must be set", platform.OperatorWatchNamespaceEnvVariable)
	}
	return ns, nil
}

// getOperatorImage returns the image currently used by the running operator if present (when running out of cluster, it may be absent).
func getOperatorImage(ctx context.Context, c ctrl.Reader) (string, error) {
	ns := platform.GetOperatorNamespace()
	name := platform.GetOperatorPodName()
	if ns == "" || name == "" {
		return "", nil
	}

	pod := corev1.Pod{}
	if err := c.Get(ctx, ctrl.ObjectKey{Namespace: ns, Name: name}, &pod); err != nil && k8serrors.IsNotFound(err) {
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
