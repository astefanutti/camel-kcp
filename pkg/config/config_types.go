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

package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	cfg "sigs.k8s.io/controller-runtime/pkg/config/v1alpha1"
)

// +kubebuilder:object:root=true

type ControllerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// The default controller runtime configuration.
	cfg.ControllerManagerConfigurationSpec `json:",inline"`

	// The name of the APIExport, in the controller workspace,
	// whose virtual workspace URL is used to configure the
	// manager client.
	APIExportName string `json:"apiExportName,omitempty"`
}
