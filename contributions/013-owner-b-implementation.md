# 013 · Implement owner B hybrid question-relevant retrieval

- **Commit:** Phase 0 `docs: record B retrieval approval and phase-0 gate`; later phases append here; see this file's Git history.
- **Task owner:** WANG Jiaqi (B).
- **AI tool:** OpenCode (qwen4-max-preview); sessions logged as B-AI-008+ in `docs/ai-use-log.yaml`.
- **Scope:** FR-02, NFR-02, INV-04, INV-05; branch `owner/b-implementation` from `main` `83c7726`.
- **Related:** [ADR-002](../docs/architecture/adr/002-question-relevant-retrieval.md), [implementation plan](../docs/plans/b-retrieval-preimplementation.md), [requirements](../docs/req/requirements.yaml), PR #2 (design approval).
- **Review:** design approved via PR #2 review 5897467590 (orangesummertime, non-author, APPROVED 2026-11-13T20:19:58Z on `da21804`, merged as `83c7726`). Implementation acceptance remains a human non-author review; no self-approval is claimed.

## Phase 0 — approval and preconditions gate (2026-11-14)

### What changed

Documentation-only. Verified the approval chain read-only (git + GitHub REST
API): PR #2 author `Wjq1024`, single APPROVED review `5897467590` by
`orangesummertime` at `2026-11-13T20:19:58Z` on head `da21804`, merged into
`main` as `83c7726` (2026-11-14). Recorded the approval in the ADR-002 Status
header, the implementation-plan Status header and the four B requirement
entries (`approval: pending → approved`), citing the review and merge commit.
No technical content was changed; the requirements registry passes its JSON
schema after the edit.

### Verification (all commands actually run; raw logs retained)

- `go test ./... -count=1` in `src`: 43 ok packages, 11 no-test packages, exit 0
  (`docs/evidence/b-impl-phase0-baseline-go.log`).
- `npm ci --no-audit --no-fund` (159 packages), `npm run build` (exit 0,
  chunk-size warning only), `npm test -- --run`: 62 files / 251 tests passed,
  exit 0 (`docs/evidence/b-impl-phase0-baseline-vitest.log`). The prompt's
  61/231 baseline predates A's PR #6 merge; current main carries A's tests.
- Toolchains: go1.27.1 linux/amd64, node v26.13.0, npm 11.20.0 (match the
  stated baselines; invoked via per-command PATH prefixes, no persistent
  environment change).
- Latest published migration `081_selected_subtitle_index.sql`; B's proposed
  082/083/084 are free today (re-check at merge).
- `src/internal/retrieval/` absent (greenfield); A's budget files present and
  untouched (`git diff main` empty for `internal/ai/budget.go`, `ledger.go`,
  `adapters/aichat/budget.go`).
- Approval evidence: `docs/evidence/b-impl-phase0-approval.json` (raw API
  payloads) and `b-impl-phase0-approval.log` (commands + exit codes).
- Phase 0 report: `docs/evidence/b-impl-phase0-report.md`.

### Limits and open items

- No implementation code exists yet; this record claims no retrieval behavior.
- One residual documentation inconsistency is flagged for human confirmation:
  the plan's overview paragraphs (`docs/plans/b-retrieval-preimplementation.md`
  "Baseline and proposed behavior" item 2 and "Frontend / backend
  responsibilities") still describe a conditional sweep trigger, while ADR-002
  §Decision 1b and the plan's detailed budget section require the unconditional
  sweep. ADR precedence resolves it; no technical text was edited in Phase 0.
- Phase 7 depends on the ≥20-question evaluation set with gold labels prepared
  personally by WANG Jiaqi; its existence was asked in the Phase 0 report.
  Phase 7 waits for that set; no questions or labels are invented.
- Network activity: read-only GitHub API GETs and npm registry reads only; no
  push, no provider call, no browser.