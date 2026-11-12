# 005 · Main synchronization and review follow-ups

- **Commit:** `docs: reconcile main and review follow-ups`; see this file's Git history.
- **Task owner:** Kefan CHEN.
- **Review:** The prior approval was for `05b8feb`; this new revision requires independent re-review.
- **Scope:** main's README update, INV-01 traceability, ADR review status, review-memo clarification, and contribution wording.

## What changed and why

The branch now includes the latest main commit assigning task D to Wenlong Lu. A dedicated AC-INV-01-2 maps A-T22 to INV-01. The ADR and owner-authored review follow-up now describe the actual review sequence and explain that the first review's A-T01–A-T21 scope predates A-T22. The contribution record distinguishes owner-side repository setting results from what the independent reviewer could verify.

## Contribution and verification

This update removes stale review statements and keeps the requirement-to-test link complete before final merge. The requirement schema, fit references, document links, whitespace, branch ancestry, and repository text scan were checked. No application code, live provider call, or implementation test was added.

## Presentation explanation

“After the reviewer approved the document package, main advanced and the reviewer spotted several small consistency issues. I brought main's task assignment into the branch, linked A-T22 to a specific acceptance criterion, and clarified which review findings belong to which revision. The new revision goes back for review.”
