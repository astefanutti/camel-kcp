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
	. "github.com/onsi/gomega/gstruct"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kcp-dev/logicalcluster/v2"

	v1 "github.com/apache/camel-k/pkg/apis/camel/v1"

	. "github.com/apache/camel-kcp/test/support"
)

func TestIntegration(t *testing.T) {
	test := With(t)
	test.T().Parallel()

	// Create the test workspace
	workspace := test.NewTestWorkspace()

	// Create a namespace
	namespace := test.NewTestNamespace(InWorkspace(workspace))

	// Create the Integration
	integration := &v1.Integration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
	}
	_, err := test.Client().CamelV1().Integrations(namespace.Name).Create(logicalcluster.WithCluster(test.Ctx(), logicalcluster.From(workspace)), integration, metav1.CreateOptions{})
	test.Expect(err).NotTo(HaveOccurred())
}
