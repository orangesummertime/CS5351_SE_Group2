#!/usr/bin/env bash
# Self-test: prompt and eval-config changes are not passed by the retrieval replay.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
EVAL="$SCRIPT_DIR/semantic-eval.sh"

new_repo() {
  local dir
  dir=$(mktemp -d)
  git -C "$dir" init -q -b main
  git -C "$dir" config user.email test@example.com
  git -C "$dir" config user.name Test
  git -C "$dir" config commit.gpgsign false
  printf '%s\n' "$dir"
}

commit_all() {
  local dir="$1"
  local message="$2"
  git -C "$dir" add -A
  git -C "$dir" commit -q -m "$message"
  git -C "$dir" rev-parse HEAD
}

run_eval() {
  local dir="$1"
  local base="$2"
  env -u GITHUB_EVENT_PATH -u GITHUB_ACTIONS GITHUB_BASE_SHA="$base" bash "$EVAL"
}

repo=$(new_repo)
printf 'base\n' >"$repo/README.md"
base=$(commit_all "$repo" "base")
mkdir -p "$repo/src/internal/ai"
printf 'package ai\n\nconst prompt = "changed"\n' >"$repo/src/internal/ai/prompts.go"
commit_all "$repo" "change prompts" >/dev/null
cd "$repo"
set +e
output=$(run_eval "$repo" "$base" 2>&1)
code=$?
set -e
if [[ "$code" -eq 0 ]]; then
  echo "semantic-eval_test: prompt change was accepted" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi
if [[ "$output" != *"scope unmet"* ]]; then
  echo "semantic-eval_test: prompt change did not report unmet scope" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi
if [[ "$output" == *"replay passed"* ]]; then
  echo "semantic-eval_test: prompt change was passed by the retrieval replay" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi

repo=$(new_repo)
printf 'base\n' >"$repo/README.md"
base=$(commit_all "$repo" "base")
mkdir -p "$repo/test/offline/eval-b"
printf 'runs: []\n' >"$repo/test/offline/eval-b/eval-configs.yaml"
commit_all "$repo" "change eval config" >/dev/null
cd "$repo"
set +e
output=$(run_eval "$repo" "$base" 2>&1)
code=$?
set -e
if [[ "$code" -eq 0 || "$output" != *"scope unmet"* || "$output" == *"replay passed"* ]]; then
  echo "semantic-eval_test: eval-configs change was not reported unmet" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi

repo=$(new_repo)
printf 'base\n' >"$repo/README.md"
base=$(commit_all "$repo" "base")
printf 'note\n' >"$repo/docs-note.md"
commit_all "$repo" "docs only" >/dev/null
cd "$repo"
set +e
output=$(run_eval "$repo" "$base" 2>&1)
code=$?
set -e
if [[ "$code" -ne 0 || "$output" != *"skipping replay"* ]]; then
  echo "semantic-eval_test: unrelated change did not skip" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi

repo=$(new_repo)
printf 'base\n' >"$repo/README.md"
base=$(commit_all "$repo" "base")
cd "$repo"
set +e
output=$(env -u GITHUB_EVENT_PATH -u GITHUB_ACTIONS GITHUB_BASE_SHA="0000000000000000000000000000000000000000" bash "$EVAL" 2>&1)
code=$?
set -e
if [[ "$code" -eq 0 || "$output" == *"skipping replay"* ]]; then
  echo "semantic-eval_test: missing base was treated as a skip" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi

echo "semantic-eval_test: passed"
