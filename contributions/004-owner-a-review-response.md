# 004 · Owner A review response

- **Commit:** `docs: address owner A review findings`; see this file's Git history.
- **Task owner:** Kefan CHEN.
- **Review:** Responds to the independent CHANGES REQUIRED decision on PR #1; re-review is pending.
- **Scope:** requirements, ADR, implementation/test plan, baseline note, README, review follow-up, and contribution index.

## What changed and why

The requirements registry now includes INV-02 and INV-03, with fit criteria linked to the requested negative and failure-path tests. INV-01 no longer lists its own registry file as its source. The plan adds A-T22 for agent-derived entry points and Check, and specifies how streaming chat reports unknown usage. A separate streaming measurement arm is proposed without making live calls. The README and baseline note link the reviewer's fresh rerun. The authenticated owner-side API reported an independent-approval rule on main, while the reviewer could verify only that main is protected; Issue #3 tracks the six course CI gates.

The signed reviewer findings remain intact. An owner follow-up records which items were addressed and which still need the reviewer's decision, live evidence, or future implementation.

## Contribution and verification

This change makes the design and tests more traceable and closes the documented bypass and presentation questions at the planning level. YAML/schema validation, test-ID coverage, documentation links, the authenticated owner-side repository settings response, and the PR diff were checked. The reviewer could not independently verify the specific protection settings. No new application code, live provider call, or implementation test was run.

## Presentation explanation

“The reviewer found missing invariant registrations, an untested entry point, and unclear streaming usage behavior. I revised the registry and tests, documented the conservative unknown-usage behavior, and kept implementation blocked pending re-review and live evidence. The repository now has a human review gate while the CI gates are tracked separately.”
