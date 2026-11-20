#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "${SCRIPT_DIR}/common.sh"

# replay_consumes_path is true only for files the frozen Owner B retrieval
# replay reads. Prompt templates and eval-configs.yaml are not in that set
# (replay loads questions.yaml, negative-cases.yaml, and frozen vectors).
replay_consumes_path() {
  case "$1" in
    test/offline/eval-b/questions.yaml|test/offline/eval-b/negative-cases.yaml|test/offline/eval-b/vectors/*)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

if ! files="$(changed_files)"; then
  echo "semantic-eval: cannot determine the changed files; refusing to skip or pass" >&2
  exit 1
fi

unsupported=""
supported=""
while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  if is_prompt_or_model_path "$file"; then
    if replay_consumes_path "$file"; then
      supported="${supported}"$'\n'"${file}"
    else
      echo "semantic-eval: unsupported scope (frozen retrieval replay does not consume this file): $file" >&2
      unsupported="${unsupported}"$'\n'"${file}"
    fi
  fi
done <<< "$files"

if [[ -n "${unsupported}" ]]; then
  echo "semantic-eval: scope unmet. Prompt or model-configuration changes need an evaluation that reads those files. The frozen Owner B retrieval replay does not, so it is not a pass." >&2
  exit 1
fi

if [[ -z "${supported}" ]]; then
  echo "semantic-eval: no prompt or model-configuration paths changed; skipping replay."
  exit 0
fi

echo "semantic-eval: replaying frozen Owner B evaluation set for paths the replay consumes"
while IFS= read -r file; do
  [[ -z "$file" ]] && continue
  echo "  $file"
done <<< "$supported"
(cd test/offline/eval-b && go test ./replay -count=1)
echo "semantic-eval: replay passed"
