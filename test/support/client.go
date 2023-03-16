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

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"sigs.k8s.io/controller-runtime/pkg/kcp"

	"github.com/kcp-dev/client-go/kubernetes"
	kcpclientset "github.com/kcp-dev/kcp/pkg/client/clientset/versioned/cluster"

	camel "github.com/apache/camel-k/pkg/client/camel/clientset/versioned"
	camelv1 "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1"
	camelv1alpha1 "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1alpha1"
)

type Client interface {
	Core() kubernetes.ClusterInterface
	Kcp() kcpclientset.ClusterInterface
	Camel
	Mapper() ClusterRESTMapper
	Scale() ClusterScaleInterface
}

type Camel interface {
	CamelV1() camelv1.CamelV1Interface
	CamelV1alpha1() camelv1alpha1.CamelV1alpha1Interface
}

type testClient struct {
	core   kubernetes.ClusterInterface
	kcp    kcpclientset.ClusterInterface
	camel  camel.Interface
	mapper ClusterRESTMapper
	scale  ClusterScaleInterface
}

var _ Client = (*testClient)(nil)

func (t *testClient) Core() kubernetes.ClusterInterface {
	return t.core
}

func (t *testClient) Kcp() kcpclientset.ClusterInterface {
	return t.kcp
}

func (t *testClient) CamelV1() camelv1.CamelV1Interface {
	return t.camel.CamelV1()
}

func (t *testClient) CamelV1alpha1() camelv1alpha1.CamelV1alpha1Interface {
	return t.camel.CamelV1alpha1()
}

func (t *testClient) Mapper() ClusterRESTMapper {
	return t.mapper
}

func (t *testClient) Scale() ClusterScaleInterface {
	return t.scale
}

func newTestClient() (Client, error) {
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{
			Context: clientcmdapi.Context{
				Cluster: "base",
			},
		},
	).ClientConfig()
	if err != nil {
		return nil, err
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	kcpClient, err := kcpclientset.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	httpClient, err := kcp.ClusterAwareHTTPClient(cfg)
	if err != nil {
		return nil, err
	}

	camelClient, err := camel.NewForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}

	restMapper, err := NewRESTMapperForConfig(cfg)
	if err != nil {
		return nil, err
	}

	scaleClient, err := NewScaleClientForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return &testClient{
		core:   kubeClient,
		kcp:    kcpClient,
		camel:  camelClient,
		mapper: restMapper,
		scale:  scaleClient,
	}, nil
}
