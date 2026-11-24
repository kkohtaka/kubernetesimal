#!/usr/bin/env bash
# PostToolUse hook: format the Go file that was just written or edited.
set -euo pipefail

file="$(jq -r '.tool_response.filePath // .tool_input.file_path // empty')"
case "$file" in
  *.go) [ -f "$file" ] && gofmt -w "$file" ;;
esac
exit 0
