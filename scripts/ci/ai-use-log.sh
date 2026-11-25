#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "${SCRIPT_DIR}/common.sh"

if ! files="$(changed_files)"; then
  echo "ai-use-log: cannot determine the changed files" >&2
  exit 1
fi

need_log=0
while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  if is_implementation_path "$file" || is_prompt_or_model_path "$file"; then
    echo "code or prompt path changed: $file"
    need_log=1
  fi
done <<< "$files"

if [[ "$need_log" -eq 0 ]]; then
  echo "ai-use-log: no code or prompt changes; gate passes."
  exit 0
fi

if printf '%s\n' "$files" | grep -qx "docs/ai-use-log.yaml"; then
  echo "ai-use-log: docs/ai-use-log.yaml was updated in this change set."
  exit 0
fi

echo "ai-use-log: code or prompt changes require an update to docs/ai-use-log.yaml" >&2
exit 1
