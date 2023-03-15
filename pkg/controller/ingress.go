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

package controller

import (
	"context"
	"net/url"
	"reflect"

	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	networkingv1ac "k8s.io/client-go/applyconfigurations/networking/v1"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/kontext"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kcp-dev/logicalcluster/v3"

	"github.com/apache/camel-k/pkg/util/log"
	"github.com/apache/camel-k/pkg/util/monitoring"

	"github.com/apache/camel-kcp/pkg/client"
	"github.com/apache/camel-kcp/pkg/config"
)

func AddKaotoIngressController(mgr manager.Manager, c client.Client, cfg *config.ServiceConfiguration) error {
	return builder.ControllerManagedBy(mgr).
		Named("kaoto-ingress-controller").
		For(&networkingv1.Ingress{}, builder.WithPredicates(
			predicate.Funcs{
				UpdateFunc: func(e event.UpdateEvent) bool {
					previous, ok := e.ObjectOld.(*networkingv1.Ingress)
					if !ok {
						return false
					}
					ingress, ok := e.ObjectNew.(*networkingv1.Ingress)
					if !ok {
						return false
					}
					if ingress.Namespace != kaotoNamespaceName {
						return false
					}
					if reflect.DeepEqual(previous.Status.LoadBalancer.Ingress, ingress.Status.LoadBalancer.Ingress) {
						return false
					}
					return true
				},
			}),
		).
		Complete(monitoring.NewInstrumentedReconciler(
			&kaotoIngressReconciler{
				reconciler{
					cfg:      cfg,
					client:   c,
					recorder: mgr.GetEventRecorderFor("kaoto-ingress-controller"),
				},
			},
			schema.GroupVersionKind{
				Group:   networkingv1.SchemeGroupVersion.Group,
				Version: networkingv1.SchemeGroupVersion.Version,
				Kind:    "Ingress",
			},
		))
}

type kaotoIngressReconciler struct {
	reconciler
}

func (r *kaotoIngressReconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	rlog := log.Log.WithName("controller").WithName("kaoto-ingress").WithValues("request-name", request.Name)
	rlog.Info("Reconciling Ingress")

	// Add the logical cluster to the context
	ctx = kontext.WithCluster(ctx, logicalcluster.Name(request.ClusterName))

	ingress, err := r.client.NetworkingV1().Ingresses(request.Namespace).Get(ctx, request.Name, metav1.GetOptions{})
	if err != nil && !errors.IsNotFound(err) {
		return reconcile.Result{}, err
	}
	if len(ingress.Status.LoadBalancer.Ingress) == 0 {
		return reconcile.Result{}, nil
	}

	endpoint := url.URL{
		Scheme: "http",
		Host:   ingress.Status.LoadBalancer.Ingress[0].IP,
		Path:   request.ClusterName + "/kaoto",
	}

	ingressConfig := networkingv1ac.Ingress(request.Name, request.Namespace).
		WithAnnotations(map[string]string{
			"kaoto.io/ingress": endpoint.String(),
		})

	_, err = r.client.NetworkingV1().Ingresses(kaotoNamespaceName).
		Apply(ctx, ingressConfig, metav1.ApplyOptions{FieldManager: applyManager, Force: true})
	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}
