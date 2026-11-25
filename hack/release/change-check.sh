#!/usr/bin/env bash
# Fails when a change touches what users get without adding a change file to .changes/unreleased.
#
# Usage: change-check.sh <base revision>
#
# What users get is the operator's code, its API, and its deployment manifests. Tests, generated code, tooling,
# documentation, and dependency updates need no change file. A change to those paths that changes nothing for users,
# such as a refactoring, is exempted by a human, who puts the "skip-changelog" label on the pull request.
set -euo pipefail

base=$1

changed=$(git diff --name-only "${base}" -- \
  main.go api controller controllers hash k8s net observability pki ssh config \
  ':(exclude)*_test.go' ':(exclude)**/zz_generated.*' ':(exclude)**/testdata/**' ':(exclude)config/samples/**')

if [ -z "${changed}" ]; then
  echo "No change to what users get. No change file is needed."
  exit 0
fi

# Untracked files count as well, so that the check gives the same answer before and after the commit.
committed=$(git diff --name-only --diff-filter=A "${base}" -- .changes/unreleased)
untracked=$(git ls-files --others --exclude-standard -- .changes/unreleased)
added=$(printf '%s\n%s\n' "${committed}" "${untracked}" | grep '\.yaml$' || true)

if [ -n "${added}" ]; then
  echo "Change files:"
  printf '%s\n' "${added}" | sed 's/^/  /'
  exit 0
fi

{
  echo "These files change what users get, but no change file was added to .changes/unreleased:"
  printf '%s\n' "${changed}" | sed 's/^/  /'
  echo
  echo "Add one with: make change KIND=<Added|Changed|Deprecated|Removed|Fixed|Security> BODY=\"...\""
  echo "If nothing changes for users, ask the owner to put the skip-changelog label on the pull request."
} >&2
exit 1
