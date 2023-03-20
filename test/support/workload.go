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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"sort"

	"github.com/onsi/gomega"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	workloadplugin "github.com/kcp-dev/kcp/pkg/cliplugins/workload/plugin"
	"github.com/kcp-dev/logicalcluster/v3"

	workloadv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/workload/v1alpha1"
)

type SyncTargetConfig struct {
	name           string
	labels         map[string]string
	kubeConfigPath string
	workspace      syncTargetWorkspace
	syncer         syncer
}

var _ Option[*SyncTargetConfig] = (*withLabel[*SyncTargetConfig])(nil)

func (s *SyncTargetConfig) SetName(name string) {
	s.name = name
}

func (s *SyncTargetConfig) GetLabels() map[string]string {
	return s.labels
}

func (s *SyncTargetConfig) SetLabels(labels map[string]string) {
	s.labels = labels
}

type syncTargetWorkspace struct {
	path logicalcluster.Path
	url  string
}

var WithKubeConfigByName = &withKubeConfigByName{}

type withKubeConfigByName struct{}

var _ Option[*SyncTargetConfig] = (*withKubeConfigByName)(nil)

func WithKubeConfigByID(id string) Option[*SyncTargetConfig] {
	return &withKubeConfigByID{id}
}

type withKubeConfigByID struct {
	ID string
}

var _ Option[*SyncTargetConfig] = (*withKubeConfigByName)(nil)

func (o *withKubeConfigByName) applyTo(config *SyncTargetConfig) error {
	return WithKubeConfigByID(config.name).applyTo(config)
}

func (o *withKubeConfigByID) applyTo(config *SyncTargetConfig) error {
	dir := os.Getenv(clustersKubeConfigDir)
	if dir == "" {
		return fmt.Errorf("%s environment variable is not set", clustersKubeConfigDir)
	}

	config.kubeConfigPath = path.Join(dir, o.ID+".kubeconfig")

	return nil
}

func createSyncTarget(t Test, name string, options ...Option[*SyncTargetConfig]) (*workloadv1alpha1.SyncTarget, func() error) {
	config := &SyncTargetConfig{
		name: name,
	}

	for _, option := range options {
		t.Expect(option.applyTo(config)).To(gomega.Succeed())
	}

	t.Expect(config.workspace.path.Empty()).NotTo(gomega.BeTrue())
	t.Expect(config.workspace.url).NotTo(gomega.BeEmpty())
	t.Expect(config.kubeConfigPath).NotTo(gomega.BeEmpty())

	// Run the KCP workload plugin sync command
	cleanup, err := applyKcpWorkloadSync(t, config)
	t.Expect(err).NotTo(gomega.HaveOccurred())

	// Get the workload cluster and return it
	syncTarget, err := t.Client().Kcp().Cluster(config.workspace.path).WorkloadV1alpha1().SyncTargets().Get(t.Ctx(), name, metav1.GetOptions{})
	t.Expect(err).NotTo(gomega.HaveOccurred())

	return syncTarget, cleanup
}

func deleteSyncTarget(t Test, syncTarget *workloadv1alpha1.SyncTarget) {
	propagationPolicy := metav1.DeletePropagationBackground
	err := t.Client().Kcp().Cluster(logicalcluster.From(syncTarget).Path()).WorkloadV1alpha1().SyncTargets().Delete(t.Ctx(), syncTarget.Name, metav1.DeleteOptions{PropagationPolicy: &propagationPolicy})
	t.Expect(err).NotTo(gomega.HaveOccurred())
}

func applyKcpWorkloadSync(t Test, config *SyncTargetConfig) (func() error, error) {
	cleanup := func() error { return nil }

	// Configure workload plugin kubeconfig for test workspace
	// clusterServer := t.Client().GetConfig().Host + config.workspace.Path()
	syncCommandOutput := new(bytes.Buffer)
	syncOptions := workloadplugin.NewSyncOptions(genericclioptions.IOStreams{In: os.Stdin, Out: syncCommandOutput, ErrOut: os.Stderr})
	syncOptions.KubectlOverrides.ClusterInfo.Server = config.workspace.url
	// syncOptions.ResourcesToSync = []string{"ingresses.networking.k8s.io", "services"}
	// syncOptions.SyncTargetName = config.name
	syncOptions.OutputFile = "-"
	syncOptions.SyncTargetLabels = labelsAsKeyValuePairs(config.labels)
	syncOptions.KCPNamespace = config.syncer.namespace
	syncOptions.SyncerImage = config.syncer.image
	syncOptions.Replicas = config.syncer.replicas

	err := syncOptions.Complete([]string{config.name})
	if err != nil {
		return cleanup, err
	}

	err = syncOptions.Run(t.Ctx())
	if err != nil {
		return cleanup, err
	}

	// Apply the syncer resources to the workload cluster
	clientConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: config.kubeConfigPath},
		&clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return cleanup, err
	}

	client, err := dynamic.NewForConfig(clientConfig)
	if err != nil {
		return cleanup, err
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(clientConfig)
	if err != nil {
		return cleanup, err
	}
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))

	decoder := yaml.NewYAMLToJSONDecoder(bytes.NewReader(syncCommandOutput.Bytes()))

	var resources []*unstructured.Unstructured

	cleanup = func() error {
		errs := make([]error, 0)
		// Iterate over the resources in reverse order
		for i := len(resources) - 1; i >= 0; i-- {
			resource := resources[i]
			mapping, err := restMapper.RESTMapping(resource.GroupVersionKind().GroupKind(), resource.GroupVersionKind().Version)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			propagationPolicy := metav1.DeletePropagationForeground
			err = client.Resource(mapping.Resource).Namespace(resource.GetNamespace()).Delete(t.Ctx(), resource.GetName(), metav1.DeleteOptions{PropagationPolicy: &propagationPolicy})
			if err != nil && !apierrors.IsNotFound(err) {
				errs = append(errs, err)
			}
		}
		return errors.NewAggregate(errs)
	}

	for {
		resource := &unstructured.Unstructured{}
		err := decoder.Decode(resource)
		if err == io.EOF {
			break
		}
		if err != nil {
			return cleanup, err
		}
		mapping, err := restMapper.RESTMapping(resource.GroupVersionKind().GroupKind(), resource.GroupVersionKind().Version)
		if err != nil {
			return cleanup, err
		}
		data, err := json.Marshal(resource)
		if err != nil {
			return cleanup, err
		}
		_, err = client.Resource(mapping.Resource).Namespace(resource.GetNamespace()).Patch(t.Ctx(), resource.GetName(), types.ApplyPatchType, data, ApplyOptions.ToPatchOptions())
		if err != nil {
			return cleanup, err
		}
		resources = append(resources, resource)
	}

	return cleanup, nil
}

func labelsAsKeyValuePairs(labels map[string]string) []string {
	pairs := make([]string, 0, len(labels))
	for key, value := range labels {
		pairs = append(pairs, key+"="+value)
	}
	// Sort for determinism
	sort.StringSlice(pairs).Sort()
	return pairs
}
