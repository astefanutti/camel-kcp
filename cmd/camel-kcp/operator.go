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
	"os"
	"runtime"

	"github.com/pkg/errors"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"

	ctrl "sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	"github.com/apache/camel-k/pkg/client"
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

// findOrCreateIntegrationPlatform create default integration platform in operator namespace if not already exists.
// nolint: unused
func findOrCreateIntegrationPlatform(ctx context.Context, c client.Client, operatorNamespace string) error {
	var platformName string
	if defaults.OperatorID() != "" {
		platformName = defaults.OperatorID()
	} else {
		platformName = platform.DefaultPlatformName
	}

	if pl, err := kubernetes.GetIntegrationPlatform(ctx, c, platformName, operatorNamespace); pl == nil || apierrors.IsNotFound(err) {
		defaultPlatform := v1.NewIntegrationPlatform(operatorNamespace, platformName)
		if defaultPlatform.Labels == nil {
			defaultPlatform.Labels = make(map[string]string)
		}
		defaultPlatform.Labels["camel.apache.org/platform.generated"] = "true"

		if _, err := c.CamelV1().IntegrationPlatforms(operatorNamespace).Create(ctx, &defaultPlatform, metav1.CreateOptions{}); err != nil {
			return err
		}

		// Make sure that IntegrationPlatform installed in operator namespace can be seen by others
		if err := install.IntegrationPlatformViewerRole(ctx, c, operatorNamespace); err != nil && !apierrors.IsAlreadyExists(err) {
			return errors.Wrap(err, "Error while installing global IntegrationPlatform viewer role")
		}
	} else {
		return err
	}

	return nil
}

// getOperatorImage returns the image currently used by the running operator if present (when running out of cluster, it may be absent).
// nolint: unused
func getOperatorImage(ctx context.Context, c ctrl.Reader) (string, error) {
	ns := platform.GetOperatorNamespace()
	name := platform.GetOperatorPodName()
	if ns == "" || name == "" {
		return "", nil
	}

	pod := corev1.Pod{}
	if err := c.Get(ctx, ctrl.ObjectKey{Namespace: ns, Name: name}, &pod); err != nil && apierrors.IsNotFound(err) {
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

func isAPIResourceInstalled(c discovery.DiscoveryInterface, groupVersion string, kind string) (bool, error) {
	resources, err := c.ServerResourcesForGroupVersion(groupVersion)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	for _, resource := range resources.APIResources {
		if resource.Kind == kind {
			return true, nil
		}
	}

	return false, nil
}
