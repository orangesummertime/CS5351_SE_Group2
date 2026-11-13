# A · Shared provider-call budget: implementation and test plan

**Status: implementation prepared for review following group confirmation on 2026-11-13.** Design, live evidence and integration inventory were reviewed and merged in PR #1, PR #4 and PR #5 respectively. The group member confirmed proceeding under this A plan. Implementation changes still require review before merge. This plan covers FR-01/NFR-01.

## Baseline and proposed behavior

Tangentry already has AI translation, summaries, chat and caching. The adapter does not account for all completion POSTs or expose actual token usage. Retries, fallback, summary map/reduce, translation batches and recovery can amplify one user action. See [baseline](../evidence/a-baseline.md) and [ADR](../architecture/adr/001-provider-call-budget.md).

An optional integer caps **new provider completion request starts per user-triggered invocation**. Omitted means unlimited and preserves existing behavior. Explicit `0`, negative, non-integer or greater than `100` is rejected before any provider request. The UI labels omission “No call limit”; it never uses an unlabeled zero for unlimited. A resumed summary or translation is a new explicit invocation with a fresh budget and reusable completed checkpoints. This is neither a token nor money ceiling.

## Boundaries and interfaces

1. `internal/ai` owns a concurrency-safe invocation ledger: maximum, atomic started-request count, validated per-call and cumulative prompt/output/total usage, and known/unknown state. A valid total equals prompt plus output; missing, partial, negative or non-numeric usage is unknown, not zero.
2. The adapter reserves immediately before every completion POST: `/chat/completions`, `/responses` and both streaming paths. Every fallback/retry needs another slot. `GET /models`, cached reads and history clear consume none.
3. The [ADR error table](../architecture/adr/001-provider-call-budget.md#error-classification-for-budgeted-invocations) governs failures. A started request always consumes its slot. Ambiguous failure never silently retries. The implicit `chat.go:96` stream fallback must obtain a new reservation.
4. Service, HTTP/SSE and UI distinguish `completed`, `budget_exhausted`, `usage_unknown`, cancellation and provider failure. Partial checkpoints cannot appear completed. A blocked terminal emits no success `event: done`.
5. Agent-derived and Check operations count any completion POST they start under the same per-invocation rule. Zero-call Check/cache paths consume none. The deterministic entry-point cases are specified in A-T22.

For streaming chat, this proposal keeps the upstream request format and measures whether each provider returns valid usage. Because the current request does not ask for streaming usage, a visible answer with missing usage ends as `usage_unknown`, even if the text appears complete. The proposed UI copy is: “The answer was received, but usage data is unavailable. This task cannot make another AI request. Start a new task to continue.” It must not display a completed state or a known-zero token count. No claim is made that either provider supports streaming usage.

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
| A-T12 | AC-FR-01-1, AC-FR-01-2, INV-03 | HTTP/SSE and UI presentation: bounded/unknown/normal states, explicit streaming-chat unknown-usage copy, no `event: done` on blocked terminal, no known-zero count or key leak. |
| A-T13 | AC-FR-01-4 | Invalid limit fails before any POST. |
| A-T14 | AC-FR-01-5 | Two valid positive usages: exact per-call and cumulative prompt/output/total, `usageKnown=true`, `completed`. |
| A-T15 | AC-FR-01-4 | Matrix: omitted unlimited; 1 and 100 valid; 0, negative, non-integer and 101 invalid; explicit UI label. |
| A-T16 | INV-01 | Two concurrent chats with limit 1 each may each start one POST; budgets remain isolated. |
| A-T17 | INV-01, INV-02 | Table-driven ADR error classes: assert slot/refund/retry/terminal; ambiguous stream failure makes no hidden retry. |
| A-T18 | AC-FR-01-1 | SSE disconnect mid-stream: no `event: done` or false completed UI; started slot consumed. |
| A-T19 | AC-FR-01-1 | Corrupted checkpoint cannot be reused as complete or bypass the new limit. |
| A-T20 | INV-01 | Simulated crash after reservation: live invocation does not refund or start an extra POST; no durable crash-survival claim. |
| A-T21 | AC-FR-01-2 | `usage_unknown` blocks same invocation; a new explicit invocation may resume valid completed work with a fresh limit. |
| A-T22 | AC-INV-01-1, INV-01 | Agent-derived translate and summarize use the same reservation rule under limit 1; Check's synthetic completion counts as one slot when it starts a POST; zero-call Check/cache paths consume none. |

## Live usage-presence measurement gate

**Executed on 2026-11-13; [results and limits](../evidence/a-live-usage-presence.md).** The reviewed protocol required at least 20 non-streaming completions **per provider** on CityU gateway DeepSeek V4 Flash and one separately configured provider, plus a separate streaming arm. The run used Mistral-hosted GLM 5.3 Flash as the second provider and three streaming samples per provider. The primary target met the ≥95% valid-usage threshold in its non-streaming arm (20/20). CityU streaming returned no usage (0/3), so the strict `usage_unknown` design remains necessary. The per-request evidence is redacted and separates provider and response path. Independent human review of this evidence is required before Phase 1.

## Implementation and review gate

After live measurement and independent human approval, use course-specified OpenCode + DeepSeek V4 Flash for implementation and retain full raw sessions. Phases: domain/adapter, service/checkpoints, then HTTP/SSE/UI. After **each** phase run focused tests and full Go and frontend suites with uncached Go `-count=1`, retaining raw logs. Track existing Windows failures separately; compare like environments. Browser E2E must use only an existing or embedded browser.

PR #1's final design revision received independent non-author approval and was merged. PR #4's evidence and PR #5's inventory were subsequently approved and merged. Those GitHub reviews explicitly covered documentation and evidence. The group member then confirmed proceeding under the existing A plan on 2026-11-13. The implementation PR must retain the test evidence and original implementation sessions and receive its own review before merge.
