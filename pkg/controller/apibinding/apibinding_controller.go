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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kcp-dev/logicalcluster/v2"

	apisv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"

	v1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	"github.com/apache/camel-k/pkg/util/monitoring"

	"github.com/apache/camel-kcp/pkg/client"
	"github.com/apache/camel-kcp/pkg/config"
	"github.com/apache/camel-kcp/pkg/platform"
)

func Add(mgr manager.Manager, c client.Client, cfg *config.ServiceConfiguration) error {
	return builder.ControllerManagedBy(mgr).
		Named("apibinding-controller").
		For(&apisv1alpha1.APIBinding{}, builder.WithPredicates(
			predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					binding, ok := e.ObjectNew.(*apisv1alpha1.APIBinding)
					if !ok {
						return false
					}
					return binding.Status.Phase == apisv1alpha1.APIBindingPhaseBound
				},
			})).
		Complete(monitoring.NewInstrumentedReconciler(
			&reconciler{
				cfg:      cfg,
				client:   c,
				recorder: mgr.GetEventRecorderFor("camel-kcp-apibinding-controller"),
			},
			schema.GroupVersionKind{
				Group:   apisv1alpha1.SchemeGroupVersion.Group,
				Version: apisv1alpha1.SchemeGroupVersion.Version,
				Kind:    "APIBinding",
			},
		))
}

var _ reconcile.Reconciler = &reconciler{}

type reconciler struct {
	cfg      *config.ServiceConfiguration
	client   client.Client
	recorder record.EventRecorder
}

func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	rlog := Log.WithValues("request-name", request.Name)
	rlog.Info("Reconciling APIBinding")

	// Add the logical cluster to the context
	ctx = logicalcluster.WithCluster(ctx, logicalcluster.New(request.ClusterName))

	if ip := r.cfg.Service.OnAPIBinding.DefaultPlatform; ip != nil {
		if ip.Namespace == "" {
			ip.Namespace = platform.GetOperatorNamespace()
		}
		if ip.Name == "" {
			ip.Name = platform.DefaultPlatformName
		}

		if err := r.maybeCreateNamespace(ctx, ip.Namespace); err != nil {
			return reconcile.Result{}, err
		}

		if err := r.maybeCreatePlatform(ctx, ip); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
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

	_, err = r.client.CoreV1().Namespaces().Create(ctx, namespace, metav1.CreateOptions{})
	return err
}

func (r *reconciler) maybeCreatePlatform(ctx context.Context, ip *config.IntegrationPlatform) error {
	p := &v1.IntegrationPlatform{
		ObjectMeta: ip.ObjectMeta,
		Spec:       ip.Spec,
	}

	if err := r.client.Get(ctx, ctrl.ObjectKeyFromObject(p), p); errors.IsNotFound(err) {
		return r.client.Create(ctx, p)
	} else if err != nil {
		return err
	}

	return nil
}
