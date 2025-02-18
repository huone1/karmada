#!/usr/bin/env bash
set -o errexit
set -o nounset
set -o pipefail

# This script starts a local karmada control plane based on current codebase and with a certain number of clusters joined.	REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
# This script depends on utils in: ${REPO_ROOT}/hack/util.sh
# 1. used by developer to setup develop environment quickly.
# 2. used by e2e testing to setup test environment automatically.

REPO_ROOT=$(dirname "${BASH_SOURCE[0]}")/..
source "${REPO_ROOT}"/hack/util.sh

# variable define
KUBECONFIG_PATH=${KUBECONFIG_PATH:-"${HOME}/.kube"}
MAIN_KUBECONFIG=${MAIN_KUBECONFIG:-"${KUBECONFIG_PATH}/karmada.config"}
HOST_CLUSTER_NAME=${HOST_CLUSTER_NAME:-"karmada-host"}
KARMADA_APISERVER_CLUSTER_NAME=${KARMADA_APISERVER_CLUSTER_NAME:-"karmada-apiserver"}
MEMBER_CLUSTER_KUBECONFIG=${MEMBER_CLUSTER_KUBECONFIG:-"${KUBECONFIG_PATH}/members.config"}
MEMBER_CLUSTER_1_NAME=${MEMBER_CLUSTER_1_NAME:-"member1"}
MEMBER_CLUSTER_2_NAME=${MEMBER_CLUSTER_2_NAME:-"member2"}
PULL_MODE_CLUSTER_NAME=${PULL_MODE_CLUSTER_NAME:-"member3"}
CLUSTER_VERSION=${CLUSTER_VERSION:-"kindest/node:v1.19.1"}

KIND_LOG_FILE=${KIND_LOG_FILE:-"/tmp/karmada"}

#step0: prepare
# install kind and kubectl
util::install_tools sigs.k8s.io/kind v0.10.0
# we choose v1.18.0, because in kubectl after versions 1.18 exist a bug which will give wrong output when using jsonpath.
# bug details: https://github.com/kubernetes/kubernetes/pull/98057
util::install_kubectl "v1.18.0" "amd64"

#step1. create host cluster and member clusters in parallel
util::create_cluster "${HOST_CLUSTER_NAME}" "${MAIN_KUBECONFIG}" "${CLUSTER_VERSION}" "${KIND_LOG_FILE}"
util::create_cluster "${MEMBER_CLUSTER_1_NAME}" "${MEMBER_CLUSTER_KUBECONFIG}" "${CLUSTER_VERSION}" "${KIND_LOG_FILE}" "${REPO_ROOT}/artifacts/kindClusterConfig/member1.yaml"
util::create_cluster "${MEMBER_CLUSTER_2_NAME}" "${MEMBER_CLUSTER_KUBECONFIG}" "${CLUSTER_VERSION}" "${KIND_LOG_FILE}" "${REPO_ROOT}/artifacts/kindClusterConfig/member2.yaml"
util::create_cluster "${PULL_MODE_CLUSTER_NAME}" "${MEMBER_CLUSTER_KUBECONFIG}" "${CLUSTER_VERSION}" "${KIND_LOG_FILE}"

#step2. make images and get karmadactl
export VERSION="latest"
export REGISTRY="swr.ap-southeast-1.myhuaweicloud.com/karmada"
make images --directory="${REPO_ROOT}"

GO111MODULE=on go install "github.com/karmada-io/karmada/cmd/karmadactl"
GOPATH=$(go env GOPATH | awk -F ':' '{print $1}')
KARMADACTL_BIN="${GOPATH}/bin/karmadactl"

#step3. wait until the host cluster ready
echo "Waiting for the host clusters to be ready..."
util::check_clusters_ready "${MAIN_KUBECONFIG}" "${HOST_CLUSTER_NAME}"

#step4. load components images to kind cluster
export VERSION="latest"
export REGISTRY="swr.ap-southeast-1.myhuaweicloud.com/karmada"
kind load docker-image "${REGISTRY}/karmada-controller-manager:${VERSION}" --name="${HOST_CLUSTER_NAME}"
kind load docker-image "${REGISTRY}/karmada-scheduler:${VERSION}" --name="${HOST_CLUSTER_NAME}"
kind load docker-image "${REGISTRY}/karmada-webhook:${VERSION}" --name="${HOST_CLUSTER_NAME}"

#step5. install karmada control plane components
KARMADA_APISERVER_IP=$(docker inspect --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "${HOST_CLUSTER_NAME}-control-plane")
"${REPO_ROOT}"/hack/deploy-karmada.sh "${MAIN_KUBECONFIG}" "${HOST_CLUSTER_NAME}" "${KARMADA_APISERVER_IP}"

#step6. wait until the member cluster ready and join member clusters
util::check_clusters_ready "${MEMBER_CLUSTER_KUBECONFIG}" "${MEMBER_CLUSTER_1_NAME}"
util::check_clusters_ready "${MEMBER_CLUSTER_KUBECONFIG}" "${MEMBER_CLUSTER_2_NAME}"

# connecting networks between member1 and member2 clusters
echo "connecting cluster networks..."
util::add_routes "${MEMBER_CLUSTER_1_NAME}" "${MEMBER_CLUSTER_KUBECONFIG}" "${MEMBER_CLUSTER_2_NAME}"
util::add_routes "${MEMBER_CLUSTER_2_NAME}" "${MEMBER_CLUSTER_KUBECONFIG}" "${MEMBER_CLUSTER_1_NAME}"
echo "cluster networks connected"

#join push mode member clusters
export KUBECONFIG="${MAIN_KUBECONFIG}"
kubectl config use-context "${KARMADA_APISERVER_CLUSTER_NAME}"
${KARMADACTL_BIN} join member1 --cluster-kubeconfig="${MEMBER_CLUSTER_KUBECONFIG}"
${KARMADACTL_BIN} join member2 --cluster-kubeconfig="${MEMBER_CLUSTER_KUBECONFIG}"

# wait until the pull mode cluster ready
util::check_clusters_ready "${MEMBER_CLUSTER_KUBECONFIG}" "${PULL_MODE_CLUSTER_NAME}"
kind load docker-image "${REGISTRY}/karmada-agent:${VERSION}" --name="${PULL_MODE_CLUSTER_NAME}"

#step7. deploy karmada agent in pull mode member clusters
"${REPO_ROOT}"/hack/deploy-karmada-agent.sh "${MAIN_KUBECONFIG}" "${KARMADA_APISERVER_CLUSTER_NAME}" "${MEMBER_CLUSTER_KUBECONFIG}" "${PULL_MODE_CLUSTER_NAME}"

function print_success() {
  echo
  echo "Local Karmada is running."
  echo "To start using your karmada, run:"
cat <<EOF
  export KUBECONFIG=${MAIN_KUBECONFIG}
EOF
}

print_success
