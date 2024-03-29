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

	corev1alpha1 "github.com/kcp-dev/kcp/pkg/apis/core/v1alpha1"
	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	workloadv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/workload/v1alpha1"
)

type Test interface {
	T() *testing.T
	Ctx() context.Context
	Client() Client

	gomega.Gomega

	NewTestNamespace(...Option[*corev1.Namespace]) *corev1.Namespace
	NewTestWorkspace(...Option[*tenancyv1alpha1.Workspace]) *tenancyv1alpha1.Workspace
	NewSyncTarget(name string, options ...Option[*SyncTargetConfig]) *workloadv1alpha1.SyncTarget
}

type Option[T any] interface {
	applyTo(to T) error
}

type errorOption[T any] func(to T) error

// nolint: unused
// To be removed when the false-positivity is fixed.
func (o errorOption[T]) applyTo(to T) error {
	return o(to)
}

var _ Option[any] = errorOption[any](nil)

func With(t *testing.T) Test {
	t.Helper()
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
	t *testing.T
	// nolint: containedctx
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

func (t *T) NewTestWorkspace(options ...Option[*tenancyv1alpha1.Workspace]) *tenancyv1alpha1.Workspace {
	t.T().Helper()
	workspace := createTestWorkspace(t, options...)
	t.T().Cleanup(func() {
		deleteTestWorkspace(t, workspace)
	})
	t.Eventually(Workspace(t, workspace.Name), TestTimeoutShort).
		Should(gomega.WithTransform(WorkspacePhase, gomega.Equal(corev1alpha1.LogicalClusterPhaseReady)))

	var err error
	workspace, err = GetWorkspace(t, workspace.Name)
	t.Expect(err).NotTo(gomega.HaveOccurred())

	t.T().Logf("Created workspace %v:%v", TestWorkspace, workspace.Name)

	return workspace
}

func (t *T) NewTestNamespace(options ...Option[*corev1.Namespace]) *corev1.Namespace {
	t.T().Helper()
	namespace := createTestNamespace(t, options...)
	t.T().Cleanup(func() {
		deleteTestNamespace(t, namespace)
	})
	return namespace
}

func (t *T) NewSyncTarget(name string, options ...Option[*SyncTargetConfig]) *workloadv1alpha1.SyncTarget {
	workloadCluster, cleanup := createSyncTarget(t, name, options...)
	t.T().Cleanup(func() {
		deleteSyncTarget(t, workloadCluster)
	})
	t.T().Cleanup(func() {
		t.Expect(cleanup()).To(gomega.Succeed())
	})
	return workloadCluster
}
