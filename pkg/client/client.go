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

package client

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"

	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/kcp"

	kcpclientset "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"
	schedulingv1alpha1 "github.com/kcp-dev/kcp/pkg/client/clientset/versioned/typed/scheduling/v1alpha1"

	camelclient "github.com/apache/camel-k/pkg/client"
	camel "github.com/apache/camel-k/pkg/client/camel/clientset/versioned"
	camelv1 "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1"
	camelv1alpha1 "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1alpha1"
)

type client struct {
	ctrl.Client
	discovery discovery.DiscoveryInterface
	kubernetes.Interface
	kcp    kcpclientset.Interface
	camel  camel.Interface
	scheme *runtime.Scheme
	config *rest.Config
	rest   rest.Interface
}

func NewClient(cfg *rest.Config, scheme *runtime.Scheme, c ctrl.Client) (Client, error) {
	httpClient, err := kcp.ClusterAwareHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	discoveryClient, err := NewClusterAwareDiscovery(cfg)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	kcpClient, err := kcpclientset.NewForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	camelClient, err := camel.NewForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	restClient, err := NewRESTClientForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}

	return &client{
		Client:    c,
		discovery: discoveryClient,
		Interface: kubeClient,
		kcp:       kcpClient,
		camel:     camelClient,
		scheme:    scheme,
		config:    cfg,
		rest:      restClient,
	}, nil
}

var _ Client = &client{}

func (c *client) Discovery() discovery.DiscoveryInterface {
	return c.discovery
}

func (c *client) KcpSchedulingV1alpha1() schedulingv1alpha1.SchedulingV1alpha1Interface {
	return c.kcp.SchedulingV1alpha1()
}

func (c *client) CamelV1() camelv1.CamelV1Interface {
	return c.camel.CamelV1()
}

func (c *client) CamelV1alpha1() camelv1alpha1.CamelV1alpha1Interface {
	return c.camel.CamelV1alpha1()
}

func (c *client) GetScheme() *runtime.Scheme {
	return c.scheme
}

func (c *client) GetConfig() *rest.Config {
	return c.config
}

func (c *client) GetCurrentNamespace(kubeConfig string) (string, error) {
	return camelclient.GetCurrentNamespace(kubeConfig)
}

func (c *client) ServerOrClientSideApplier() camelclient.ServerOrClientSideApplier {
	return camelclient.ServerOrClientSideApplier{
		Client: c,
	}
}

func (c *client) ScalesClient() (scale.ScalesGetter, error) {
	// Polymorphic scale client
	groupResources, err := restmapper.GetAPIGroupResources(c.Discovery())
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)
	resolver := scale.NewDiscoveryScaleKindResolver(c.Discovery())
	return scale.New(c.rest, mapper, dynamic.LegacyAPIPathResolverFunc, resolver), nil
}
