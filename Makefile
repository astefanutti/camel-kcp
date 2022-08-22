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

KCP_BRANCH := release-0.7
NUM_CLUSTERS := 2

IMAGE_TAG_BASE ?= quay.io/astefanutti/camel-kcp
IMAGE_TAG ?= latest
IMG ?= $(IMAGE_TAG_BASE):$(IMAGE_TAG)

KUBECONFIG ?= $(shell pwd)/.kcp/admin.kubeconfig
CLUSTERS_KUBECONFIG_DIR ?= $(shell pwd)/tmp

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DELETE_ON_ERROR:
MAKEFLAGS += --warn-undefined-variables
MAKEFLAGS += --no-builtin-rules

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
	$(CONTROLLER_GEN) rbac:roleName=camel-kcp paths="./..." output:rbac:artifacts:config=config/rbac

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
	go build -o bin ./cmd/...

.PHONY: docker-build
docker-build: ## Build docker image
	docker build -t ${IMG} .

##@ Deployment

.PHONY: install
install: kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: kustomize generate-ld-config ## Deploy controller to the K8s cluster specified in ~/.kube/config
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/deploy/local | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config
	$(KUSTOMIZE) build config/deploy/local | kubectl delete -f -

## Local Deployment
LD_DIR=config/deploy/local
LD_CONTROLLER_CONFIG_ENV=$(LD_DIR)/controller-config.env

$(LD_CONTROLLER_CONFIG_ENV):
	envsubst \
		< $(LD_CONTROLLER_CONFIG_ENV).template \
		> $(LD_CONTROLLER_CONFIG_ENV)

.PHONY: generate-ld-config
generate-ld-config: $(LD_CONTROLLER_CONFIG_ENV) ## Generate local deployment files

.PHONY: clean-ld-env
clean-ld-env:
	-rm -f $(LD_CONTROLLER_CONFIG_ENV)

.PHONY: clean-ld-config
clean-ld-config: clean-ld-env ## Remove local deployment files

.PHONY: local-setup
local-setup: export KCP_VERSION=${KCP_BRANCH}
local-setup: clean kind kcp kustomize build ## Setup kcp locally with KinD clusters
	./scripts/local-setup.sh -c ${NUM_CLUSTERS}

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN): ## Ensure that the directory exists
	mkdir -p $(LOCALBIN)

## Tool Binaries
KCP ?= $(LOCALBIN)/kcp
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
KUSTOMIZE ?= $(LOCALBIN)/kustomize
KIND ?= $(LOCALBIN)/kind

## Tool Versions
CONTROLLER_TOOLS_VERSION ?= v0.8.0
KUSTOMIZE_VERSION ?= v4.5.4
KIND_VERSION ?= v0.14.0

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary
$(CONTROLLER_GEN):
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary
$(KUSTOMIZE):
	curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

.PHONY: kcp
kcp: $(KCP) ## Download kcp locally if necessary
$(KCP):
	rm -rf ./tmp/kcp
	git clone --depth=1 --branch ${KCP_BRANCH} https://github.com/kcp-dev/kcp ./tmp/kcp
	cd ./tmp/kcp && make
	cp ./tmp/kcp/bin/* $(LOCALBIN)
	rm -rf ./tmp/kcp

.PHONY: kind
kind: $(KIND) ## Download kind locally if necessary
$(KIND):
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/kind@$(KIND_VERSION)
