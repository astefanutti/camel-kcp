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
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/flowcontrol"

	camelv1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	camelv1alpha1 "github.com/apache/camel-k/pkg/apis/camel/v1alpha1"
	camelv1client "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1"
	camelv1alpha1client "github.com/apache/camel-k/pkg/client/camel/clientset/versioned/typed/camel/v1alpha1"
)

func NewCamelClientsForConfigAndClient(c *rest.Config, httpClient *http.Client) (*camelv1client.CamelV1Client, *camelv1alpha1client.CamelV1alpha1Client, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		if configShallowCopy.Burst <= 0 {
			return nil, nil, fmt.Errorf("burst is required to be greater than 0 when RateLimiter is not set and QPS is set to greater than 0")
		}
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}

	camelV1, err := newCamelV1ClientForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, nil, err
	}
	camelV1alpha1, err := newCamelV1alpha1ClientForConfigAndClient(&configShallowCopy, httpClient)
	if err != nil {
		return nil, nil, err
	}

	return camelV1, camelV1alpha1, nil
}

func newCamelV1alpha1ClientForConfigAndClient(c *rest.Config, h *http.Client) (*camelv1alpha1client.CamelV1alpha1Client, error) {
	config := *c
	if err := setConfigDefaults(camelv1alpha1.SchemeGroupVersion, &config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientForConfigAndClient(&config, h)
	if err != nil {
		return nil, err
	}
	return camelv1alpha1client.New(client), nil
}

func newCamelV1ClientForConfigAndClient(c *rest.Config, h *http.Client) (*camelv1client.CamelV1Client, error) {
	config := *c
	if err := setConfigDefaults(camelv1.SchemeGroupVersion, &config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientForConfigAndClient(&config, h)
	if err != nil {
		return nil, err
	}
	return camelv1client.New(client), nil
}

func setConfigDefaults(gv schema.GroupVersion, config *rest.Config) error {
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}
