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
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1ac "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1ac "k8s.io/client-go/applyconfigurations/core/v1"
	metav1ac "k8s.io/client-go/applyconfigurations/meta/v1"
	networkingv1ac "k8s.io/client-go/applyconfigurations/networking/v1"
	rbacv1ac "k8s.io/client-go/applyconfigurations/rbac/v1"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	camelv1 "github.com/apache/camel-k/pkg/apis/camel/v1"
)

func (r *reconciler) applyKaotoResources(ctx context.Context, request reconcile.Request, camelNamespaceName string) error {
	serviceAccount := corev1ac.ServiceAccount("kaoto", kaotoNamespaceName)
	_, err := r.client.CoreV1().ServiceAccounts(kaotoNamespaceName).
		Apply(ctx, serviceAccount, metav1.ApplyOptions{FieldManager: applyManager, Force: true})
	if err != nil {
		return err
	}

	clusterRole := rbacv1ac.ClusterRole("kaoto").WithRules(
		rbacv1ac.PolicyRule().
			WithAPIGroups(camelv1.SchemeGroupVersion.Group).
			WithResources("integrations", "kameletbindings", "kamelets").
			WithVerbs("create", "get", "list", "patch", "update", "watch"),
		rbacv1ac.PolicyRule().
			WithAPIGroups("").
			WithResources("pods").
			WithVerbs("get", "list", "watch"),
	)
	_, err = r.client.RbacV1().ClusterRoles().
		Apply(ctx, clusterRole, metav1.ApplyOptions{FieldManager: applyManager, Force: true})
	if err != nil {
		return err
	}

	clusterRoleBinding := rbacv1ac.ClusterRoleBinding("kaoto").
		WithSubjects(rbacv1ac.Subject().
			WithKind(rbacv1.ServiceAccountKind).
			WithNamespace(kaotoNamespaceName).
			WithName("kaoto")).
		WithRoleRef(rbacv1ac.RoleRef().
			WithAPIGroup(rbacv1.GroupName).
			WithKind("ClusterRole").
			WithName("kaoto"))
	_, err = r.client.RbacV1().ClusterRoleBindings().
		Apply(ctx, clusterRoleBinding, metav1.ApplyOptions{FieldManager: applyManager, Force: true})
	if err != nil {
		return err
	}

	deploymentKaotoUI := appsv1ac.Deployment("kaoto-ui", kaotoNamespaceName).
		WithSpec(appsv1ac.DeploymentSpec().
			WithReplicas(1).
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(map[string]string{"app": "kaoto-ui"})).
			WithTemplate(corev1ac.PodTemplateSpec().WithLabels(map[string]string{"app": "kaoto-ui"}).
				WithSpec(corev1ac.PodSpec().WithContainers(
					corev1ac.Container().
						WithName("kaoto-ui").
						WithImage("ghcr.io/astefanutti/kaoto-ui:latest").
						WithPorts(corev1ac.ContainerPort().
							WithName("http").
							WithContainerPort(8080).
							WithProtocol(corev1.ProtocolTCP)).
						WithTerminationMessagePolicy(corev1.TerminationMessageReadFile).
						WithTerminationMessagePath(corev1.TerminationMessagePathDefault)).
					WithRestartPolicy(corev1.RestartPolicyAlways))))
	_, err = r.client.AppsV1().Deployments(kaotoNamespaceName).
		Apply(ctx, deploymentKaotoUI, metav1.ApplyOptions{FieldManager: applyManager, Force: true})
	if err != nil {
		return err
	}

	deploymentKaotoBackend := appsv1ac.Deployment("kaoto-backend", kaotoNamespaceName).
		WithSpec(appsv1ac.DeploymentSpec().
			WithReplicas(1).
			WithSelector(metav1ac.LabelSelector().WithMatchLabels(map[string]string{"app": "kaoto-backend"})).
			WithTemplate(corev1ac.PodTemplateSpec().WithLabels(map[string]string{"app": "kaoto-backend"}).
				WithSpec(corev1ac.PodSpec().WithContainers(
					corev1ac.Container().
						WithName("kaoto-backend").
						WithImage("ghcr.io/astefanutti/kaoto-backend:latest").
						WithPorts(corev1ac.ContainerPort().
							WithName("http").
							WithContainerPort(8081).
							WithProtocol(corev1.ProtocolTCP)).
						WithEnv(corev1ac.EnvVar().WithName("CATALOG_NAMESPACE").WithValue(camelNamespaceName)).
						WithTerminationMessagePolicy(corev1.TerminationMessageReadFile).
						WithTerminationMessagePath(corev1.TerminationMessagePathDefault)).
					WithRestartPolicy(corev1.RestartPolicyAlways).
					WithServiceAccountName("kaoto"))))
	_, err = r.client.AppsV1().Deployments(kaotoNamespaceName).
		Apply(ctx, deploymentKaotoBackend, metav1.ApplyOptions{FieldManager: applyManager, Force: true})
	if err != nil {
		return err
	}

	serviceKaotoUI := corev1ac.Service("kaoto-ui", kaotoNamespaceName).WithSpec(corev1ac.ServiceSpec().
		WithPorts(corev1ac.ServicePort().
			WithName("http").
			WithProtocol(corev1.ProtocolTCP).
			WithPort(80).
			WithTargetPort(intstr.FromString("http"))).
		WithSelector(map[string]string{"app": "kaoto-ui"}).
		WithSessionAffinity(corev1.ServiceAffinityNone).
		WithPublishNotReadyAddresses(true))
	_, err = r.client.CoreV1().Services(kaotoNamespaceName).
		Apply(ctx, serviceKaotoUI, metav1.ApplyOptions{FieldManager: applyManager, Force: true})
	if err != nil {
		return err
	}

	serviceKaotoBackend := corev1ac.Service("kaoto-backend-svc", kaotoNamespaceName).WithSpec(corev1ac.ServiceSpec().
		WithPorts(corev1ac.ServicePort().
			WithName("http").
			WithProtocol(corev1.ProtocolTCP).
			WithPort(8081).
			WithTargetPort(intstr.FromString("http"))).
		WithSelector(map[string]string{"app": "kaoto-backend"}).
		WithSessionAffinity(corev1.ServiceAffinityNone).
		WithPublishNotReadyAddresses(true))
	_, err = r.client.CoreV1().Services(kaotoNamespaceName).
		Apply(ctx, serviceKaotoBackend, metav1.ApplyOptions{FieldManager: applyManager, Force: true})
	if err != nil {
		return err
	}

	ingress := networkingv1ac.Ingress("kaoto", kaotoNamespaceName).
		WithAnnotations(map[string]string{
			"nginx.ingress.kubernetes.io/use-regex":      "true",
			"nginx.ingress.kubernetes.io/rewrite-target": "/$2",
		}).
		WithSpec(networkingv1ac.IngressSpec().
			WithRules(networkingv1ac.IngressRule().
				WithHTTP(networkingv1ac.HTTPIngressRuleValue().
					WithPaths(networkingv1ac.HTTPIngressPath().
						WithPath("/" + request.ClusterName + "/kaoto(/|$)(.*)").
						WithPathType(networkingv1.PathTypePrefix).
						WithBackend(networkingv1ac.IngressBackend().
							WithService(networkingv1ac.IngressServiceBackend().
								WithName("kaoto-ui").
								WithPort(networkingv1ac.ServiceBackendPort().
									WithName("http"))))))))
	_, err = r.client.NetworkingV1().Ingresses(kaotoNamespaceName).
		Apply(ctx, ingress, metav1.ApplyOptions{FieldManager: applyManager, Force: true})
	if err != nil {
		return err
	}

	return nil
}
