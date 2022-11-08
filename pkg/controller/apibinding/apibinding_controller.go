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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"

	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrl "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/kcp-dev/logicalcluster/v2"

	apisv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/apis/v1alpha1"

	v1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	"github.com/apache/camel-k/pkg/client"
	"github.com/apache/camel-k/pkg/util/monitoring"

	"github.com/apache/camel-kcp/pkg/platform"
)

func Add(mgr manager.Manager, c client.Client) error {
	return add(mgr, newReconciler(mgr, c))
}

func newReconciler(mgr manager.Manager, c client.Client) reconcile.Reconciler {
	return monitoring.NewInstrumentedReconciler(
		&reconciler{
			client:   c,
			reader:   mgr.GetAPIReader(),
			scheme:   mgr.GetScheme(),
			recorder: mgr.GetEventRecorderFor("camel-kcp-apibinding-controller"),
		},
		schema.GroupVersionKind{
			Group:   apisv1alpha1.SchemeGroupVersion.Group,
			Version: apisv1alpha1.SchemeGroupVersion.Version,
			Kind:    "APIBinding",
		},
	)
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	return builder.ControllerManagedBy(mgr).
		Named("apibinding-controller").
		// Watch for changes to primary resource APIBinding
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
		Complete(r)
}

var _ reconcile.Reconciler = &reconciler{}

type reconciler struct {
	// Split client that reads objects from the cache and writes to the API server.
	client client.Client
	// Non-caching client to be used whenever caching may cause race conditions,
	// like in the builds scheduling critical section.
	reader   ctrl.Reader
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

func (r *reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	rlog := Log.WithValues("request-name", request.Name)
	rlog.Info("Reconciling APIBinding")

	// Add the logical cluster to the context
	ctx = logicalcluster.WithCluster(ctx, logicalcluster.New(request.ClusterName))

	namespaceName := platform.GetOperatorNamespace()
	if namespaceName == "" {
		namespaceName = platform.DefaultNamespaceName
	}

	if err := r.maybeCreateNamespace(ctx, namespaceName); err != nil {
		return reconcile.Result{}, err
	}

	ip := v1.NewIntegrationPlatform(namespaceName, platform.DefaultPlatformName)

	if err := r.createOrPatchPlatform(ctx, &ip); err != nil {
		return reconcile.Result{}, err
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

func (r *reconciler) createOrPatchPlatform(ctx context.Context, ip *v1.IntegrationPlatform) error {
	if _, err := ctrlutil.CreateOrPatch(ctx, r.client, ip, func() error {
		emptyRegistry := v1.RegistrySpec{}
		if ip.Spec.Build.Registry == emptyRegistry {
			// FIXME: configuration
			ip.Spec.Build.Registry = v1.RegistrySpec{
				Address:  "192.168.0.24:5001",
				Insecure: true,
			}
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}
