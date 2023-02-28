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
	"context"
	"io"
	"net/http"
	"net/url"
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
	name := "hello"
	integration := &camelv1.Integration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: camelv1.IntegrationSpec{
			Flows: []camelv1.Flow{
				Flow(test, `
from:
  uri: platform-http:/hello
  steps:
    - transform:
        simple: Happy e2e testing!
    - to: log:info
`),
			},
			Traits: camelv1.Traits{
				Health: &traitv1.HealthTrait{
					Trait: traitv1.Trait{
						Enabled: pointer.Bool(true),
					},
				},
				Ingress: &traitv1.IngressTrait{
					// TODO: configure path to avoid conflicts
				},
			},
		},
	}

	_, err := test.Client().CamelV1().Integrations(namespace.Name).
		Create(Inside(test.Ctx(), workspace), integration, metav1.CreateOptions{})
	test.Expect(err).NotTo(HaveOccurred())

	test.Eventually(Integration(test, namespace, name), TestTimeoutLong).
		Should(WithTransform(ConditionStatus(camelv1.IntegrationConditionReady), Equal(corev1.ConditionTrue)))

	test.Eventually(Ingress(test, namespace, name), TestTimeoutShort).
		Should(WithTransform(LoadBalancerIngresses, HaveLen(1)))

	ingress, err := GetIngress(test, namespace, name)
	test.Expect(err).NotTo(HaveOccurred())

	endpoint := url.URL{Scheme: "http", Host: ingress.Status.LoadBalancer.Ingress[0].IP, Path: "hello"}
	response, err := requestBody(test.Ctx(), endpoint.String())
	test.Expect(err).NotTo(HaveOccurred())

	test.Expect(response, Equal([]byte("Happy e2e testing!")))
}

func requestBody(ctx context.Context, url string) (data []byte, err error) {
	request, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return
	}
	client := http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return
	}

	defer func(Body io.ReadCloser) {
		e := Body.Close()
		if err == nil {
			err = e
		}
	}(response.Body)

	data, err = io.ReadAll(response.Body)
	return
}
