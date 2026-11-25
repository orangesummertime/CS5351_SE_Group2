# Owner E regression after merging upstream main

- **Date:** 2026-11-25
- **Branch:** `owner-e-liu-ruoxuan`
- **Merge commit:** `4c29ba7c8bd9143d9686bfa439230c3f3dd4ed82`
- **Parents:** `23094c7c103cc4d577a4132c13dde14b2895aeb3` (previous PR #11 head) and `e53e7a289566a91e31a340623af3f7ba3ab12bb7` (`orangesummertime/main`, merge of PR #12)
- **Environment:** Linux. Module tests used Go 1.27.1, the version pinned in `src/go.mod` and `.github/workflows/ci.yml`. Node was v22.23.4. The workflow file pins Node 24.21.0, and the 2026-11-20 record in `e-verification.md` used Node v24.22.1. This file reports the toolchain that actually ran.

These commands were run on the merged tree before this note was added. They are a local regression of that merge. They are not an independent review, not a GitHub review, and not a test-approval record. No `APPROVED` file was added under `docs/evidence/test-approvals/`. PR #11 was not merged.

## Focused freshness and AI packages

Same package list as `docs/evidence/e-verification.md`:

```bash
cd src
go test ./internal/freshness ./internal/ai ./internal/adapters/sqlite ./internal/transport/http ./internal/layering ./internal/citation -count=1
```

Result: all six packages `ok`, exit 0. `./internal/ai` includes the comparison tests that arrived with PR #12.

## Recorded regression script

```bash
bash scripts/ci/regression.sh
```

Result: exit 0.

- Frontend: Vitest v5.0.5, 71 files, 307 tests passed. That run includes `chatFreshness.test.ts` and `ComparisonPanel.test.tsx`.
- Frontend production build: exit 0. The existing chunk-size warning was printed. `tsc --noEmit` is part of that build.
- Go: `go test ./... -count=1 -skip TestCompatibilityFFmpegProducesH264WithAudioAndOriginalTimeline` from `src`. 47 packages `ok`, 11 packages with no test files, no package failed. The skipped FFmpeg duration test is the pre-existing exception already named in `scripts/ci/regression.sh` and `docs/evidence/e-verification.md`. It was not re-run and is not claimed fixed.

## Other course-gate commands on this tree

```bash
bash scripts/ci/schema.sh
python3 scripts/ci/safety-paths.py
python3 scripts/ci/test_approval_gate.py
bash scripts/ci/semantic-eval_test.sh
bash scripts/ci/static-analysis-diff_test.sh
```

Results: schema valid against `req-schema.json`; safety-paths found no credential or obvious permission-bypass literals; approval-gate self-tests `Ran 10 tests` `OK`; semantic-eval self-test passed; static-analysis diff self-test passed. All exited 0.

After the merge commit, the diff-based gates were run with `GITHUB_BASE_SHA=e53e7a289566a91e31a340623af3f7ba3ab12bb7` so the base is current `orangesummertime/main`, not the older fork `main`:

```bash
bash scripts/ci/ai-use-log.sh
bash scripts/ci/semantic-eval.sh
python3 scripts/ci/test-approval.py
bash scripts/ci/static-analysis.sh
```

- `ai-use-log.sh`: exit 0. `docs/ai-use-log.yaml` is in the change set.
- `semantic-eval.sh`: exit 0. No prompt or model-configuration path changed relative to that base.
- `static-analysis.sh`: exit 0. gofmt clean for 12 changed Go files, `go vet ./...`, `go mod verify`, `tsc --noEmit`, and safety-paths.
- `test-approval.py`: exit 1. The only record in the diff is `docs/evidence/test-approvals/owner-e.pending.yaml` with `decision=PENDING`. The printed requirement scope was `FR-05, INV-04, INV-06, INV-07, NFR-05`. `INV-04` is included because its `source` list names `src/internal/ai/chat.go`, which this branch changes; the `INV-04` statement itself was not edited. This failure is the gate's existing pending state. It was not closed by writing an approval.

## Not run

The optional long-conversation follow-up (restore alignment past the 39-message client window) was not started. Conflict resolution did not require it.
