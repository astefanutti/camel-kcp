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
	"net/http"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/scale"
)

// NewClusterAwareDiscovery returns a discovery.DiscoveryInterface that works with APIExport virtual workspace API server.
func NewClusterAwareDiscovery(config *rest.Config) (discovery.DiscoveryInterface, error) {
	c := rest.CopyConfig(config)
	c.Host += "/clusters/*"
	return discovery.NewDiscoveryClientForConfig(c)
}

var scaleConverter = scale.NewScaleConverter()
var codecs = serializer.NewCodecFactory(scaleConverter.Scheme())

func NewRESTClientForConfigAndClient(config *rest.Config, httpClient *http.Client) (*rest.RESTClient, error) {
	cfg := rest.CopyConfig(config)
	// so that the RESTClientFor doesn't complain
	cfg.GroupVersion = &schema.GroupVersion{}
	cfg.NegotiatedSerializer = codecs.WithoutConversion()
	if len(cfg.UserAgent) == 0 {
		cfg.UserAgent = rest.DefaultKubernetesUserAgent()
	}
	return rest.RESTClientForConfigAndClient(cfg, httpClient)
}
