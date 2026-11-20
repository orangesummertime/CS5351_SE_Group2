#!/usr/bin/env bash
# Self-tests for the static-analysis diff/gofmt behavior.
# Uses temporary repositories and does not build the frontend.
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
if ! command -v gofmt >/dev/null; then
  echo "static-analysis-diff_test: gofmt is required" >&2
  exit 1
fi

failures=0
assert_fail() {
  local name="$1"
  local output="$2"
  local code="$3"
  if [[ "$code" -eq 0 ]]; then
    echo "FAIL $name: expected non-zero exit" >&2
    printf '%s\n' "$output" >&2
    failures=$((failures + 1))
  fi
}
assert_ok() {
  local name="$1"
  local output="$2"
  local code="$3"
  if [[ "$code" -ne 0 ]]; then
    echo "FAIL $name: expected exit 0" >&2
    printf '%s\n' "$output" >&2
    failures=$((failures + 1))
  fi
}
assert_contains() {
  local name="$1"
  local output="$2"
  local needle="$3"
  if [[ "$output" != *"$needle"* ]]; then
    echo "FAIL $name: output missing [$needle]" >&2
    printf '%s\n' "$output" >&2
    failures=$((failures + 1))
  fi
}
assert_not_contains() {
  local name="$1"
  local output="$2"
  local needle="$3"
  if [[ "$output" == *"$needle"* ]]; then
    echo "FAIL $name: output unexpectedly contained [$needle]" >&2
    printf '%s\n' "$output" >&2
    failures=$((failures + 1))
  fi
}

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

# Source the checker inside an isolated repository so common.sh does not
# attach to the project checkout.
bootstrap=$(new_repo)
printf 'base\n' >"$bootstrap/README.md"
commit_all "$bootstrap" "base" >/dev/null
cd "$bootstrap"
# shellcheck source=static-analysis.sh
source "$SCRIPT_DIR/static-analysis.sh"
unset GITHUB_EVENT_PATH GITHUB_ACTIONS || true

# A pull-request base SHA that was not fetched must not report a clean gofmt run.
out=$(mktemp)
export GITHUB_BASE_SHA="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
set +e
check_changed_gofmt >"$out" 2>&1
code=$?
set -e
unset GITHUB_BASE_SHA
text=$(cat "$out")
assert_fail "missing base" "$text" "$code"
assert_not_contains "missing base" "$text" "gofmt clean"

# Go files in the diff with nothing checked is fatal even if a caller drops the list.
set +e
fail_if_go_unchecked 1 0 >"$out" 2>&1
code=$?
set -e
text=$(cat "$out")
assert_fail "zero checked" "$text" "$code"
assert_contains "zero checked" "$text" "none were checked"
set +e
fail_if_go_unchecked 1 1 >"$out" 2>&1
code=$?
set -e
assert_ok "checked count matches" "$(cat "$out")" "$code"

# A deliberately misformatted changed Go file fails.
dirty_repo=$(new_repo)
printf 'base\n' >"$dirty_repo/README.md"
base=$(commit_all "$dirty_repo" "base")
mkdir -p "$dirty_repo/src/internal/demo"
printf 'package demo\n\nfunc BadlyFormatted( ) int { return 1 }\n' >"$dirty_repo/src/internal/demo/demo.go"
commit_all "$dirty_repo" "bad gofmt" >/dev/null
cd "$dirty_repo"
export GITHUB_BASE_SHA="$base"
unset GITHUB_EVENT_PATH GITHUB_ACTIONS || true
set +e
check_changed_gofmt >"$out" 2>&1
code=$?
set -e
text=$(cat "$out")
assert_fail "misformatted go file" "$text" "$code"
assert_contains "misformatted go file" "$text" "gofmt would rewrite"

# A clean changed Go file is checked and passes.
clean_repo=$(new_repo)
printf 'base\n' >"$clean_repo/README.md"
base=$(commit_all "$clean_repo" "base")
mkdir -p "$clean_repo/src/internal/demo"
printf 'package demo\n\nfunc Good() int { return 1 }\n' >"$clean_repo/src/internal/demo/demo.go"
commit_all "$clean_repo" "clean gofmt" >/dev/null
cd "$clean_repo"
export GITHUB_BASE_SHA="$base"
set +e
check_changed_gofmt >"$out" 2>&1
code=$?
set -e
text=$(cat "$out")
assert_ok "clean go file" "$text" "$code"
assert_contains "clean go file" "$text" "1 checked"

# A docs-only diff may check zero Go files.
docs_repo=$(new_repo)
printf 'base\n' >"$docs_repo/README.md"
base=$(commit_all "$docs_repo" "base")
printf 'note\n' >"$docs_repo/docs-note.md"
commit_all "$docs_repo" "docs" >/dev/null
cd "$docs_repo"
export GITHUB_BASE_SHA="$base"
set +e
check_changed_gofmt >"$out" 2>&1
code=$?
set -e
text=$(cat "$out")
assert_ok "docs only" "$text" "$code"
assert_contains "docs only" "$text" "0 checked"

if [[ "$failures" -ne 0 ]]; then
  echo "static-analysis-diff_test: $failures failure(s)" >&2
  exit 1
fi
echo "static-analysis-diff_test: passed"
