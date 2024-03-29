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
	corev1 "k8s.io/api/core/v1"

	conditionsapi "github.com/kcp-dev/kcp/pkg/apis/third_party/conditions/apis/conditions/v1alpha1"
	conditionsutil "github.com/kcp-dev/kcp/pkg/apis/third_party/conditions/util/conditions"

	camelv1 "github.com/apache/camel-k/pkg/apis/camel/v1"
	camelv1alpha1 "github.com/apache/camel-k/pkg/apis/camel/v1alpha1"
)

type conditionType interface {
	~string
}

func ConditionStatus[T conditionType](conditionType T) func(any) corev1.ConditionStatus {
	return func(object any) corev1.ConditionStatus {
		switch o := object.(type) {
		case conditionsutil.Getter:
			if c := conditionsutil.Get(o, conditionsapi.ConditionType(conditionType)); c != nil {
				return c.Status
			}

		case *camelv1.Integration:
			if c := o.Status.GetCondition(camelv1.IntegrationConditionType(conditionType)); c != nil {
				return c.Status
			}

		case *camelv1alpha1.KameletBinding:
			if c := o.Status.GetCondition(camelv1alpha1.KameletBindingConditionType(conditionType)); c != nil {
				return c.Status
			}
		}

		return corev1.ConditionUnknown
	}
}
