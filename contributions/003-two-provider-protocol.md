# 003 · Two-provider measurement protocol

- **Commit:** `docs: scope usage measurement to two providers`; see this file's Git history.
- **Task owner:** Kefan CHEN.
- **Scope:** `docs/plans/a-budget-preimplementation.md` and the contribution index.
- **Review:** Pending independent group review.

## What changed and why

The proposed live usage-presence measurement now covers two providers in total: CityU gateway DeepSeek V4 Flash and one separately configured provider. This reflects the owner's requested scope. The proposal still calls for at least 20 non-streaming completions per provider; no provider identity, credential, spending limit, or network execution is authorized by this edit.

## Contribution and verification

The reduced scope is explicit for reviewers and keeps results separate by provider. The updated wording and document links were checked. No live provider request or application test was run.

## Presentation explanation

“I reduced the proposed measurement to two providers while retaining separate per-provider results. This was a planning change only; the live evidence and implementation still need review.”
