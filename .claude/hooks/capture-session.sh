#!/usr/bin/env bash
# SessionEnd hook: copy non-trivial session transcripts for later distillation.
set -uo pipefail

input=$(cat)

read -r transcript_path session_id reason <<<"$(printf '%s' "$input" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get("transcript_path", ""), d.get("session_id", ""), d.get("reason", ""))
except Exception:
    print("", "", "")
' 2>/dev/null)"

[[ -z "$transcript_path" || ! -f "$transcript_path" ]] && exit 0

root="${CLAUDE_PROJECT_DIR:-$(pwd)}"
pending_dir="$root/.claude/kb_source/_pending"

tool_count=$(python3 -c '
import json, sys
n = 0
with open(sys.argv[1]) as f:
    for line in f:
        try:
            evt = json.loads(line)
        except Exception:
            continue
        msg = evt.get("message", {})
        for block in (msg.get("content") or []):
            if isinstance(block, dict) and block.get("type") == "tool_use":
                n += 1
print(n)
' "$transcript_path" 2>/dev/null || echo 0)

if [[ "${tool_count:-0}" -lt 5 ]]; then
  exit 0
fi

mkdir -p "$pending_dir"
ts=$(date -u +%Y%m%dT%H%M%SZ)
out="$pending_dir/${ts}-${session_id:-unknown}.jsonl"

cp -f "$transcript_path" "$out" 2>/dev/null || exit 0

cat > "${out%.jsonl}.meta.json" <<EOF
{
  "session_id": "${session_id}",
  "captured_at": "${ts}",
  "reason": "${reason}",
  "tool_use_count": ${tool_count},
  "transcript": "${out}"
}
EOF

exit 0
