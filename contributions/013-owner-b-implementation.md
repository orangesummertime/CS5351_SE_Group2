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

## Phase 1 — domain core (2026-11-14)

### What changed

New stdlib-only domain package `src/internal/retrieval/` implementing the
approved ADR-002/plan contracts: exact plan domain types (`Passage`,
`Selection`, `Embedder`, `PassageSelector`, `ContextAssembler`, …) and ports
(`PassageSource`, `PDFText`); sentence/segment splitter (CJK+Latin
terminators, abbreviations, quotes, code fences, emoji; 1–3 sentences /
200–800 runes; subtitle cue groups 3–8 cues / 15–60 s; headings/pages never
crossed); deterministic lexical scorer (exact phrase, distinct-term coverage,
saturating TF, proximity); unconditional position-stratified sweep sampler
(indices `floor(k*N/S)`, `Stride=ceil(N/S)`, `Coverage=S/N`); mode-aware
proportional capped budget mathematics with code-asserted invariants; hybrid
scoring with weight validation/normalization and the stable tie-break;
whitespace-normalized dedupe; six-layer assembler (rune + 200,000-byte caps,
drop sequence, degraded 8k profile, constraint re-injection); INV-04 preflight
and INV-05 re-read verifier; selector orchestrating validate → preflight →
load/split → lexical → sweep → embed → dedupe/verify.

### Verification (test-first; raw logs retained)

- First run RED: 8 failure groups recorded in
  `docs/evidence/b-impl-phase1-red.log`; fixed within phase scope
  (implementation bugs: ellipsis re-scan, grouping min-check order, wrapped
  sentinel comparison, zero-capacity lexical fallback, reminder re-injection
  flag; plus documented test-expectation corrections).
- GREEN: 31 test functions covering B-T02, B-T06, B-T08, B-T10, B-T14, B-T15,
  B-T16, B-T17, B-T19, B-T21 (both halves), B-T22, B-T29, B-T39, B-T40 (A/B/C)
  plus budget worked examples, sweep/weights/dedupe/no-match/empty-input/
  embedder-failure/determinism (`b-impl-phase1-retrieval.log`).
- Gates: focused packages exit 0 (`b-impl-phase1-focused.log`); full Go suite
  44 ok / 11 no-test exit 0 (`b-impl-phase1-go-full.log`); layering + vet +
  A-budget empty-diff proof (`b-impl-phase1-layering.log`); frontend unchanged
  62 files / 251 tests + build exit 0 (`b-impl-phase1-web.log`); two-run
  determinism diff identical (`b-impl-phase1-determinism.log`).
- Phase report: `docs/evidence/b-impl-phase1-report.md`.

### Limits

- Adapters (SQLite source, PDF, embedding/rerank), transport and frontend are
  later phases; the domain is exercised through fakes only. Fake hash vectors
  are unit-test-only, never an NFR-02 claim. B-T33's full fake-transport mode
  matrix runs in Phase 4 (budget math half already reproduced here).
- NFC normalization is deferred (stdlib-only constraint); matching uses
  lowercase + whitespace/line-ending normalization on precomposed fixtures.
- No recall measurement yet; Phase 7 waits for WANG Jiaqi's evaluation set.