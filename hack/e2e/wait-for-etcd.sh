#!/usr/bin/env bash
# Waits until the Etcd object in the e2e namespace is running with the given number of ready members, and prints
# what the operator reports once a minute, so that a run that is stuck shows where.
#
# Usage: wait-for-etcd.sh <members> <timeout in seconds>
set -euo pipefail

members=$1
deadline=$(( $(date +%s) + $2 ))
last_report=0

while true; do
  phase=$(kubectl -n e2e get etcd etcd -o jsonpath='{.status.phase}')
  ready=$(kubectl -n e2e get etcd etcd -o jsonpath='{.status.readyReplicas}')
  if [ "${phase}" = "Running" ] && [ "${ready:-0}" = "${members}" ]; then
    echo "etcd is running with ${members} ready member(s)."
    exit 0
  fi

  now=$(date +%s)
  if [ $(( now - last_report )) -ge 60 ]; then
    last_report=${now}
    echo "--- $(date -u +%H:%M:%S) phase=${phase:-unknown} ready=${ready:-0}/${members}"
    kubectl -n e2e get etcdnode,vmi -o wide || true
    kubectl -n e2e get etcdnode \
      -o jsonpath='{range .items[*]}{.metadata.name}{": "}{range .status.conditions[*]}{.type}={.status} {.message}{"; "}{end}{"\n"}{end}' \
      || true
  fi
  if [ "${now}" -ge "${deadline}" ]; then
    echo "Timed out waiting for etcd to run with ${members} ready member(s)." >&2
    exit 1
  fi
  sleep 10
done
