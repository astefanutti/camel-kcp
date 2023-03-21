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

func WithSyncer() *Syncer {
	return &Syncer{
		// TODO: allow this to be set based on the KCP version exported by default
		image:     "ghcr.io/kcp-dev/kcp/syncer:main",
		namespace: "default",
		replicas:  1,
	}
}

type Syncer struct {
	image           string
	namespace       string
	replicas        int
	resourcesToSync []string
}

var _ Option[*SyncTargetConfig] = (*Syncer)(nil)

func (s *Syncer) Image(image string) *Syncer {
	s.image = image
	return s
}

func (s *Syncer) Namespace(namespace string) *Syncer {
	s.namespace = namespace
	return s
}

func (s *Syncer) Replicas(replicas int) *Syncer {
	s.replicas = replicas
	return s
}

func (s *Syncer) ResourcesToSync(resourcesToSync ...string) *Syncer {
	s.resourcesToSync = resourcesToSync
	return s
}

// nolint: unused
// To be removed when the false-positivity is fixed.
func (s *Syncer) applyTo(config *SyncTargetConfig) error {
	config.syncer = *s
	return nil
}
