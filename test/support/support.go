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

package support

import (
	"context"
	"os"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/kontext"

	"github.com/kcp-dev/logicalcluster/v3"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
)

const (
	testWorkspaceName = "TEST_WORKSPACE"

	clustersKubeConfigDir = "CLUSTERS_KUBECONFIG_DIR"

	TestTimeoutShort  = 1 * time.Minute
	TestTimeoutMedium = 2 * time.Minute
	TestTimeoutLong   = 5 * time.Minute
)

var (
	TestWorkspace = getEnvLogicalClusterName(testWorkspaceName, logicalcluster.NewPath("root:camel-k"))

	CamelWorkspaceType = tenancyv1alpha1.WorkspaceTypeReference{Name: "camel-k"}

	ApplyOptions = metav1.ApplyOptions{FieldManager: "camel-kcp-test", Force: true}
)

func getEnvLogicalClusterName(key string, fallback logicalcluster.Path) logicalcluster.Path {
	value, found := os.LookupEnv(key)
	if !found {
		return fallback
	}
	return logicalcluster.NewPath(value)
}

type inside interface {
	*tenancyv1alpha1.Workspace | *corev1.Namespace
}

func Inside[T inside](ctx context.Context, object T) context.Context {
	switch o := any(object).(type) {
	case *tenancyv1alpha1.Workspace:
		return kontext.WithCluster(ctx, logicalcluster.Name(logicalcluster.From(o).Path().Join(o.Name).String()))
	case *corev1.Namespace:
		return kontext.WithCluster(ctx, logicalcluster.From(o))
	default:
		return ctx
	}
}
