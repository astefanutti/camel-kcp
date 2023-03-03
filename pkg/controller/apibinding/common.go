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

package apibinding

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	schedulingv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/scheduling/v1alpha1"

	"github.com/apache/camel-k/pkg/util/log"

	"github.com/apache/camel-kcp/pkg/client"
	"github.com/apache/camel-kcp/pkg/config"
)

const applyManager = "camel-kcp"

var Log = log.Log.WithName("controller").WithName("api-binding")

type reconciler struct {
	reconcile.Reconciler
	cfg      *config.ServiceConfiguration
	client   client.Client
	recorder record.EventRecorder
}

func (r *reconciler) maybeCreateNamespace(ctx context.Context, name string) error {
	_, err := r.client.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return nil
	} else if !errors.IsNotFound(err) {
		return err
	}

	namespace := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	// Use client-go non-caching client
	_, err = r.client.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	return err
}

// +kubebuilder:rbac:groups="scheduling.kcp.io",resources=placements,verbs=get;create

func (r *reconciler) maybeCreatePlacement(ctx context.Context, placementConfig *config.Placement) error {
	placement := &schedulingv1alpha1.Placement{
		ObjectMeta: placementConfig.ObjectMeta,
		Spec:       placementConfig.Spec,
	}

	// Use client-go non-caching client
	if _, err := r.client.KcpSchedulingV1alpha1().Placements().Get(ctx, placement.Name, metav1.GetOptions{}); errors.IsNotFound(err) {
		if _, err := r.client.KcpSchedulingV1alpha1().Placements().Create(ctx, placement, metav1.CreateOptions{}); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	return nil
}
