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

// +kubebuilder:object:generate=true

package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cfg "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"

	schedulingv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/scheduling/v1alpha1"

	v1 "github.com/apache/camel-k/pkg/apis/camel/v1"
)

// +kubebuilder:object:root=true

type ServiceConfiguration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The default controller runtime configuration.
	cfg.ControllerManagerConfigurationSpec `json:",inline"`

	// The service configuration specification.
	Service ServiceConfigurationSpec `json:"service,omitempty"`
}

type ServiceConfigurationSpec struct {
	// The APIExports used to configure the controller managers.
	APIExports APIExports `json:"apiExports,omitempty"`
}

type APIExports struct {
	// The Camel K APIExport used to configure the Camel K manager.
	// nolint:tagliatelle
	CamelK CamelKAPIExport `json:"camel-k,omitempty"`
	// The Kaoto APIExport used to configure the Kaoto manager.
	Kaoto KaotoAPIExport `json:"kaoto,omitempty"`
}

type CamelKAPIExport struct {
	// The reference to the Camel K APIExport.
	LocalAPIExportReference `json:",inline,omitempty"`

	// The desired state of the consumer workspace when the
	// Camel K APIExport is bound into it.
	OnAPIBinding OnAPIBinding `json:"onApiBinding,omitempty"`
}

type KaotoAPIExport struct {
	// The reference to the Kaoto APIExport.
	LocalAPIExportReference `json:",inline,omitempty"`
}

type OnAPIBinding struct {
	// The specification of the default integration platform,
	// that's created when the Camel K APIExport is bound,
	// in the consumer workspace.
	// +optional
	DefaultPlatform *IntegrationPlatform `json:"createDefaultPlatform,omitempty"`

	// The specification of the default placement, that's created
	// when the service APIExport is bound, in the consumer workspace.
	// +optional
	DefaultPlacement *Placement `json:"createDefaultPlacement,omitempty"`
}

type IntegrationPlatform struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              v1.IntegrationPlatformSpec `json:"spec,omitempty"`
}

type Placement struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              schedulingv1alpha1.PlacementSpec `json:"spec,omitempty"`
}

// LocalAPIExportReference provides the name necessary to resolve an APIExport
// relative to the local workspace.
type LocalAPIExportReference struct {
	// APIExportName is the name of the APIExport.
	//
	// +required
	// +kubebuilder:validation:Required
	// +kube:validation:MinLength=1
	APIExportName string `json:"apiExportName"`
}
