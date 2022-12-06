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
	"context"
	"sync"
	"testing"

	"github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	tenancyv1beta1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1beta1"
)

type Test interface {
	T() *testing.T
	Ctx() context.Context
	Client() Client

	gomega.Gomega

	NewTestNamespace(...Option) *corev1.Namespace
	NewTestWorkspace() *tenancyv1beta1.Workspace
}

type Option interface {
	applyTo(interface{}) error
}

type errorOption func(to interface{}) error

func (o errorOption) applyTo(to interface{}) error {
	return o(to)
}

var _ Option = errorOption(nil)

func With(t *testing.T) Test {
	ctx := context.Background()
	if deadline, ok := t.Deadline(); ok {
		withDeadline, cancel := context.WithDeadline(ctx, deadline)
		t.Cleanup(cancel)
		ctx = withDeadline
	}

	return &T{
		WithT: gomega.NewWithT(t),
		t:     t,
		ctx:   ctx,
	}
}

type T struct {
	*gomega.WithT
	t      *testing.T
	ctx    context.Context
	client Client
	once   sync.Once
}

func (t *T) T() *testing.T {
	return t.t
}

func (t *T) Ctx() context.Context {
	return t.ctx
}

func (t *T) Client() Client {
	t.once.Do(func() {
		c, err := newTestClient()
		if err != nil {
			t.T().Fatalf("Error creating client: %v", err)
		}
		t.client = c
	})
	return t.client
}

func (t *T) NewTestWorkspace() *tenancyv1beta1.Workspace {
	workspace := createTestWorkspace(t)
	t.T().Cleanup(func() {
		deleteTestWorkspace(t, workspace)
	})
	t.T().Logf("Creating workspace %v:%v", TestOrganization, workspace.Name)
	t.Eventually(Workspace(t, workspace.Name)).
		Should(gomega.WithTransform(WorkspacePhase, gomega.Equal(tenancyv1alpha1.ClusterWorkspacePhaseReady)))
	return workspace
}

func (t *T) NewTestNamespace(options ...Option) *corev1.Namespace {
	namespace := createTestNamespace(t, options...)
	t.T().Cleanup(func() {
		deleteTestNamespace(t, namespace)
	})
	return namespace
}
