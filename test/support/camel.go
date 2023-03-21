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
	"encoding/json"

	"github.com/onsi/gomega"

	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	camelv1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	camelv1alpha1 "github.com/apache/camel-k/pkg/apis/camel/v1alpha1"
)

func Integration(t Test, namespace *corev1.Namespace, name string) func(g gomega.Gomega) *camelv1.Integration {
	return func(g gomega.Gomega) *camelv1.Integration {
		integration, err := t.Client().CamelV1().Integrations(namespace.Name).Get(Inside(t.Ctx(), namespace), name, metav1.GetOptions{})
		g.Expect(err).NotTo(gomega.HaveOccurred())
		return integration
	}
}

func IntegrationReplicas(integration *camelv1.Integration) int {
	if replicas := integration.Status.Replicas; replicas != nil {
		return int(*replicas)
	}
	return -1
}

func KameletBinding(t Test, namespace *corev1.Namespace, name string) func(g gomega.Gomega) *camelv1alpha1.KameletBinding {
	return func(g gomega.Gomega) *camelv1alpha1.KameletBinding {
		binding, err := t.Client().CamelV1alpha1().KameletBindings(namespace.Name).Get(Inside(t.Ctx(), namespace), name, metav1.GetOptions{})
		g.Expect(err).NotTo(gomega.HaveOccurred())
		return binding
	}
}

func KameletBindingPhase(binding *camelv1alpha1.KameletBinding) camelv1alpha1.KameletBindingPhase {
	return binding.Status.Phase
}

func Flow(t Test, f string) camelv1.Flow {
	t.T().Helper()
	data, err := yaml.YAMLToJSON([]byte(f))
	t.Expect(err).NotTo(gomega.HaveOccurred())
	return camelv1.Flow{
		RawMessage: camelv1.RawMessage(data),
	}
}

func EndpointProperties(t Test, properties map[string]string) *camelv1alpha1.EndpointProperties {
	t.T().Helper()
	data, err := json.Marshal(properties)
	t.Expect(err).NotTo(gomega.HaveOccurred())
	return &camelv1alpha1.EndpointProperties{
		RawMessage: data,
	}
}
