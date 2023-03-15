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

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"sigs.k8s.io/controller-runtime/pkg/kcp"

	"github.com/kcp-dev/client-go/kubernetes"

	kcpclientset "github.com/kcp-dev/kcp/pkg/client/clientset/versioned/cluster"

	camel "github.com/apache/camel-k/pkg/client/camel/clientset/versioned"
	camelv1 "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1"
	camelv1alpha1 "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1alpha1"

	"github.com/apache/camel-kcp/pkg/client"
)

type Client interface {
	Core() kubernetes.ClusterInterface
	Kcp() kcpclientset.ClusterInterface
	Camel
	Scale() scale.ScalesGetter
	Mapper() meta.RESTMapper
}

type Camel interface {
	CamelV1() camelv1.CamelV1Interface
	CamelV1alpha1() camelv1alpha1.CamelV1alpha1Interface
}

type testClient struct {
	core      kubernetes.ClusterInterface
	kcp       kcpclientset.ClusterInterface
	scale     scale.ScalesGetter
	camel     camel.Interface
	discovery discovery.DiscoveryInterface
	mapper    meta.RESTMapper
	rest      rest.Interface
}

func (t *testClient) Core() kubernetes.ClusterInterface {
	return t.core
}

func (t *testClient) Kcp() kcpclientset.ClusterInterface {
	return t.kcp
}

func (t *testClient) Mapper() meta.RESTMapper {
	return t.mapper
}

func (t *testClient) Scale() scale.ScalesGetter {
	return t.scale
}

func (t *testClient) CamelV1() camelv1.CamelV1Interface {
	return t.camel.CamelV1()
}

func (t *testClient) CamelV1alpha1() camelv1alpha1.CamelV1alpha1Interface {
	return t.camel.CamelV1alpha1()
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

	discoveryClient, err := client.NewClusterAwareDiscovery(cfg)
	if err != nil {
		return nil, err
	}

	restMapper, err := newRESTMapper(discoveryClient)
	if err != nil {
		return nil, err
	}

	restClient, err := client.NewRESTClientForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}

	scaleClient, err := newScaleClient(discoveryClient, restMapper, restClient)
	if err != nil {
		return nil, err
	}

	return &testClient{
		core:      kubeClient,
		kcp:       kcpClient,
		scale:     scaleClient,
		camel:     camelClient,
		discovery: discoveryClient,
		mapper:    restMapper,
		rest:      restClient,
	}, nil
}

func newRESTMapper(d discovery.DiscoveryInterface) (meta.RESTMapper, error) {
	groupResources, err := restmapper.GetAPIGroupResources(d)
	if err != nil {
		return nil, err
	}
	return restmapper.NewDiscoveryRESTMapper(groupResources), nil
}

// newScaleClient returns a polymorphic scale client
func newScaleClient(d discovery.DiscoveryInterface, m meta.RESTMapper, r rest.Interface) (scale.ScalesGetter, error) {
	resolver := scale.NewDiscoveryScaleKindResolver(d)
	return scale.New(r, m, dynamic.LegacyAPIPathResolverFunc, resolver), nil
}
