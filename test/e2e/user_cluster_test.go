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

package e2e

import (
	"testing"

	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kcp-dev/logicalcluster/v3"

	schedulingv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/scheduling/v1alpha1"
	workloadv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/workload/v1alpha1"

	camelv1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	. "github.com/apache/camel-kcp/test/support"
)

func TestUserCluster(t *testing.T) {
	test := With(t)
	test.T().Parallel()

	// Create the test workspace
	workspace := test.NewTestWorkspace(OfType(CamelWorkspaceType))

	// Create the syncer namespace
	test.NewTestNamespace(InWorkspace[*corev1.Namespace](workspace), WithName[*corev1.Namespace]("kcp-syncer"))

	// Create the syncer
	test.NewSyncTarget("user", InWorkspace[*SyncTargetConfig](workspace), WithLabel[*SyncTargetConfig]("org.apache.camel/user-plane", ""), WithKubeConfigByName, Syncer().Namespace("kcp-syncer"))

	// Create location
	location := &schedulingv1alpha1.Location{
		TypeMeta: metav1.TypeMeta{
			APIVersion: schedulingv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Location",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "user",
			Labels: map[string]string{
				"org.apache.camel/user-plane": "",
			},
		},
		Spec: schedulingv1alpha1.LocationSpec{
			Resource: schedulingv1alpha1.GroupVersionResource{
				Group:    workloadv1alpha1.SchemeGroupVersion.Group,
				Version:  workloadv1alpha1.SchemeGroupVersion.Version,
				Resource: "synctargets",
			},
			InstanceSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Operator: metav1.LabelSelectorOpExists,
						Key:      "org.apache.camel/user-plane",
					},
				},
			},
		},
	}
	location, err := test.Client().Kcp().SchedulingV1alpha1().Cluster(logicalcluster.NewPath(workspace.Spec.Cluster)).Locations().
		Create(test.Ctx(), location, metav1.CreateOptions{})
	test.Expect(err).NotTo(HaveOccurred())

	// Create placement
	placement := &schedulingv1alpha1.Placement{
		TypeMeta: metav1.TypeMeta{
			APIVersion: schedulingv1alpha1.SchemeGroupVersion.String(),
			Kind:       "Placement",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "user",
		},
		Spec: schedulingv1alpha1.PlacementSpec{
			LocationResource: schedulingv1alpha1.GroupVersionResource{
				Group:    workloadv1alpha1.SchemeGroupVersion.Group,
				Version:  workloadv1alpha1.SchemeGroupVersion.Version,
				Resource: "synctargets",
			},
			LocationSelectors: []metav1.LabelSelector{
				{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Operator: metav1.LabelSelectorOpExists,
							Key:      "org.apache.camel/user-plane",
						},
					},
				},
			},
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"kubernetes.io/metadata.name": "user",
				},
			},
		},
	}
	placement, err = test.Client().Kcp().SchedulingV1alpha1().Cluster(logicalcluster.NewPath(workspace.Spec.Cluster)).Placements().
		Create(test.Ctx(), placement, metav1.CreateOptions{})
	test.Expect(err).NotTo(HaveOccurred())

	// Create the integration namespace
	namespace := test.NewTestNamespace(InWorkspace[*corev1.Namespace](workspace), WithName[*corev1.Namespace]("user"))

	// Create the Integration
	name := "hello"
	integration := &camelv1.Integration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: camelv1.IntegrationSpec{
			Flows: []camelv1.Flow{
				Flow(test, `
from:
  uri: timer:hello
  steps:
    - transform:
        simple: Happy e2e testing!
    - to: log:info
`),
			},
		},
	}

	_, err = test.Client().CamelV1().Integrations(namespace.Name).
		Create(Inside(test.Ctx(), workspace), integration, metav1.CreateOptions{})
	test.Expect(err).NotTo(HaveOccurred())

	test.Eventually(Integration(test, namespace, name), TestTimeoutLong).
		Should(WithTransform(ConditionStatus(camelv1.IntegrationConditionReady), Equal(corev1.ConditionTrue)))
}
