# 007 · Align A decision record with live evidence

- **Commit:** `docs: align A decision record with live evidence`; see this file's Git history.
- **Task owner:** Kefan CHEN.
- **Scope:** ADR-001 status, streaming evidence link, and review gate.
- **Review:** PR #4 remains open for independent group review.

## What changed and why

The decision record still described the streaming measurement as future work after the evidence had been collected. This update links the observed CityU and Mistral streaming results and distinguishes the approved design in PR #1 from the pending live-evidence review in PR #4.

## Contribution and verification

The ADR now gives the group a current reason for the proposed `usage_unknown` behavior without implying that the budget feature exists. The linked evidence and status statements were checked against PR #1's merge and PR #4's open state. No application code or new provider call was made.

## Presentation explanation

“After measuring the two providers, I updated the architecture decision to show the observed streaming behavior and the remaining review gate. The design can now be explained from evidence rather than a future assumption.”
