# ADR-001 · Per-invocation provider-call budget

- **Status:** revised after CHANGES REQUIRED; independent re-review pending.
- **Owner:** Kefan CHEN.
- **Related:** [FR-01, NFR-01 and INV-01](../../req/requirements.yaml), [plan and tests](../../plans/a-budget-preimplementation.md), [debt](../../debt-register.md).

## Context and decision

One Tangentry action may start many provider requests through summary map/reduce, translation batches and recovery, JSON-mode retry, 404 `/responses` fallback, or empty-stream fallback. Four completion POST sites exist in [protocol.go](../../../src/internal/adapters/aichat/protocol.go); `chat.go:96` can initiate a fallback. Summary `TotalCalls` is a checkpoint/plan count, not a transport attempt count. The adapter currently does not parse usage. `GET /models`, cache reads and history clear make no completion POST.

Attach a concurrency-safe per-invocation ledger to context. `internal/ai` owns limit and terminal semantics; `aichat` reserves **before each completion POST** and reconciles valid usage afterward. An omitted limit means unlimited; explicit integer 1–100 is accepted; 0, negative, non-integer or >100 is rejected before requests. This is an opt-in cap on started requests, not tokens or money. Resume is a new explicit invocation; completed checkpoints can be reused. Simultaneous chats get separate budgets. Agent-derived/Check entry points obey the same rule for provider POSTs; their exact UI/API exposure still needs review.

Valid usage has nonnegative integer prompt/output/total counts with total = prompt + output. Record per-call and cumulative values. Missing, partial, malformed or inconsistent usage after a successful completion becomes `usage_unknown`; no later completion starts in that invocation. The cap and usage status are exposed through service, HTTP/SSE and UI. A blocked operation is not success; partial checkpoints stay partial. No SQLite usage ledger in v1.

Streaming chat keeps the upstream request format for this proposal. The current stream request does not ask for usage, so a returned answer without valid usage is shown as `usage_unknown`, with explicit UI copy and no known-zero token count or further POST in the same invocation. The separate streaming measurement arm will show whether each provider returns usable data; this behavior must be reviewed again if the results undermine practical chat availability.

## Error classification for budgeted invocations

“Slot” means a completion POST was started. Every started POST consumes one slot, including a rejected request; no refund is inferred from an HTTP status. “Retry” below is automatic retry **within the same invocation**, only with another available slot and known non-generation. A blocked retry returns `budget_exhausted`. Cancellation is distinct from provider failure. These policies are proposed for reviewer approval.

| Response / failure | Slot | Refund | Automatic retry / fallback | Terminal when no retry |
| --- | --- | --- | --- | --- |
| 404 on `/chat/completions` with known `/responses` compatibility | 1 | No | One compatible `/responses` attempt after a fresh reservation | Provider failure or budget exhausted |
| Explicit unsupported JSON-mode parameter rejection | 1 | No | One compatible retry after fresh reservation | Provider failure or budget exhausted |
| Other 4xx, including malformed request | 1 | No | No | Provider failure |
| 401 / 403 | 1 | No | No | Authentication/authorization failure |
| 429 | 1 | No | No | Rate-limited provider failure |
| 5xx, including 504 | 1 | No | No | Provider failure; consumption may be unknown |
| Timeout | 1 | No | No | Timeout; consumption unknown |
| Connection reset / ambiguous stream failure | 1 | No | No | Transport failure; consumption unknown |
| User cancellation after start / SSE disconnect | 1 | No | No | Cancelled; consumption unknown |
| Empty stream with no usable content | 1 | No | Existing `chat.go:96` non-stream fallback only after a new reservation and only if clearly no completion occurred | Provider failure or budget exhausted |

A response that definitely completed but has missing/malformed usage terminates as `usage_unknown`, even if slots remain. For 5xx/timeout/reset/cancellation, unknown **consumption** does not imply a valid usage count; no automatic retry occurs. A later explicit user action starts a new invocation. All response/fallback paths must be exercised by [A-T17](../../plans/a-budget-preimplementation.md).

## Invariants

| Statement | Source requirement | Enforcing component | Verification | Runtime fallback action |
| --- | --- | --- | --- | --- |
| INV-01: started completion POSTs never exceed the invocation limit, including concurrency, retries and fallback. | [NFR-01](../../req/requirements.yaml) | Atomic `ai` reservation called at all four adapter POST sites and before chat fallback | A-T02–A-T07, A-T09, A-T16–A-T17, A-T20 | Reject unreserved POST; report `budget_exhausted`; preserve partial work. |
| INV-02: unknown or invalid usage is never presented as known zero or followed by another POST in that invocation. | [FR-01](../../req/requirements.yaml) | Adapter usage parser and `ai` ledger | A-T08–A-T09, A-T17, A-T21 | Mark `usage_unknown`, stop further starts, show partial state. |
| INV-03: a blocked, failed or interrupted invocation never emits a completed result. | [FR-01](../../req/requirements.yaml) | Service terminal state, HTTP/SSE and UI presentation | A-T02, A-T06, A-T12, A-T18–A-T19 | Emit non-success terminal, no success `event: done`. |

## Alternatives and deferred scope

Counting service method calls misses transport retries and segmented work. An account-wide proxy requires shared persistent coordination. A hard token ceiling cannot be guaranteed from usage returned after completion. [Debt register](../../debt-register.md) states reopen triggers for those cuts.

## Reopen conditions

Reopen this ADR before implementation if live measurement shows valid usage below the reviewer-approved threshold, a provider uses a different usage format or stream semantics, the group requires global or lifetime budgets, or agent-derived/Check entry points cannot adopt the same boundary. Reopen after implementation if new completion POST sites, fallback paths or crash-survival requirements appear. Record a new human review decision rather than silently changing the limit, error classifications or invariants.

## Review gate

This proposal requires independent non-author approval and live evidence before implementation. The new fork has no recorded review decision for this ADR.
