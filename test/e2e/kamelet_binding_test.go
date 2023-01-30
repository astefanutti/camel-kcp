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

	camelv1alpha1 "github.com/apache/camel-k/pkg/apis/camel/v1alpha1"

	. "github.com/apache/camel-kcp/test/support"
)

func TestKameletBinding(t *testing.T) {
	test := With(t)
	test.T().Parallel()

	// Create the test workspace
	workspace := test.NewTestWorkspace(OfType(CamelWorkspaceType))

	// Create a namespace
	namespace := test.NewTestNamespace(InWorkspace[*corev1.Namespace](workspace))

	// Create the Integration
	binding := &camelv1alpha1.KameletBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: camelv1alpha1.KameletBindingSpec{
			Source: camelv1alpha1.Endpoint{
				Ref: &corev1.ObjectReference{
					Kind:       "Kamelet",
					APIVersion: camelv1alpha1.SchemeGroupVersion.String(),
					Name:       "timer-source",
				},
				Properties: EndpointProperties(test, map[string]string{
					"message": "Hello!",
				}),
			},
			Sink: camelv1alpha1.Endpoint{
				Ref: &corev1.ObjectReference{
					Kind:       "Kamelet",
					APIVersion: camelv1alpha1.SchemeGroupVersion.String(),
					Name:       "log-sink",
				},
			},
		},
	}

	_, err := test.Client().CamelV1alpha1().KameletBindings(namespace.Name).
		Create(Inside(test.Ctx(), workspace), binding, metav1.CreateOptions{})
	test.Expect(err).NotTo(HaveOccurred())

	test.Eventually(KameletBinding(test, namespace, binding.Name), TestTimeoutLong).
		Should(And(
			WithTransform(KameletBindingPhase, Equal(camelv1alpha1.KameletBindingPhaseReady)),
			WithTransform(ConditionStatus(camelv1alpha1.KameletBindingConditionReady), Equal(corev1.ConditionTrue)),
		))
}
