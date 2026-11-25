#!/usr/bin/env bash
# Prints the manifests that install the given version of the operator.
#
# Usage: manifest.sh <version>
set -euo pipefail

version=$1
image="ghcr.io/kkohtaka/kubernetesimal/kubernetesimal:${version}"

rendered=$(kubectl kustomize config/default)

# config/manager names the image that the Deployment runs. Fail loudly if that ever changes, instead of publishing
# manifests that point at the wrong image.
placeholder=$(printf '%s\n' "${rendered}" | grep -o 'image: .*kubernetesimal/kubernetesimal:latest' | sort -u || true)
if [ -z "${placeholder}" ] || [ "$(printf '%s\n' "${placeholder}" | wc -l)" -ne 1 ]; then
  echo "expected exactly one image reference to replace, found: ${placeholder:-none}" >&2
  exit 1
fi

printf '%s\n' "${rendered}" | sed "s|${placeholder}|image: ${image}|"
