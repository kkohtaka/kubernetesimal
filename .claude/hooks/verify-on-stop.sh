#!/usr/bin/env bash
# Stop hook: when Go files have uncommitted changes, do not let the turn end until the code builds, passes vet, and
# introduces no new lint issues. The full test suite runs in CI and with 'make test'.
set -uo pipefail

input="$(cat)"
# Avoid a loop: if this hook already blocked once in this turn, let the turn end.
if [ "$(jq -r '.stop_hook_active // false' <<<"$input")" = "true" ]; then
  exit 0
fi

cd "${CLAUDE_PROJECT_DIR:-.}" || exit 0

if [ -z "$(git status --porcelain -- '*.go' 2>/dev/null)" ]; then
  exit 0
fi

if ! out="$(go build ./... 2>&1 && go vet ./... 2>&1 && make --no-print-directory lint 2>&1)"; then
  {
    echo "Go files have uncommitted changes that do not pass the checks. Fix the issues below, or explain why they cannot be fixed."
    echo "$out" | tail -n 60
  } >&2
  exit 2
fi
exit 0
