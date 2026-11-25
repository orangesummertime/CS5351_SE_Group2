# Bug diary

Append-only notes of defects found during group work. An entry is not a claim
that the defect is fixed unless a later note says so and points to tests.

## E · 2026-11-20 · stored chat history had no freshness after source change

- **Found while:** implementing Owner E (FR-05).
- **Symptom:** `ChatHistory` and `GET /api/ai/chat` returned stored messages as saved at generation time. After `saved_content` was updated or deleted, old answers and C-verified citations could still be read without an expired or unavailable label.
- **Not an upstream summary bug:** summary results are cached by source-text hash and are a different path.
- **Related observation:** the chat panel did not render chat citations at all, so even C's generation-time status was not visible in the UI. E added freshness and citation labels for stored chat answers; this is not a claim that C's full verification UI is complete.
- **Related observation:** `citation.StatusStale` existed as a constant but was not assigned by C's validator. E does not reuse that constant for post-change freshness; it uses `freshness` / `versionStatus` (`expired` / `unavailable` / `current`).
- **Follow-up:** covered by ADR-003 and tests E-T01–E-T10. Independent review is still required.

## E · 2026-11-20 · course CI gates were not present as PR checks

- **Found while:** checking `.github/workflows` and the public branch-protection API.
- **Symptom:** only `Release` existed, and it runs on version tags. `main` was protected, but required status checks were off and empty. The six course gates could not block merges.
- **Follow-up:** in-repo jobs were added in `.github/workflows/ci.yml`. GitHub enforcement and Issue #3 remain open admin work (E-D02).
