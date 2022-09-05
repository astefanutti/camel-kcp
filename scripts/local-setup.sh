#!/bin/bash

# Licensed to the Apache Software Foundation (ASF) under one or more
# contributor license agreements.  See the NOTICE file distributed with
# this work for additional information regarding copyright ownership.
# The ASF licenses this file to You under the Apache License, Version 2.0
# (the "License"); you may not use this file except in compliance with
# the License.  You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

LOCAL_SETUP_DIR="$(dirname "${BASH_SOURCE[0]}")"
source "${LOCAL_SETUP_DIR}"/.setupEnv

usage() { echo "usage: ./local-setup.sh -c <number of clusters>" 1>&2; exit 1; }
while getopts ":c:" arg; do
  case "${arg}" in
    c)
      NUM_CLUSTERS=${OPTARG}
      ;;
    *)
      usage
      ;;
  esac
done
shift $((OPTIND-1))

source "${LOCAL_SETUP_DIR}"/.startUtils

if [ -z "${NUM_CLUSTERS}" ]; then
    usage
fi

set -e pipefail

trap cleanup EXIT 1 2 3 6 15

cleanup() {
  echo "Killing kcp"
  kill "$KCP_PID"
}

TEMP_DIR="./tmp"
KCP_LOG_FILE="${TEMP_DIR}"/kcp.log

KIND_CLUSTER_PREFIX="kcp-cluster-"
KCP_CONTROL_CLUSTER_NAME="${KIND_CLUSTER_PREFIX}control"
ORG_WORKSPACE=root:camel-k

: ${KCP_VERSION:="release-0.8"}
KCP_SYNCER_IMAGE="ghcr.io/kcp-dev/kcp/syncer:${KCP_VERSION}"

for ((i=1;i<=NUM_CLUSTERS;i++))
do
	CLUSTERS="${CLUSTERS}${KIND_CLUSTER_PREFIX}${i} "
done

mkdir -p ${TEMP_DIR}

createCluster() {
  cluster=$1;
  port80=$2;
  port443=$3;
  cat <<EOF | ${KIND_BIN} create cluster --name "${cluster}" --kubeconfig ${TEMP_DIR}/"${cluster}".kubeconfig --config=-
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.24.0@sha256:0866296e693efe1fed79d5e6c7af8df71fc73ae45e3679af05342239cdc5bc8e
  kubeadmConfigPatches:
  - |
    kind: InitConfiguration
    nodeRegistration:
      kubeletExtraArgs:
        node-labels: "ingress-ready=true"
  extraPortMappings:
  - containerPort: 80
    hostPort: ${port80}
    protocol: TCP
  - containerPort: 443
    hostPort: ${port443}
    protocol: TCP
EOF
}

createSyncTarget() {
  [[ -n "$4" ]] && target=${4} || target=${1}
  createCluster $1 $2 $3

  kubectl create namespace kcp-syncer --dry-run=client -o yaml | kubectl apply -f -
  ${KUBECTL_KCP_BIN} workload sync "${target}" --kcp-namespace kcp-syncer --syncer-image=${KCP_SYNCER_IMAGE} --output-file ${TEMP_DIR}/"${target}"-syncer.yaml

  echo "Deploying kcp syncer to ${1}"
  kubectl --kubeconfig ${TEMP_DIR}/"${1}".kubeconfig apply -f ${TEMP_DIR}/"${target}"-syncer.yaml
}

# Delete existing KinD clusters
clusterCount=$(${KIND_BIN} get clusters | grep ${KIND_CLUSTER_PREFIX} | wc -l)
if ! [[ $clusterCount =~ "0" ]] ; then
  echo "Deleting previous KinD clusters."
  ${KIND_BIN} get clusters | grep ${KIND_CLUSTER_PREFIX} | xargs "${KIND_BIN}" delete clusters
fi

