#!/usr/bin/env bash
# Shared helpers for course CI gates. Intended to run from the repository root.
set -euo pipefail

repo_root() {
  git rev-parse --show-toplevel
}

cd "$(repo_root)"

base_ref() {
  if [[ -n "${GITHUB_BASE_SHA:-}" ]]; then
    printf '%s\n' "${GITHUB_BASE_SHA}"
    return 0
  fi
  if [[ -n "${GITHUB_EVENT_PATH:-}" ]] && command -v python3 >/dev/null; then
    local sha
    sha="$(python3 -c 'import json,os,pathlib; p=os.environ.get("GITHUB_EVENT_PATH");
d=json.loads(pathlib.Path(p).read_text()) if p else {};
print((d.get("pull_request") or {}).get("base",{}).get("sha") or "")' 2>/dev/null || true)"
    if [[ -n "${sha}" ]]; then
      printf '%s\n' "${sha}"
      return 0
    fi
  fi
  if git rev-parse --verify origin/main >/dev/null 2>&1; then
    printf '%s\n' "origin/main"
    return 0
  fi
  if git rev-parse --verify main >/dev/null 2>&1; then
    printf '%s\n' "main"
    return 0
  fi
  echo "base_ref: no pull-request base SHA and no main ref" >&2
  return 1
}

# changed_files lists paths changed since the pull-request base.
# A missing base is a failure. Callers must not treat that as an empty diff.
changed_files() {
  local base
  if ! base="$(base_ref)"; then
    return 1
  fi
  if ! git rev-parse --verify --quiet "${base}^{commit}" >/dev/null; then
    if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
      echo "changed_files: fetching missing base ${base}" >&2
      if ! git fetch --no-tags origin "$base"; then
        echo "changed_files: base ${base} is not available" >&2
        return 1
      fi
    fi
  fi
  if ! git rev-parse --verify --quiet "${base}^{commit}" >/dev/null; then
    echo "changed_files: base ${base} is not available; fetch the pull request base before diffing" >&2
    return 1
  fi
  if git merge-base --is-ancestor "$base" HEAD 2>/dev/null; then
    git diff --name-only "${base}...HEAD"
  else
    git diff --name-only "$base" HEAD
  fi
}

is_implementation_path() {
  local f="$1"
  case "$f" in
    src/*_test.go|src/*/*_test.go|src/*/*/*_test.go|src/*/*/*/*_test.go|src/*/*/*/*/*_test.go) return 1 ;;
    src/web/src/*.test.ts|src/web/src/*.test.tsx|src/web/src/*/*.test.ts|src/web/src/*/*/*.test.ts|src/web/src/*/*/*/*.test.ts) return 1 ;;
    src/web/src/*.test.tsx|src/web/src/*/*.test.tsx|src/web/src/*/*/*.test.tsx|src/web/src/*/*/*/*.test.tsx) return 1 ;;
    src/web/src/*) return 0 ;;
    src/cmd/*|src/internal/*) return 0 ;;
    prompts/*) return 0 ;;
    *) return 1 ;;
  esac
}

is_prompt_or_model_path() {
  local f="$1"
  case "$f" in
    src/internal/ai/prompts.go|src/web/src/features/ai/promptTemplates.ts|prompts/*|test/offline/eval-b/eval-configs.yaml)
      return 0
      ;;
    *) return 1 ;;
  esac
}
