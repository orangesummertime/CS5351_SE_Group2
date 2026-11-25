# Owner E · freshness test and implementation plan

- **Owner:** 刘若轩 (E), GitHub `@h5f8fzckp7-coder`, student ID 60862168.
- **Status:** plan and tests prepared for independent review. This file is not an approval. Implementation on the same branch still requires a non-author test-approval record before merge (see `docs/evidence/test-approvals/`).
- **Related:** [FR-05, NFR-05, INV-06, INV-07](../req/requirements.yaml), [ADR-003](../architecture/adr/003-citation-version-freshness.md), [CI gates](e-ci-gates.md).

## Distinction from existing behaviour

| Existing behaviour | Not in E's increment |
| --- | --- |
| Upstream summary citations and source-hash result cache | Unchanged; not claimed as FR-05. |
| C generation-time citation `Status` (`verified` / `invalid` / …) | Preserved on stored citations. E adds a separate `freshness` / `versionStatus`. |
| B `content_revision` / `content_hash` and retrieval `stale_source` at query time | Consumed, not duplicated. FR-05 is about *stored chat answers* after a later source change. |

## Storage rules

- Chat history remains the existing `ai` result row (`kind=chat`, key derived from content ID).
- Each new assistant message captures `sourceRevision` and `sourceHash`. Citations capture `contentRevision` and `contentHash`.
- Freshness is derived at read time. GET / reopen does not delete or rewrite historical text.
- No edit to shipped migrations. No second content-version column migration.
- Related notes and comparison results are not migrated in v1.

## Test plan

All tests are deterministic and offline. No live provider, browser E2E or wall-clock assertion is required for acceptance.

| ID | Area | Expected | Maps to |
| --- | --- | --- | --- |
| E-T01 | Frozen NFR-05 suite (≥20 cases: update, delete, reopen, concurrent, negatives) | Overshoot of stale-as-current is 0 | AC-NFR-05-1 |
| E-T02 | Update then `ChatHistory` / HTTP GET | Old answer and citations are `expired`, not currently valid | AC-FR-05-1 |
| E-T03 | Delete then reopen | Status is `unavailable` | AC-FR-05-2 |
| E-T04 | Unchanged reopen, then stable reopen after change | Unchanged stays `current`; after change, two reopens agree and are not `current` | AC-FR-05-3, AC-INV-07-1 |
| E-T05 | Forged client freshness / snapshot / citations | Labels stripped; not stored as current | AC-FR-05-4, AC-INV-06-1 |
| E-T06 | New question after an update | Old message expired; new message stamped current | AC-FR-05-5 |
| E-T07 | Concurrent save/delete vs history read; post-join reopen | Final reopen is not currently valid | AC-NFR-05-2 |
| E-T08 | Missing reader or lookup error | Fail closed, not currently valid | AC-INV-06-2 |
| E-T09 | Frontend presentation negatives | `verified` alone is not shown as 当前有效; expired/unavailable labels are used | NFR-05 display |
| E-T10 | Legacy message without a snapshot | Treated as expired when the source still exists | INV-06 |

Negative tests are E-T05, E-T08, E-T09, E-T10 and the negative rows inside E-T01.

## Commands

```bash
cd src
go test ./internal/freshness ./internal/ai ./internal/adapters/sqlite ./internal/transport/http ./internal/layering ./internal/citation -count=1
cd web
npx vitest run src/features/ai/chatFreshness.test.ts
```

Full regression (also the CI regression gate):

```bash
cd src && go test ./... -count=1
cd web && npm test && npm run build
```
