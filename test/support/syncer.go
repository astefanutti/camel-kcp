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

func Syncer() *syncer {
	return &syncer{
		// TODO: allow this to be set based on the KCP version exported by default
		image:     "ghcr.io/kcp-dev/kcp/syncer:main",
		namespace: "default",
		replicas:  1,
	}
}

type syncer struct {
	image           string
	namespace       string
	replicas        int
	resourcesToSync []string
}

var _ Option[*SyncTargetConfig] = (*syncer)(nil)

func (s *syncer) Image(image string) *syncer {
	s.image = image
	return s
}

func (s *syncer) Namespace(namespace string) *syncer {
	s.namespace = namespace
	return s
}

func (s *syncer) Replicas(replicas int) *syncer {
	s.replicas = replicas
	return s
}

func (s *syncer) ResourcesToSync(resourcesToSync ...string) *syncer {
	s.resourcesToSync = resourcesToSync
	return s
}

func (s *syncer) applyTo(config *SyncTargetConfig) error {
	config.syncer = *s
	return nil
}
