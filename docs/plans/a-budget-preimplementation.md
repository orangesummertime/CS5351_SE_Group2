# A · Shared provider-call budget: implementation and test plan

**Status: proposed; independent review and live measurement pending. No budget implementation code exists.** This plan covers FR-01/NFR-01.

## Baseline and proposed behavior

Tangentry already has AI translation, summaries, chat and caching. The adapter does not account for all completion POSTs or expose actual token usage. Retries, fallback, summary map/reduce, translation batches and recovery can amplify one user action. See [baseline](../evidence/a-baseline.md) and [ADR](../architecture/adr/001-provider-call-budget.md).

An optional integer caps **new provider completion request starts per user-triggered invocation**. Omitted means unlimited and preserves existing behavior. Explicit `0`, negative, non-integer or greater than `100` is rejected before any provider request. The UI labels omission “No call limit”; it never uses an unlabeled zero for unlimited. A resumed summary or translation is a new explicit invocation with a fresh budget and reusable completed checkpoints. This is neither a token nor money ceiling.

## Boundaries and interfaces

1. `internal/ai` owns a concurrency-safe invocation ledger: maximum, atomic started-request count, validated per-call and cumulative prompt/output/total usage, and known/unknown state. A valid total equals prompt plus output; missing, partial, negative or non-numeric usage is unknown, not zero.
2. The adapter reserves immediately before every completion POST: `/chat/completions`, `/responses` and both streaming paths. Every fallback/retry needs another slot. `GET /models`, cached reads and history clear consume none.
3. The [ADR error table](../architecture/adr/001-provider-call-budget.md#error-classification-for-budgeted-invocations) governs failures. A started request always consumes its slot. Ambiguous failure never silently retries. The implicit `chat.go:96` stream fallback must obtain a new reservation.
4. Service, HTTP/SSE and UI distinguish `completed`, `budget_exhausted`, `usage_unknown`, cancellation and provider failure. Partial checkpoints cannot appear completed. A blocked terminal emits no success `event: done`.
5. Agent-derived and Check operations count any completion POST they start under the same per-invocation rule. Zero-call Check/cache paths consume none. Entry-point and UI semantics need reviewer confirmation before code.

Concurrent chat invocations have isolated budgets. No global cap or SQLite migration is proposed for v1. [Deferred scope](../debt-register.md) records the three cuts.

## Deterministic pre-implementation tests

Fake providers or `httptest` record raw POST counts. Each row traces to a criterion or invariant.

| ID | AC / invariant | Fixed input and expected outcome |
| --- | --- | --- |
| A-T01 | AC-FR-01-3 | Omitted limit: existing chat, summary, translation outputs and wire shapes remain compatible. |
| A-T02 | AC-FR-01-1, INV-01 | Fixture yields **exactly two** `summaryPlan` groups; limit 1 starts one POST, then `budget_exhausted`, with valid partial checkpoint. |
| A-T03 | INV-01 | Multi-batch translation limit 1: force half-batch recovery; it cannot start an extra POST. |
| A-T04 | AC-NFR-01-1, INV-01 | 404 chat fallback needs second reservation; limit 1 blocks `/responses`. |
| A-T05 | AC-NFR-01-1, INV-01 | Explicit JSON-mode rejection needs a counted retry; limit 1 blocks it. |
| A-T06 | AC-NFR-01-1, INV-01 | Empty stream fallback needs a new reservation; limit 1 blocks it and never reports success. |
| A-T07 | AC-NFR-01-2, INV-01 | Two simultaneous reservations for final slot: at most one starts; race detection where supported. |
| A-T08 | AC-FR-01-2, INV-02 | All four response paths × missing, partial, negative and non-numeric usage: `usage_unknown`, no known zero, no next POST. |
| A-T09 | INV-01, INV-02 | Timeout/cancel after start: no refund or silent retry; terminal states distinct. |
| A-T10 | AC-FR-01-3 | Cache hit, history read/clear and invalid input make zero provider requests. |
| A-T11 | AC-FR-01-1 | Resume capped summary/translation as a new explicit invocation; reuse only valid completed checkpoints. |
| A-T12 | AC-FR-01-1, AC-FR-01-2 | HTTP/SSE and UI presentation: bounded/unknown/normal states, no `event: done` on blocked terminal, non-success copy, no key leak. |
| A-T13 | AC-FR-01-4 | Invalid limit fails before any POST. |
| A-T14 | AC-FR-01-5 | Two valid positive usages: exact per-call and cumulative prompt/output/total, `usageKnown=true`, `completed`. |
| A-T15 | AC-FR-01-4 | Matrix: omitted unlimited; 1 and 100 valid; 0, negative, non-integer and 101 invalid; explicit UI label. |
| A-T16 | INV-01 | Two concurrent chats with limit 1 each may each start one POST; budgets remain isolated. |
| A-T17 | INV-01, INV-02 | Table-driven ADR error classes: assert slot/refund/retry/terminal; ambiguous stream failure makes no hidden retry. |
| A-T18 | AC-FR-01-1 | SSE disconnect mid-stream: no `event: done` or false completed UI; started slot consumed. |
| A-T19 | AC-FR-01-1 | Corrupted checkpoint cannot be reused as complete or bypass the new limit. |
| A-T20 | INV-01 | Simulated crash after reservation: live invocation does not refund or start an extra POST; no durable crash-survival claim. |
| A-T21 | AC-FR-01-2 | `usage_unknown` blocks same invocation; a new explicit invocation may resume valid completed work with a fresh limit. |

## Live usage-presence measurement gate

**Protocol only; not executed.** Before Phase 1, obtain explicit authorization for provider identities, credentials, call/spending ceiling and network use. Run at least 20 non-streaming completions **per provider** on CityU gateway DeepSeek V4 Flash and one separately configured provider (two providers total). Use a fixed harmless short prompt and parameters. Record provider/model, endpoint family, timestamp, HTTP status, presence and validity of prompt/output/total usage, plus redacted response metadata. Do not commit credentials or user data. Report numerator/denominator separately by provider and response path; do not pool. A ≥95% valid usage presence threshold for the primary target is a **proposal requiring an explicit review decision**. If presence is lower or formats differ, reopen strict-stop design before coding. Store live evidence separately under `docs/evidence/`.

## Implementation and review gate

After live measurement and independent human approval, use course-specified OpenCode + DeepSeek V4 Flash for implementation and retain full raw sessions. Phases: domain/adapter, service/checkpoints, then HTTP/SSE/UI. After **each** phase run focused tests and full Go and frontend suites with uncached Go `-count=1`, retaining raw logs. Track existing Windows failures separately; compare like environments. Browser E2E must use only an existing or embedded browser.

The new fork needs an independent non-author review of this design, test cases, and live evidence before implementation. No approval from the previous repository is carried forward.