# Start kcp
echo "Starting kcp, writing logs to ${KCP_LOG_FILE}"
${KCP_BIN} --v=9 start --run-controllers > ${KCP_LOG_FILE} 2>&1 &
KCP_PID=$!

if ! ps -p ${KCP_PID}; then
  echo "####"
  echo "---> kcp failed to start, see ${KCP_LOG_FILE} for info."
  echo "####"
  exit 1 #this will trigger cleanup function
fi

echo "Waiting for kcp server to be ready..."
wait_for "grep 'Bootstrapped ClusterWorkspaceShard root|root' ${KCP_LOG_FILE}" "kcp" "1m" "5"
sleep 5

${KUBECTL_KCP_BIN} workspace use "root"
${KUBECTL_KCP_BIN} workspace create "camel-k" --type universal --enter || ${KUBECTL_KCP_BIN} workspace use "camel-k"

# Create control plane sync target and wait for it to be ready
${KUBECTL_KCP_BIN} workspace use "${ORG_WORKSPACE}"
${KUBECTL_KCP_BIN} workspace create "camel-kcp" --enter || ${KUBECTL_KCP_BIN} workspace use "camel-kcp"
echo "Creating kcp SyncTarget control cluster"
createSyncTarget $KCP_CONTROL_CLUSTER_NAME 8081 8444 "control"
kubectl wait --timeout=300s --for=condition=Ready=true synctargets "control"

# Create data plane sync targets and wait for them to be ready
echo "Creating $NUM_CLUSTERS kcp SyncTarget cluster(s)"
port80=8082
port443=8445
for cluster in $CLUSTERS; do
  createSyncTarget "$cluster" $port80 $port443

  echo "Deploying Ingress controller to ${cluster}"
  kubeconfig=${TEMP_DIR}/"${cluster}".kubeconfig
  VERSION=controller-v1.2.1
  curl https://raw.githubusercontent.com/kubernetes/ingress-nginx/"${VERSION}"/deploy/static/provider/kind/deploy.yaml | sed "s/--publish-status-address=localhost/--report-node-internal-ip-address/g" | kubectl --kubeconfig "${kubeconfig}" apply -f -
  kubectl --kubeconfig "${kubeconfig}" annotate ingressclass nginx "ingressclass.kubernetes.io/is-default-class=true"
  echo "Waiting for deployments to be ready ..."
  kubectl --kubeconfig "${kubeconfig}" -n ingress-nginx wait --timeout=300s --for=condition=Available deployments --all

  port80=$((port80 + 1))
  port443=$((port443 + 1))
done
kubectl wait --timeout=300s --for=condition=Ready=true synctargets ${CLUSTERS}

# Install APIs
identityHash=$(kubectl get apiexport kubernetes -o json | jq -r .status.identityHash)

# Install APIExport
sed -e "s/IDENTITY_HASH/$identityHash/" config/kcp/identity-hash-patch.yaml > config/kcp/add-identity-hash.yaml
${KUSTOMIZE_BIN} build config/kcp | kubectl apply --server-side -f -

# Switch to data workspace
${KUBECTL_KCP_BIN} workspace use "${ORG_WORKSPACE}"
${KUBECTL_KCP_BIN} workspace create "demo" --enter || ${KUBECTL_KCP_BIN} workspace use "demo"

# Install APIBinding(s)
sed -e "s/IDENTITY_HASH/$identityHash/" config/demo/identity-hash-patch.yaml > config/demo/add-identity-hash.yaml
${KUSTOMIZE_BIN} build config/demo | kubectl apply --server-side -f -

echo ""
echo "KCP PID          : ${KCP_PID}"
echo ""
echo "The KinD clusters have been registered, and kcp is running, now you should run camel-kcp."
echo ""
echo "Run Option 1 (Local):"
echo ""
echo "       cd ${PWD}"
echo "       KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ./bin/camel-kcp"
echo ""
read -p "Press enter to exit -> It will kill the kcp process running in background"
