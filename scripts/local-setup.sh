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
ORG_WORKSPACE=root:default

KUBECONFIG_KCP_ADMIN=.kcp/admin.kubeconfig

: ${KCP_VERSION:="release-0.7"}
KCP_SYNCER_IMAGE="ghcr.io/kcp-dev/kcp/syncer:${KCP_VERSION}"

for ((i=1;i<=NUM_CLUSTERS;i++))
do
	CLUSTERS="${CLUSTERS}${KIND_CLUSTER_PREFIX}${i} "
done

mkdir -p ${TEMP_DIR}

[[ -n "$KUBECONFIG" ]] && KUBECONFIG="$KUBECONFIG" || KUBECONFIG="$HOME/.kube/config"

createCluster() {
  cluster=$1;
  port80=$2;
  port443=$3;
  cat <<EOF | ${KIND_BIN} create cluster --name "${cluster}" --config=-
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

  ${KIND_BIN} get kubeconfig --name="${cluster}" > ${TEMP_DIR}/"${cluster}".kubeconfig
}

createSyncTarget() {
  [[ -n "$4" ]] && target=${4} || target=${1}
  echo "Creating SyncTarget (${target})"
  createCluster $1 $2 $3

  echo "Deploying kcp syncer to ${1}"
  KUBECONFIG=${KUBECONFIG_KCP_ADMIN} kubectl create namespace kcp-syncer --dry-run=client -o yaml | kubectl --kubeconfig=${KUBECONFIG_KCP_ADMIN} apply -f -
  KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workload sync "${target}" --kcp-namespace kcp-syncer --syncer-image=${KCP_SYNCER_IMAGE} --output-file ${TEMP_DIR}/"${target}"-syncer.yaml

  kubectl config use-context kind-"${1}"

  kubectl apply -f ${TEMP_DIR}/"${target}"-syncer.yaml
}

createDataSyncTarget() {
  createSyncTarget $1 $2 $3 $4

  echo "Deploying Ingress controller to ${1}"
  VERSION=controller-v1.2.1
  curl https://raw.githubusercontent.com/kubernetes/ingress-nginx/"${VERSION}"/deploy/static/provider/kind/deploy.yaml | sed "s/--publish-status-address=localhost/--report-node-internal-ip-address/g" | kubectl apply -f -
  kubectl annotate ingressclass nginx "ingressclass.kubernetes.io/is-default-class=true"
  echo "Waiting for deployments to be ready ..."
  kubectl -n ingress-nginx wait --timeout=300s --for=condition=Available deployments --all
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

KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace use "root"
KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace create "default" --type universal --enter

# Create control plane sync target and wait for it to be ready
KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace use "${ORG_WORKSPACE}"
KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace create "control-compute" --enter || KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace use "control-compute"
createSyncTarget $KCP_CONTROL_CLUSTER_NAME 8081 8444 "control"
KUBECONFIG=${KUBECONFIG_KCP_ADMIN} kubectl wait --timeout=300s --for=condition=Ready=true synctargets "control"

# Create data plane sync target clusters and wait for them to be ready
KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace use "${ORG_WORKSPACE}"
KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace create "data-compute" --enter || KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace use "data-compute"
echo "Creating $NUM_CLUSTERS kcp SyncTarget cluster(s)"
port80=8082
port443=8445
for cluster in $CLUSTERS; do
  createDataSyncTarget "$cluster" $port80 $port443
  port80=$((port80 + 1))
  port443=$((port443 + 1))
done
KUBECONFIG=${KUBECONFIG_KCP_ADMIN} kubectl wait --timeout=300s --for=condition=Ready=true synctargets ${CLUSTERS}

# Switch to data workspace
KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace use "${ORG_WORKSPACE}"
KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace create "data" --enter || KUBECONFIG=${KUBECONFIG_KCP_ADMIN} ${KUBECTL_KCP_BIN} workspace use "data"

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
