# Owner E verification evidence

- **Date:** 2026-11-20 (Hong Kong time)
- **Branch:** `owner-e-liu-ruoxuan`
- **Environment:** Linux, Go 1.27.3, Node v24.22.1. Commands were run from the repository root. This file does not record local absolute paths.

These results support the local implementation. They are not an independent review and not GitHub CI enforcement.

## Schema

```bash
bash scripts/ci/schema.sh
```

Result: `requirements.yaml is valid against req-schema.json` (`ajv-cli@5 --spec=draft2020`). Exit 0.

## Safety-path check

```bash
python3 scripts/ci/safety-paths.py
```

Result: `safety-paths: no credential or obvious permission-bypass literals found`. Exit 0.

## Focused freshness tests

```bash
cd src
go test ./internal/freshness ./internal/ai ./internal/adapters/sqlite ./internal/transport/http ./internal/layering ./internal/citation -count=1
```

Result: all six packages `ok` (exit 0). This includes the frozen NFR-05 suite, service reopen/update/delete/concurrency/negative tests, SQLite integration, HTTP history tests, layering and citation packages.

## Frontend

```bash
cd src/web
npx tsc --noEmit
npx vitest run src/features/ai/chatFreshness.test.ts
npm test
npm run build
```

Results:

- `tsc --noEmit`: exit 0 (also run as part of `npm run build`).
- Focused freshness tests: pass.
- Full Vitest: 69 files, 292 tests passed, exit 0.
- Production build: exit 0 (chunk-size warning only).

## Regression (same policy as `scripts/ci/regression.sh`)

Frontend build is required before Go packages that embed `web/dist`. The known pre-existing test `TestCompatibilityFFmpegProducesH264WithAudioAndOriginalTimeline` (exact duration mismatch already recorded by Owner A) was skipped so the run measures other packages.

```bash
cd src/web && npm test && npm run build
cd ../src && go test ./... -count=1 -skip TestCompatibilityFFmpegProducesH264WithAudioAndOriginalTimeline
```

Result: all executed Go packages passed, exit 0. The FFmpeg exact-duration case is a pre-existing environmental failure, not introduced by E, and is documented rather than claimed fixed.

Without the skip, that tools package test still fails on this machine with a duration of `1.020998` versus the expected exact timeline, matching Owner A's earlier exception.

## Approval-gate self-test checkout shapes

`test_this_pr_rejects_unrelated_baseline_approval` called `git merge-base main HEAD` and required this pull request's FR-05 diff. That case was replaced by `test_isolated_fixture_rejects_unrelated_baseline_approval`, which builds a temporary repository and does not read the host branch. The other gate cases were not changed. These runs used separate clones so the working branch was left in place. They are local reproductions of the two checkout shapes, not a GitHub Actions log.

On `d064280`, before the replacement, `python3 scripts/ci/test_approval_gate.py`:

- Detached HEAD, remote-tracking `origin/main` present, no local `main`. `git merge-base main HEAD` exited 128 (`fatal: Not a valid object name main`). The suite exited 1: the coupled case errored, and the other nine cases passed.
- Local `main` pointed at the same commit as HEAD, so `git diff --name-only main HEAD` was empty. The suite exited 1: the coupled case failed because `FR-05` was not in the empty requirement scope, and the other nine cases passed.

After the replacement, the same command exited 0 (10 tests) in both shapes, and in the normal checkout where local `main` is still the pull-request base.

## Gates not satisfied on this branch

- Test-approval CI job: expected to fail until a non-author writes an APPROVED record in `docs/evidence/test-approvals/`.
- GitHub required status checks: not enabled (see `e-branch-protection.md`).
- Issue #3 boxes: left unchecked.
- Official OpenCode + DeepSeek V4 Flash annotated transcripts: not produced (debt E-D04).
