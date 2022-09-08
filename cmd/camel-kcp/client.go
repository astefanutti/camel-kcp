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

package main

import (
	"net/http"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"

	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/kcp"

	"github.com/apache/camel-k/pkg/client"
	camel "github.com/apache/camel-k/pkg/client/camel/clientset/versioned"
	camelv1 "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1"
	camelv1alpha1 "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1alpha1"
)

// newClusterAwareDiscovery returns a discovery.DiscoveryInterface that works with APIExport virtual workspace API server.
func newClusterAwareDiscovery(config *rest.Config) (discovery.DiscoveryInterface, error) {
	c := rest.CopyConfig(config)
	c.Host += "/clusters/*"
	return discovery.NewDiscoveryClientForConfig(c)
}

type kcpClient struct {
	ctrl.Client
	discovery discovery.DiscoveryInterface
	kubernetes.Interface
	camel      camel.Interface
	scheme     *runtime.Scheme
	config     *rest.Config
	restClient rest.Interface
}

func NewClient(cfg *rest.Config, scheme *runtime.Scheme, discovery discovery.DiscoveryInterface, c ctrl.Client) (client.Client, error) {
	httpClient, err := kcp.ClusterAwareHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	camelClientset, err := camel.NewForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}

	restClient, err := newRESTClientForConfigAndClient(cfg, httpClient)
	if err != nil {
		return nil, err
	}

	return &kcpClient{
		Client:     c,
		discovery:  discovery,
		Interface:  clientset,
		camel:      camelClientset,
		scheme:     scheme,
		config:     cfg,
		restClient: restClient,
	}, nil
}

var _ client.Client = &kcpClient{}

func (c *kcpClient) Discovery() discovery.DiscoveryInterface {
	return c.discovery
}

func (c *kcpClient) CamelV1() camelv1.CamelV1Interface {
	return c.camel.CamelV1()
}

func (c *kcpClient) CamelV1alpha1() camelv1alpha1.CamelV1alpha1Interface {
	return c.camel.CamelV1alpha1()
}

func (c *kcpClient) GetScheme() *runtime.Scheme {
	return c.scheme
}

func (c *kcpClient) GetConfig() *rest.Config {
	return c.config
}

func (c *kcpClient) GetCurrentNamespace(kubeConfig string) (string, error) {
	return client.GetCurrentNamespace(kubeConfig)
}

func (c *kcpClient) ServerOrClientSideApplier() client.ServerOrClientSideApplier {
	return client.ServerOrClientSideApplier{
		Client: c,
	}
}

func (c *kcpClient) ScalesClient() (scale.ScalesGetter, error) {
	// Polymorphic scale client
	groupResources, err := restmapper.GetAPIGroupResources(c.Discovery())
	if err != nil {
		return nil, err
	}
	mapper := restmapper.NewDiscoveryRESTMapper(groupResources)
	resolver := scale.NewDiscoveryScaleKindResolver(c.Discovery())
	return scale.New(c.restClient, mapper, dynamic.LegacyAPIPathResolverFunc, resolver), nil
}

var scaleConverter = scale.NewScaleConverter()
var codecs = serializer.NewCodecFactory(scaleConverter.Scheme())

func newRESTClientForConfigAndClient(config *rest.Config, httpClient *http.Client) (*rest.RESTClient, error) {
	cfg := rest.CopyConfig(config)
	// so that the RESTClientFor doesn't complain
	cfg.GroupVersion = &schema.GroupVersion{}
	cfg.NegotiatedSerializer = codecs.WithoutConversion()
	if len(cfg.UserAgent) == 0 {
		cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	return rest.RESTClientForConfigAndClient(cfg, httpClient)
}
