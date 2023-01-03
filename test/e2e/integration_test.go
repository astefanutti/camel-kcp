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
	"k8s.io/utils/pointer"

	camelv1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	traitv1 "github.com/apache/camel-k/pkg/apis/camel/v1/trait"

	. "github.com/apache/camel-kcp/test/support"
)

func TestIntegration(t *testing.T) {
	test := With(t)
	test.T().Parallel()

	// Create the test workspace
	workspace := test.NewTestWorkspace(OfType(CamelWorkspaceType))

	// Create a namespace
	namespace := test.NewTestNamespace(InWorkspace[*corev1.Namespace](workspace))

	// Create the Integration
	integration := &camelv1.Integration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: camelv1.IntegrationSpec{
			Flows: []camelv1.Flow{
				{
					camelv1.RawMessage(`
from:
  uri: platform-http:/hello
  steps:
    - transform:
        simple: Happy e2e testing!
    - to: log:info
`),
				},
			},
			Traits: camelv1.Traits{
				Health: &traitv1.HealthTrait{
					Trait: traitv1.Trait{
						Enabled: pointer.Bool(true),
					},
				},
			},
		},
	}
	_, err := test.Client().CamelV1().Integrations(namespace.Name).
		Create(Inside(test.Ctx(), workspace), integration, metav1.CreateOptions{})
	test.Expect(err).NotTo(HaveOccurred())

	test.Eventually(Integration(test, namespace, integration.Name), TestTimeoutMedium).
		Should(WithTransform(ConditionStatus(camelv1.IntegrationConditionReady), Equal(corev1.ConditionTrue)))
}
