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
	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kcp-dev/logicalcluster/v3"
)

func Ingress(t Test, namespace *corev1.Namespace, name string) func(g gomega.Gomega) *networkingv1.Ingress {
	return func(g gomega.Gomega) *networkingv1.Ingress {
		ingress, err := t.Client().Core().Cluster(logicalcluster.From(namespace).Path()).NetworkingV1().Ingresses(namespace.Name).Get(t.Ctx(), name, metav1.GetOptions{})
		g.Expect(err).NotTo(gomega.HaveOccurred())
		return ingress
	}
}

func GetIngress(t Test, namespace *corev1.Namespace, name string) (*networkingv1.Ingress, error) {
	t.T().Helper()
	return t.Client().Core().NetworkingV1().Cluster(logicalcluster.From(namespace).Path()).Ingresses(namespace.Name).Get(t.Ctx(), name, metav1.GetOptions{})
}

func LoadBalancerIngresses(ingress *networkingv1.Ingress) []corev1.LoadBalancerIngress {
	return ingress.Status.LoadBalancer.Ingress
}
