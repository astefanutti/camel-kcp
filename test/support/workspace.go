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

	"github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/kcp-dev/logicalcluster/v2"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	tenancyv1beta1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1beta1"
)

type WorkspaceRef interface {
	*tenancyv1beta1.Workspace | logicalcluster.Name
}

func InWorkspace[T metav1.Object, W WorkspaceRef](workspace W) Option[T] {
	switch w := any(workspace).(type) {
	case *tenancyv1beta1.Workspace:
		return &inWorkspace[T]{logicalcluster.From(w).Join(w.Name)}
	case logicalcluster.Name:
		return &inWorkspace[T]{w}
	default:
		return errorOption[T](func(to T) error {
			return fmt.Errorf("unsupported type passed to InWorkspace option: %s", w)
		})
	}
}

type inWorkspace[T metav1.Object] struct {
	workspace logicalcluster.Name
}

var _ Option[metav1.Object] = &inWorkspace[metav1.Object]{}

// nolint: unused
// To be removed when the false-positivity is fixed.
func (o *inWorkspace[T]) applyTo(to T) error {
	to.SetAnnotations(map[string]string{
		logicalcluster.AnnotationKey: o.workspace.String(),
	})

	return nil
}

func OfType(typeRef tenancyv1alpha1.ClusterWorkspaceTypeReference) Option[*tenancyv1beta1.Workspace] {
	return &ofType{workspaceTypeReference: typeRef}
}

type ofType struct {
	workspaceTypeReference tenancyv1alpha1.ClusterWorkspaceTypeReference
}

var _ Option[*tenancyv1beta1.Workspace] = &ofType{}

// nolint: unused
// To be removed when the false-positivity is fixed.
func (o *ofType) applyTo(to *tenancyv1beta1.Workspace) error {
	to.Spec.Type = o.workspaceTypeReference
	return nil
}

func Workspace(t Test, name string) func() *tenancyv1beta1.Workspace {
	return func() *tenancyv1beta1.Workspace {
		c, err := t.Client().Kcp().Cluster(TestWorkspace).TenancyV1beta1().Workspaces().Get(t.Ctx(), name, metav1.GetOptions{})
		t.Expect(err).NotTo(gomega.HaveOccurred())
		return c
	}
}

func WorkspacePhase(workspace *tenancyv1beta1.Workspace) tenancyv1alpha1.ClusterWorkspacePhaseType {
	return workspace.Status.Phase
}

func createTestWorkspace(t Test, options ...Option[*tenancyv1beta1.Workspace]) *tenancyv1beta1.Workspace {
	t.T().Helper()
	workspace := &tenancyv1beta1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-ws-",
		},
		Spec: tenancyv1beta1.WorkspaceSpec{
			Type: tenancyv1alpha1.ClusterWorkspaceTypeReference{},
		},
	}

	for _, option := range options {
		t.Expect(option.applyTo(workspace)).To(gomega.Succeed())
	}

	workspace, err := t.Client().Kcp().Cluster(TestWorkspace).TenancyV1beta1().Workspaces().Create(t.Ctx(), workspace, metav1.CreateOptions{})
	if err != nil {
		t.Expect(err).NotTo(gomega.HaveOccurred())
	}

	return workspace
}

func deleteTestWorkspace(t Test, workspace *tenancyv1beta1.Workspace) {
	t.T().Helper()
	propagationPolicy := metav1.DeletePropagationBackground
	err := t.Client().Kcp().Cluster(logicalcluster.From(workspace)).TenancyV1beta1().Workspaces().Delete(t.Ctx(), workspace.Name, metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	})
	t.Expect(err).NotTo(gomega.HaveOccurred())
}

func HasImportedAPIs(t Test, workspace *tenancyv1beta1.Workspace, gvks ...schema.GroupVersionKind) func(g gomega.Gomega) bool {
	return func(g gomega.Gomega) bool {
		// Get the logical cluster for the workspace
		logicalCluster := logicalcluster.From(workspace).Join(workspace.Name)
		discovery := t.Client().Core().Cluster(logicalCluster).Discovery()

	loop:
		for _, gvk := range gvks {
			resources, err := discovery.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
			if err != nil {
				if errors.IsNotFound(err) {
					return false
				}
				g.Expect(err).NotTo(gomega.HaveOccurred())
			}
			for _, resource := range resources.APIResources {
				if resource.Kind == gvk.Kind {
					continue loop
				}
			}
			return false
		}

		return true
	}
}
