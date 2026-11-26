#!/usr/bin/env bash
# Creates the cluster that the end-to-end tests run in: a kind cluster with what the operator needs, and with the
# operator image built from this checkout. See ADR 0003. The tests themselves install the operator.
#
# Usage: cluster.sh
#
# Needs docker, kubectl, and curl. Downloads kind into bin/.
set -euo pipefail

# The versions are pinned, and updated like other dependencies. The Kubernetes version is that of the node image.
KIND_VERSION=v0.33.0
KIND_NODE_IMAGE=kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5
CALICO_VERSION=v3.33.1
KUBEVIRT_VERSION=v1.10.0
# The webhook of the operator gets its certificate from cert-manager. This goes away with the webhook.
CERT_MANAGER_VERSION=v1.21.3

CLUSTER_NAME=${CLUSTER_NAME:-e2e}

cd "$(dirname "$0")/../.."

# Virtual machines are never emulated: that is an order of magnitude slower, and fails somewhere else.
if [ ! -r /dev/kvm ] || [ ! -w /dev/kvm ]; then
  echo "/dev/kvm is missing or not usable by $(id -un). The end-to-end tests need hardware virtualization." >&2
  exit 1
fi

mkdir -p bin
kind=bin/kind
if [ ! -x "${kind}" ] || [ "$("${kind}" version -q)" != "${KIND_VERSION}" ]; then
  curl -fsSL -o "${kind}" "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-amd64"
  chmod +x "${kind}"
fi

# The node only becomes ready once the network plugin is installed.
"${kind}" create cluster --name "${CLUSTER_NAME}" --image "${KIND_NODE_IMAGE}" --config hack/e2e/kind.yaml

kubectl apply -f "https://raw.githubusercontent.com/projectcalico/calico/${CALICO_VERSION}/manifests/calico.yaml"
kubectl wait nodes --all --for=condition=Ready --timeout=10m
kubectl -n kube-system wait deployment --all --for=condition=Available --timeout=10m

docker build --quiet --tag kubernetesimal:e2e .
"${kind}" load docker-image --name "${CLUSTER_NAME}" kubernetesimal:e2e

kubevirt="https://github.com/kubevirt/kubevirt/releases/download/${KUBEVIRT_VERSION}"
kubectl create -f "${kubevirt}/kubevirt-operator.yaml"
kubectl create -f "${kubevirt}/kubevirt-cr.yaml"
kubectl apply -f \
  "https://github.com/cert-manager/cert-manager/releases/download/${CERT_MANAGER_VERSION}/cert-manager.yaml"

kubectl -n kubevirt wait kubevirt kubevirt --for=condition=Available --timeout=15m
kubectl -n cert-manager wait deployment --all --for=condition=Available --timeout=10m

# The webhook of cert-manager can refuse requests for a moment after its Deployment is available. Wait until it
# accepts what the operator's manifests contain, so that the tests apply them once and expect that to succeed.
for attempt in $(seq 1 30); do
  if kubectl apply --dry-run=server -f - > /dev/null 2>&1 <<'ISSUER'; then
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: probe
  namespace: default
spec:
  selfSigned: {}
ISSUER
    exit 0
  fi
  echo "cert-manager does not answer yet (attempt ${attempt})."
  sleep 10
done
echo "cert-manager did not start to answer." >&2
exit 1
