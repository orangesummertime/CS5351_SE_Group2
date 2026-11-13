# A · Implementation, verification and review guide

Status: implementation verified and prepared for review; independent implementation review and merge remain pending.

## Observable behavior

Chat, summary and translation expose an optional call limit. Blank means **No call limit** and preserves the original behavior. An explicit integer from 1 to 100 limits newly started completion requests in one invocation. Invalid values are rejected before provider work. Check also accepts the optional API field; agent-derived service calls inherit a parent invocation ledger.

The server reserves atomically at each of the four completion POST paths. Compatibility fallback and JSON-mode retry require fresh reservations. Cache/history reads and model discovery consume no completion slot. Two concurrent chats have separate ledgers. A new explicit resume receives a fresh budget and reuses only valid completed checkpoints.

Limited responses contain `budget`: maximum, started requests, outcome, known per-call usage and nullable cumulative usage. A successfully finished task is `completed` even when it uses its final slot. Missing, malformed, inconsistent or overflowing usage stops the invocation as `usage_unknown`; unknown counts are null, not zero. Cancellation/provider failure cannot be overwritten by a later success. Partial output is retained without a success terminal. A failed SSE path emits no success `done` event.

## Test traceability

The [approved cases](../plans/a-budget-preimplementation.md) remain the acceptance source. The following implementation files contain the corresponding cases; their names and assertions distinguish workflow tests, transport tests and simulated failures.

| Plan cases | Executable evidence |
| --- | --- |
| A-T01, A-T04–A-T08, A-T13–A-T17 | [adapter budget tests](../../src/internal/adapters/aichat/budget_test.go): actual local HTTP starts, four response paths, retries, invalid usage, concurrency and error classes |
| A-T07, A-T14–A-T16, A-T20–A-T21 | [ledger tests](../../src/internal/ai/ledger_test.go): atomic reservation, usage totals, isolated contexts, interrupted reservation and fresh invocation |
| A-T02–A-T03, A-T09–A-T11, A-T15–A-T16, A-T18–A-T22 | [service tests](../../src/internal/ai/budget_service_test.go): exact two-leaf summary, smaller-batch recovery, checkpoints/resume, cancellation, corruption and inherited parent ledgers |
| A-T06 | [Chat fallback tests](../../src/internal/ai/chat_budget_test.go): a fallback cannot bypass the last slot |
| A-T08–A-T09, A-T14, A-T17, A-T21 | [failure-state tests](../../src/internal/ai/budget_gap_test.go): partial/error reports, unknown cumulative usage, overflow and sticky terminal state |
| A-T01, A-T10, A-T12–A-T15, A-T18, A-T22 | [HTTP/SSE tests](../../src/internal/transport/http/ai_budget_test.go): real service/adapter chain, invalid limit before SSE headers, partial reports, exact-limit success and disconnected stream |
| A-T12, A-T15, A-T18 | [frontend tests](../../src/web/src/features/ai/callBudget.test.ts): the builders and stream classifier used by actual request hooks, limit validation, report/copy and incomplete-stream handling |

A-T20 simulates an abandoned reservation in memory. It does not claim durable budgets survive process crashes. A-T22 verifies inherited service contexts and synthetic Check calls; future agent entry points still need integration checks when introduced.

## Fixed-input comparison

[The comparison](a-budget-comparison.md) and [12 recorded rows](a-budget-comparison.json) run identical summary and translation content against pinned upstream and this implementation. Summary needs three provider operations; translation needs two. With limit 1, the implementation starts exactly one POST and preserves 1/3 summary progress or 8/16 translated texts. With limit 3 both finish. Omitted limits match the unbounded control.

The provider boundary is scripted and performs actual local Chat HTTP requests. This measures service workflow and budget enforcement, not prompt decoding, real-model quality, money saved or a speed improvement. The upstream does not support the new field; its labelled limit rows are repeated unbounded controls. The earlier [live usage evidence](a-live-usage-presence.md) separately records real provider behavior.

## Reproducible checks

Run Go commands from `src` and frontend commands from `src/web`:

```sh
go test -count=1 ./internal/ai ./internal/adapters/aichat ./internal/transport/http
CGO_ENABLED=1 go test -race -count=1 ./internal/ai ./internal/adapters/aichat ./internal/transport/http
go test -count=1 ./...
go test -count=1 -skip TestCompatibilityFFmpegProducesH264WithAudioAndOriginalTimeline ./...
go vet ./cmd/... ./internal/...
npm test -- --reporter=dot
npm run build
```

The full unfiltered Go run has the pre-existing FFmpeg exact-duration failure (`1.020998s` versus the test's exact `1.000000s`). It is retained separately; the otherwise-full regression skips only that named test. Windows permission/symlink differences are not used to judge the Linux result. Browser E2E was not run; frontend tests, type checking/build and HTTP/SSE tests are the recorded verification boundaries.

Final run on 2026-11-13: 62 frontend files / 251 tests and production build passed; focused and race Go checks for AI, provider adapter and HTTP passed; the otherwise-full Linux regression passed with only the named existing FFmpeg test excluded; `go vet` passed. Original full session exports are retained owner-locally with SHA-256 pointers in `docs/ai-use-log.yaml`.

## Review and integration limits

Group confirmation authorized implementation under the existing A plan. This implementation still needs non-author review before merge. B/C/D's future completion paths must adopt the same ledger boundary; this PR cannot certify modules that do not exist yet. No new database migration, global account cap, hard token cap or monetary cap is introduced.
