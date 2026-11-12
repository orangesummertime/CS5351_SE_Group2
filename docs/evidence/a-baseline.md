# A · Tangentry original baseline

Status: **historical observation**, not a fresh run in this fork or a result of the proposed budget feature. The recorded runs used a previous documentation-only checkout based on upstream `45611ad0e214a2d9de6f0ec5a5aa84fc493d76f2`. The raw logs below are retained as provenance; repeat the checks in this fork before treating them as current verification.

The independent reviewer reports a fresh WSL2 rerun on this fork in the [PR review memo](../review-memo.md): frontend build passed, 61 files / 231 tests passed, and Go reported 43 passing packages / 11 without tests. The raw rerun evidence is retained by the reviewer, not in this repository. This does not replace the historical logs below or show budget behavior.

| Environment | Command / artifact | Observed result |
| --- | --- | --- |
| Windows, Node 24 / npm | `cd src/web && npm ci --no-audit --no-fund && npm run build && npm test -- --run` | Build succeeded; 61 test files and 231 tests passed. Vite emitted a chunk-size warning, not a failure. [Full output](a-upstream-web-baseline.log). |
| WSL Linux native filesystem, Go 1.27.1 | `cd src && go test ./...` after copying the built `src/web/dist` for `go:embed` | Exit 0: 43 packages reported `ok`, 11 packages had no test files. [Full output](a-upstream-go-linux-baseline.log). |
| Windows, Go 1.27.1 | `cd src && go test ./...` | Exit 1. Examples include Unix-style private-file permission assertions and SQLite temporary-file cleanup/locking. This is an observed Windows failure; it is not counted as a passing baseline. Raw log retained locally outside the public repo for diagnosis. |

The Linux checkout was cloned from the same fork commit into WSL's native filesystem. Its first Go run failed at setup because `src/web/dist` is generated and untracked; after copying the already built frontend assets, the complete Go run passed. No source changes were made to get the passing result. Browser E2E was not run because this project prohibits launching a new browser instance without an explicit request.

Separately, the user's Windows desktop app was installed from the upstream [v0.0.3.6 x64 release](https://github.com/mu-zi-lee/Tangentry/releases/tag/v0.0.3.6). Its archive matched GitHub's SHA-256 `bed49df3fbb065af72d9465cf74b2068d09fa7fc898c09f9f18c9a06d9c7a474`, and its main window opened. That release is **not** the fork's pinned source commit, so its launch is not evidence that the fork's current code has been packaged or that the budget feature works.

No real AI provider, API credential, semantic quality evaluation, or budget behavior was exercised in this baseline. Those need separate controlled tests and honest reporting.
