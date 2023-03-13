# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

KCP_BRANCH := v0.11.0
NUM_CLUSTERS := 1

IMAGE_NAME ?= camel-kcp
IMAGE_REGISTRY ?= ghcr.io/astefanutti
IMAGE_TAG ?= latest
IMG ?= $(IMAGE_REGISTRY)/$(IMAGE_NAME):$(IMAGE_TAG)

KUBECONFIG ?= $(shell pwd)/.kcp/admin.kubeconfig
CLUSTERS_KUBECONFIG_DIR ?= $(shell pwd)/tmp

APIEXPORT_PREFIX ?= today

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

ARCH := $(shell go env GOARCH)
OS := $(shell go env GOOS)

.PHONY: all
all: build

##@ General

.PHONY: help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: clean
clean: ## Clean up temporary files
	-rm -rf ./.kcp
	-rm -f ./bin/*
	-rm -rf ./tmp

.PHONY: manifests
manifests: controller-gen ## Generate ClusterRole objects
	$(CONTROLLER_GEN) rbac:roleName=camel-kcp paths="./cmd/..." paths="./pkg/..." output:rbac:artifacts:config=config/rbac/kcp

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./cmd/..." paths="./pkg/..."

.PHONY: apiresourceschemas
apiresourceschemas: kustomize kcp ## Convert CRDs from config/crds to APIResourceSchemas
	$(KUSTOMIZE) build config/crd | $(KUBECTL_KCP_BIN) crd snapshot -f - --prefix $(APIEXPORT_PREFIX) > config/kcp/$(APIEXPORT_PREFIX).apiresourceschemas.yaml

.PHONY: fmt
fmt: ## Run go fmt against code
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint against code
	golangci-lint run ./...

.PHONY: test
test: ## Run tests
	go test -v ./... -coverprofile=cover.out

##@ Build

.PHONY: build
build: ## Build the project
	GOOS=$(OS) GOARCH=$(ARCH) CGO_ENABLED=0 go build -o bin ./cmd/...

.PHONY: build-image
build-image: ## Build container image
	docker build -t ${IMG} .

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: apiresourceschemas kustomize ## Install APIResourceSchemas and APIExport into kcp (using $KUBECONFIG or ~/.kube/config)
	$(KUSTOMIZE) build config/kcp | kubectl apply --server-side -f -

.PHONY: uninstall
uninstall: kcp kustomize ## Uninstall APIResourceSchemas and APIExport from kcp (using $KUBECONFIG or ~/.kube/config)
	$(KUSTOMIZE) build config/kcp | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: kustomize ## Deploy controller to the K8s cluster (using $KUBECONFIG or ~/.kube/config)
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: undeploy
undeploy: kustomize ## Undeploy controller from the K8s cluster (using $KUBECONFIG or ~/.kube/config)
	$(KUSTOMIZE) build config/default | kubectl delete -f -

.PHONY: local-deploy
local-deploy: kustomize build-image ## Deploy controller to the local K8s cluster (using the local-setup.sh script)
ifeq ($(shell uname -s 2>/dev/null || echo Unknown),Darwin)
	$(eval registry_addr:=$(shell ipconfig getifaddr en0))
else
	$(eval registry_addr:=$(shell hostname -i))
endif
	docker tag ${IMG} $(registry_addr):5001/$(IMAGE_NAME)
	docker push $(registry_addr):5001/$(IMAGE_NAME)
	$(KUSTOMIZE) fn run config/deploy/local --image gcr.io/kpt-fn/apply-setters:v0.2.0 -- registry-address=$(registry_addr):5001
	$(KUSTOMIZE) build config/deploy/local | kubectl apply -f -

## Local Deployment

.PHONY: local-setup
local-setup: export KCP_VERSION=${KCP_BRANCH}
local-setup: clean kind kcp kustomize build ## Setup kcp locally with KinD clusters
	./scripts/local-setup.sh -c ${NUM_CLUSTERS}

##@ Test

.PHONY: e2e
e2e: build ## Run e2e tests
	KUBECONFIG="$(KUBECONFIG)" CLUSTERS_KUBECONFIG_DIR="$(CLUSTERS_KUBECONFIG_DIR)" \
	go test -count=1 -timeout 60m -v ./test/e2e -tags=e2e

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN): ## Ensure that the directory exists
	mkdir -p $(LOCALBIN)

## Tool Binaries
KCP ?= $(LOCALBIN)/kcp
KUBECTL_KCP_BIN ?= $(LOCALBIN)/kubectl-kcp
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
KUSTOMIZE ?= $(LOCALBIN)/kustomize
KIND ?= $(LOCALBIN)/kind

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= v0.8.0
KUSTOMIZE_VERSION ?= v4.5.4
KIND_VERSION ?= v0.17.0

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary
$(KUSTOMIZE): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION)

.PHONY: kcp
kcp: $(KCP) ## Download kcp locally if necessary
$(KCP): $(LOCALBIN)
	rm -rf ./tmp/kcp
	git clone --depth=1 --branch ${KCP_BRANCH} https://github.com/kcp-dev/kcp ./tmp/kcp
	cd ./tmp/kcp && IGNORE_GO_VERSION=1 GOWORK=off make
	cp ./tmp/kcp/bin/* $(LOCALBIN)
	rm -rf ./tmp/kcp

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary
$(KIND): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/kind@$(KIND_VERSION)

.PHONY: get-kind-version
get-kind-version:
	@echo $(KIND_VERSION)

.PHONY: get-kcp-version
get-kcp-version:
	@echo $(KCP_BRANCH)
