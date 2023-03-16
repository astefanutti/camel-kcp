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

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	ctrl "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/kcp-dev/logicalcluster/v3"
)

func Scale[T ctrl.Object](t Test, provider func(g gomega.Gomega) T) func(g gomega.Gomega) *autoscalingv1.Scale {
	return func(g gomega.Gomega) *autoscalingv1.Scale {
		object := provider(g)

		// FIXME: Remove hard-coded GR
		scale, err := t.Client().Scale().Cluster(logicalcluster.From(object).Path()).Scales(object.GetNamespace()).
			Get(t.Ctx(), schema.GroupResource{Group: "camel.apache.org", Resource: "integrations"}, object.GetName(), metav1.GetOptions{})
		g.Expect(err).NotTo(gomega.HaveOccurred())

		return scale
	}
}

func StatusReplicas(scale *autoscalingv1.Scale) int {
	return int(scale.Status.Replicas)
}
