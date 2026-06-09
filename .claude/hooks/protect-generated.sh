#!/usr/bin/env bash
# PreToolUse hook: reject Write/Edit on generated, vendored, or asset files.
set -euo pipefail

input=$(cat)
path=$(printf '%s' "$input" | python3 -c '
import json, sys
d = json.load(sys.stdin)
ti = d.get("tool_input", {})
print(ti.get("file_path") or ti.get("notebook_path") or "")
')

[[ -z "$path" ]] && exit 0

root="${CLAUDE_PROJECT_DIR:-$(pwd)}"
rel="${path#${root%/}/}"

deny() {
  printf 'BLOCKED edit of %s\nReason: %s\n' "$rel" "$1" >&2
  exit 2
}

case "$rel" in
  *.pb.go)
    deny "Generated protobuf code. Edit the .proto and run 'make gen' instead."
    ;;
  gen/*|gen/**/*)
    deny "Generated code under gen/. Regenerate via 'make gen' instead of hand-editing."
    ;;
  vendor/*)
    deny "Vendored dependency. Update via go.mod + 'go mod vendor', do not hand-edit."
    ;;
  assets/*|assets6.3/*)
    deny "Prebuilt AMD SMI/DRM libraries. Refresh via the dedicated update process, do not hand-edit."
    ;;
esac

exit 0
