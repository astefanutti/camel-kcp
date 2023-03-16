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

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/scale"
	"k8s.io/client-go/util/flowcontrol"

	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/kcp-dev/logicalcluster/v3"

	kcpclient "github.com/kcp-dev/apimachinery/v2/pkg/client"
)

type ClusterScaleInterface interface {
	Cluster(logicalcluster.Path) scale.ScalesGetter
}

var _ ClusterScaleInterface = (*ClusterClientset)(nil)

type ClusterClientset struct {
	clientCache kcpclient.Cache[scale.ScalesGetter]
}

func (c ClusterClientset) Cluster(clusterPath logicalcluster.Path) scale.ScalesGetter {
	return c.clientCache.ClusterOrDie(clusterPath)
}

// NewForConfig creates a new ClusterClientset for the given config.
// If config's RateLimiter is not set and QPS and Burst are acceptable,
// NewForConfig will generate a rate-limiter in configShallowCopy.
// NewForConfig is equivalent to NewForConfigAndClient(c, httpClient),
// where httpClient was generated with rest.HTTPClientFor(c).
func NewForConfig(c *rest.Config) (*ClusterClientset, error) {
	configShallowCopy := *c

	if configShallowCopy.UserAgent == "" {
		configShallowCopy.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	// share the transport between all clients
	httpClient, err := rest.HTTPClientFor(&configShallowCopy)
	if err != nil {
		return nil, err
	}

	return NewForConfigAndClient(&configShallowCopy, httpClient)
}

// NewForConfigAndClient creates a new ClusterClientset for the given config and http client.
// Note the http client provided takes precedence over the configured transport values.
// If config's RateLimiter is not set and QPS and Burst are acceptable,
// NewForConfigAndClient will generate a rate-limiter in configShallowCopy.
func NewForConfigAndClient(c *rest.Config, httpClient *http.Client) (*ClusterClientset, error) {
	configShallowCopy := *c
	if configShallowCopy.RateLimiter == nil && configShallowCopy.QPS > 0 {
		if configShallowCopy.Burst <= 0 {
			return nil, fmt.Errorf("burst is required to be greater than 0 when RateLimiter is not set and QPS is set to greater than 0")
		}
		configShallowCopy.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(configShallowCopy.QPS, configShallowCopy.Burst)
	}

	cache := kcpclient.NewCache(c, httpClient, &kcpclient.Constructor[scale.ScalesGetter]{
		NewForConfigAndClient: func(cfg *rest.Config, client *http.Client) (scale.ScalesGetter, error) {
			// Should lazy discovery option be activated?
			m, err := apiutil.NewDynamicRESTMapper(cfg)
			if err != nil {
				return nil, err
			}
			d, err := discovery.NewDiscoveryClientForConfig(cfg)
			if err != nil {
				return nil, err
			}
			return scale.NewForConfig(cfg, m, dynamic.LegacyAPIPathResolverFunc, scale.NewDiscoveryScaleKindResolver(d))
		},
	})

	return &ClusterClientset{clientCache: cache}, nil
}
