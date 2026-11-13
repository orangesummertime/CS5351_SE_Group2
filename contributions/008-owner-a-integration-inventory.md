# 008 · Inventory A's integration boundaries

- **Commit:** `docs: map A provider-call integration boundaries`; see this file's Git history.
- **Task owner:** Kefan CHEN.
- **Scope:** Existing AI entry points, provider POST paths, and the shared budget boundary.
- **Review:** Proposed for parallel group review; no implementation approval is claimed.

## What changed and why

I traced the current HTTP routes through the AI service and provider adapter. The inventory identifies where chat, streaming chat, summary, translation and Check can start a completion request, and which reads or model-discovery paths start none. It also records the exact cross-owner question for later AI-backed features: each must use the same reservation boundary.

## Contribution and verification

The table gives reviewers a concrete map to check before code is written, reducing the chance that a retry, fallback or new feature bypasses A's limit. Every linked source path was checked in the current fork. No application code, provider request or executable test was added by this commit.

## Presentation explanation

“The budget must count real provider requests. I mapped each user entry point to the service and transport paths, including the stream fallback and Check. That map lets us explain where a call is reserved and why cached reads do not consume a slot.”
