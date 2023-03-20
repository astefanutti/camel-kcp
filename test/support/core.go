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

type PartialMetadata interface {
	SetName(name string)
	GetLabels() map[string]string
	SetLabels(labels map[string]string)
}

func WithName[T PartialMetadata](name string) Option[T] {
	return &withName[T]{name: name}
}

type withName[T PartialMetadata] struct {
	name string
}

var _ Option[PartialMetadata] = (*withName[PartialMetadata])(nil)

func (o *withName[T]) applyTo(to T) error {
	to.SetName(o.name)
	return nil
}

func WithLabel[T PartialMetadata](key, value string) Option[T] {
	return &withLabel[T]{key, value}
}

type withLabel[T PartialMetadata] struct {
	key, value string
}

var _ Option[PartialMetadata] = (*withLabel[PartialMetadata])(nil)

func (o *withLabel[T]) applyTo(object T) error {
	if object.GetLabels() == nil {
		object.SetLabels(map[string]string{})
	}
	object.GetLabels()[o.key] = o.value
	return nil
}

func WithLabels[T PartialMetadata](labels map[string]string) Option[T] {
	return &withLabels[T]{labels}
}

type withLabels[T PartialMetadata] struct {
	labels map[string]string
}

var _ Option[PartialMetadata] = (*withLabels[PartialMetadata])(nil)

func (o *withLabels[T]) applyTo(object T) error {
	object.SetLabels(o.labels)
	return nil
}
