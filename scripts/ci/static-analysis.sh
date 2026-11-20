#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
. "${SCRIPT_DIR}/common.sh"

# fail_if_go_unchecked fails when the diff contains Go files but the checker
# was given nothing to inspect. expected and checked are counts.
fail_if_go_unchecked() {
  local expected="$1"
  local checked="$2"
  if [[ "$expected" -gt 0 && "$checked" -eq 0 ]]; then
    echo "static-analysis: Go files changed but none were checked" >&2
    return 1
  fi
}

count_changed_go() {
  local diff_out="$1"
  local count=0
  local line
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    case "$line" in
      */node_modules/*) ;;
      *.go) count=$((count + 1)) ;;
    esac
  done <<< "$diff_out"
  printf '%s\n' "$count"
}

# check_changed_gofmt gofmts Go files in the pull-request diff.
# Diff failures are fatal. A diff that names Go files but yields nothing to
# check is also fatal.
check_changed_gofmt() {
  local diff_out
  if ! diff_out="$(changed_files)"; then
    echo "static-analysis: failed to list changed files against the pull request base" >&2
    return 1
  fi
  local go_files=()
  local line
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    case "$line" in
      */node_modules/*) ;;
      *.go) go_files+=("$line") ;;
    esac
  done <<< "$diff_out"
  local expected
  expected="$(count_changed_go "$diff_out")"
  fail_if_go_unchecked "$expected" "${#go_files[@]}" || return 1
  if [[ ${#go_files[@]} -eq 0 ]]; then
    echo "gofmt clean for changed Go files (0 checked)"
    return 0
  fi
  local dirty
  dirty="$(gofmt -l "${go_files[@]}")"
  if [[ -n "$dirty" ]]; then
    echo "gofmt would rewrite changed files:"
    printf '%s\n' "$dirty"
    return 1
  fi
  echo "gofmt clean for changed Go files (${#go_files[@]} checked)"
}

main() {
  echo "::group::gofmt"
  check_changed_gofmt
  echo "::endgroup::"

  # src/web/assets.go embeds web/dist. A clean Actions checkout does not have
  # that untracked directory, so build it before go vet.
  echo "::group::Frontend build"
  (cd src/web && npm ci --no-audit --no-fund && npm run build)
  echo "::endgroup::"

  echo "::group::go vet"
  if [[ ! -d src/web/dist ]]; then
    echo "static-analysis: src/web/dist is missing after the frontend build; go vet embeds it and must not run without it" >&2
    exit 1
  fi
  (cd src && go vet ./...)
  echo "::endgroup::"

  echo "::group::go mod verify"
  (cd src && go mod verify)
  echo "::endgroup::"

  echo "::group::TypeScript"
  (cd src/web && npx tsc --noEmit)
  echo "::endgroup::"

  echo "::group::Safety-path checks"
  python3 scripts/ci/safety-paths.py
  echo "::endgroup::"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi
