# ADR-003 · Chat answer and citation freshness after source change

- **Status:** registered for review; approval and implementation acceptance are pending an independent non-author review (suggested: Owner D). This record is not a self-approval.
- **Owner:** 刘若轩 (E).
- **Related:** [FR-05, NFR-05, INV-06, INV-07](../../req/requirements.yaml), [plan and tests](../../plans/e-freshness-preimplementation.md), [debt](../../debt-register.md), [CI gates](../../plans/e-ci-gates.md).

## Context and decision

Tangentry already persists AI chat history as a JSON blob keyed by content ID, and Owner C stores generation-time citation verification (`verified`, `invalid`, `unverified`, `stale`, `ambiguous`) on assistant messages. Owner B added server-owned `content_revision` and `content_hash` columns on `saved_content` (migration 084). The upstream reader also caches summaries by source-text hash and keeps summary citations attached to that cache. None of these, by themselves, recompute whether a *stored chat answer or citation* is still current after the saved source is later updated or deleted.

FR-05 is therefore a read-time freshness property of chat history, not a new generator and not a claim on upstream summary-cache behaviour. This ADR decides:

1. **Capture a snapshot when an assistant message is stored.** The service records `sourceRevision` and `sourceHash` on the message, and `contentRevision` / `contentHash` on each citation (falling back to C's `sourceVersion` of the form `content-revision:N` for legacy rows).
2. **Derive status at read time.** `Chat`, `ChatHistory` and `GET /api/ai/chat` recompute `versionStatus` / `freshness` against the live `saved_content` row. Historical text is kept. The derived status is not used to overwrite C's `Status`.
3. **Use three user-visible states:** `current`, `expired`, and `unavailable`. An item is `current` only when the content still exists and both the captured revision and hash match (when present). A revision-only bump from `Save` also expires evidence. A missing row is `unavailable`. Missing snapshots, lookup errors and an unwired reader fail closed and are never displayed as currently valid.
4. **Reject client-supplied freshness.** Incoming messages have citations and version labels stripped before use, consistent with C's untrusted-citation rule.
5. **Do not add a duplicate `content_revision` / `content_hash` migration.** E consumes B's columns. No already-shipped migration is edited. v1 stores snapshots in the existing chat result JSON and needs no new table.

The enforcing component is deterministic code in `internal/freshness` and `internal/ai`. The UI must show expired or unavailable labels on answers and citations and must not present those items as “当前有效”.

## Invariants

| Statement | Source requirement | Enforcing component | Verification | Runtime fallback action |
| --- | --- | --- | --- | --- |
| INV-06: displayed-as-currently-valid only when the live row exists and matches the captured snapshot; errors and forgeries fail closed. | [NFR-05](../../req/requirements.yaml) | `freshness.Assess`, `applyFreshness`, `stripUntrustedEvidence`, SQLite `CurrentContentVersion` | E-T01, E-T05, E-T07, E-T08 | Return `expired` or `unavailable`; never label `current` on doubt. |
| INV-07: reopen recomputes freshness from live content; history text is retained; repeated reopen is stable while the source is unchanged. | [FR-05](../../req/requirements.yaml) | `ChatHistory`, HTTP `GET /api/ai/chat` | E-T02–E-T04 | Do not rewrite or delete the stored blob on GET. |

## Alternatives and deferred scope

Eagerly rewriting stored history on every `Save`/`Delete` would need a secondary index of conversations and could leave a wrong durable `current` label if a write failed. Hash-only expiry would keep evidence current after a no-op `Save`; this v1 prefers the more conservative revision-or-hash rule. Applying the same status to notes or to D's comparison results needs those records to carry snapshots; that work is deferred ([debt register](../../debt-register.md)).

## Reopen conditions

Reopen this ADR if C or D persist additional long-lived evidence that users might treat as current, if B changes how `content_revision` / `content_hash` are assigned, if the group wants hash-only expiry, or if course acceptance requires a dedicated evidence table or note freshness. Record a new human review rather than silently changing the fail-closed rule.

## Review gate

Implementation on this branch was prepared for independent review. It is not approved by its author. Suggested reviewer: Wenlong Lu (Owner D). GitHub branch protection and Issue #3 boxes are not claimed as complete by this ADR.
