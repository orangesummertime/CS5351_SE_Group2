# 011 · Implement and verify per-invocation AI call budgets

- **Commit:** `feat: add per-invocation AI provider call budgets`; see this file's Git history.
- **Task owner:** Kefan CHEN.
- **Scope:** FR-01, NFR-01 and INV-01–INV-03; ledger, adapter, services, HTTP/SSE, task controls and verification evidence.
- **Review:** implementation prepared for independent group review; no merge approval is claimed.

## What changed

One task can start multiple provider requests: translation batches, summary leaf/reduce operations, compatibility fallback and JSON-mode retry. Counting only a service method or planned summary calls misses those extra starts. The implementation attaches a mutex-protected ledger to the invocation context and reserves at every actual completion POST boundary.

The optional `maxProviderCalls` field accepts 1–100; omission preserves upstream behavior. Chat, summary and translation show the control and budget result. Check supports the same API field, and agent-derived service operations reuse inherited contexts. Cached reads, history operations and model discovery do not consume completion slots.

Successful tasks remain completed even when the final slot is used. Exhaustion, usage unknown, cancellation and provider failure remain distinct. Unknown/invalid token counts are not fabricated as zero, cumulative integer overflow is rejected, and failed terminals are sticky. Valid partial checkpoints survive; a new explicit task receives a fresh budget. An SSE error or disconnection cannot be emitted as a successful completed answer.

## Contribution to the project

This is A's user-visible increment beyond the upstream reader: bounded provider request starts with observable outcomes and usage. Other owners can reuse the reservation boundary instead of inventing separate completion counters. The implementation preserves original defaults and keeps checkpoint identity independent of the chosen call limit.

The frontend shares tested payload builders and stream classification with the actual hooks. It retains partial output and reports a stopped or incomplete run rather than presenting it as success. The limit counts AI request starts, including permitted retries; it does not promise an advance token or money ceiling.

## Verification and limits

[Verification and case mapping](../docs/evidence/a-budget-verification.md) link A-T01–A-T22 to ledger, transport, service, HTTP/SSE and frontend tests. [The fixed-input comparison](../docs/evidence/a-budget-comparison.md) records real local HTTP starts on the pinned original and modified service workflows. Limit 1 stops the modified summary after 1/3 operations and translation after 8/16 texts; the original unbounded controls complete three and two operations respectively.

The comparison uses scripted valid results after a real local transport call; it is not a model-quality or speed evaluation. Real provider usage-presence measurement remains separate. The existing FFmpeg exact-duration failure is reported separately from the otherwise-full passing regression. Browser E2E and integration with future B/C/D features are not claimed. Full original implementation sessions remain owner-local and are indexed in the AI-use log.

## Presentation explanation

“I count starts at the transport boundary because one click can cause retries or multiple batches. The ledger reserves atomically, so two workers cannot both take the last slot. If usage is missing, I stop the task and show unknown values rather than zero. Completed batches are saved, but resuming is a new explicit invocation with a new limit. The original-versus-modified fixture shows the cap changes actual request counts while an omitted limit keeps the old behavior.”

Record 010 is used by the B package in PR #2; this record uses 011 to avoid a collision.
