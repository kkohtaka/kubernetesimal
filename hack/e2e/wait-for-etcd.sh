#!/usr/bin/env bash
# Waits until every given Etcd object in the e2e namespace has one ready etcd node, and prints what the operator
# reports once a minute, so that a run that is stuck shows where.
#
# It looks at the EtcdNodeDeployment instead of the Etcd object, because the Etcd object does not become ready in
# this environment: its Service points at the cluster addresses of other Services, which Kubernetes does not route.
#
# Usage: wait-for-etcd.sh <timeout in seconds> <name>...
set -euo pipefail

deadline=$(( $(date +%s) + $1 ))
shift
last_report=0

while true; do
  pending=0
  for name in "$@"; do
    ready=$(kubectl -n e2e get etcdnodedeployment "${name}" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)
    [ "${ready:-0}" = "1" ] || pending=$(( pending + 1 ))
  done
  if [ "${pending}" -eq 0 ]; then
    echo "Every etcd node is ready: $*"
    exit 0
  fi

  now=$(date +%s)
  if [ $(( now - last_report )) -ge 60 ]; then
    last_report=${now}
    echo "--- $(date -u +%H:%M:%S) waiting for ${pending} of $# etcd node(s)"
    kubectl -n e2e get etcdnode,vmi -o wide || true
    kubectl -n e2e get etcdnode \
      -o jsonpath='{range .items[*]}{.metadata.name}{": "}{range .status.conditions[*]}{.type}={.status} {.message}{"; "}{end}{"\n"}{end}' \
      || true
  fi
  if [ "${now}" -ge "${deadline}" ]; then
    echo "Timed out waiting for the etcd nodes: $*" >&2
    exit 1
  fi
  sleep 5
done
