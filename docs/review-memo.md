# CS5351 PR review memo

## PR #1 · Owner A baseline, requirements, and budget test plan

- **PR:** [Owner A: baseline, requirements, and budget test plan](https://github.com/orangesummertime/CS5351_SE_Group2/pull/1)
- **Author / owner:** Kefan CHEN (Owner A)
- **Scope:** docs-only — historical baseline evidence, draft FR-01/NFR-01/INV-01, proposed ADR-001, deterministic test design A-T01–A-T21, live measurement protocol, debt register. No budget implementation code (deliberate: course test-approval gate).
- **Reviewer:** WANG Jiaqi (GitHub: Wjq1024), assigned by the project technical lead; not the author.
- **Decision:** **CHANGES REQUIRED** (2026-11-12). This is the human test-approval gate action; it is NOT an approval. Implementation must not start until C1–C5 are addressed and re-reviewed.

### Spec alignment

Requirements registry validates against its schema (`ajv-cli@5 --spec=draft2020`, exit 0). All 8 fit criteria map to tests (cov_fit = 8/8 = 1): AC-FR-01-1→A-T02, -2→A-T08, -3→A-T01, -4→A-T15, -5→A-T14; AC-NFR-01-1→A-T04, -2→A-T07; AC-INV-01-1→A-T04. MoSCoW, explicit tradeoffs, NFR metric/threshold/method, and linked sources are present; limit semantics are explicit (omitted = unlimited labeled; 1–100 valid; 0/negative/non-integer/>100 rejected). Gaps: INV-02/INV-03 declared in the ADR are not registered in the registry (C3); remaining D1 types are group-wide follow-up.

### Architecture alignment

All five ADR factual premises were verified against source in this checkout: four completion POST sites (`protocol.go:127/181/250/356`); JSON-mode retry (`aichat/summary.go:47-50`, `aichat/translation.go:129-132`); 404→/responses fallback (`protocol.go:385-386,152-153`); empty-stream fallback (`ai/chat.go:96-97`); summary map/reduce (`summary_generate.go:380-515`); translation batching, half-batch recovery and paragraph splitting (`ai/translation.go:180-257`, `translation_batch.go:14-74`); `TotalCalls` is plan/checkpoint-only (local merge increments it without any POST, `summary_generate.go:456-470`); no usage parsed on any response path; GET /models, cache reads and history clear make zero completion POSTs; the active gate serializes summary/translation/check while chat is concurrent with isolated per-invocation budgets. The ADR error-classification table matches source behavior; the implicit `chat.go:96` fallback must take a fresh reservation. Invariants carry the D2 five elements; reopen conditions are defined. Gap: streaming chat usage semantics need an explicit decision (C4).

### Code & test quality (plan and test design; no implementation code in this PR)

A-T01–A-T21 are deterministically executable with established httptest/fake-provider conventions (raw atomic POST counters, privacy assertions). Negative tests and violation-fails coverage exist for every invariant: concurrency (A-T07 race + A-T16 isolation), unknown usage never known-zero (A-T08, four response paths × malformed set), blocked never success (A-T06/A-T12/A-T18, no `event: done`). Limit matrix, error-table-driven tests, corrupted checkpoint, crash-after-reservation and usage_unknown resume are all covered. Regression strategy (per-phase full suites, uncached Go `-count=1`, retained logs, Windows failures tracked separately) is executable and low-cost. Gap: agent-derived/Check entry-point budget semantics need an explicit test (T1, A-T22).

### Operational metadata

Baseline claims were re-executed by the reviewer in this fork (WSL2 Linux, Go 1.27.1, Node v26.13.0): frontend build exit 0 (chunk-size warning only), Vitest 61 files / 231 tests passed; `go test ./...` exit 0 with 43 ok / 11 no-test packages — package-name sets identical to the retained logs. The evidence documents honestly label the retained logs as historical observations, disclose cached entries, and redact local paths; the upstream v0.0.3.6 desktop install is explicitly disclaimed as non-project evidence. Windows failures are environmental and separated (not reproducible on the reviewer's Linux machine). Gaps: no AI-use log and no planning transcripts in this fork (C1); owner name and contribution records are otherwise in good order.

### Security & efficiency

The budget is a deterministic safety-relevant boundary on AI output. All four completion POST sites are covered; agent-derived operations use the same service/adapter path, so no counting bypass exists; zero-call paths are correctly excluded. Error messages are fixed strings with existing no-leak tests; the plan promises no credentials or source text in telemetry; security scans of docs/contributions found no credential material. Efficiency direction is correct (strict stopping and conservative retry classification reduce request/token consumption; no persistence or global coordinator in v1). The live usage-presence measurement gate (two providers, ≥20 non-streaming completions each, ≥95% primary-target threshold — approved by this review, results reported per provider) must complete before Phase 1.

### Rationale

The design premises are solid, the test design is comprehensive and traceable, and the evidence culture is honest. Three MAJOR items remain: missing AI-use disclosure/transcripts (C1), an unprotected merge gate with Issues disabled and no PR CI (C2), and unregistered invariants INV-02/INV-03 (C3). All are fixable within this PR, hence CHANGES REQUIRED rather than REJECT. No BLOCKER.

### Changes requested

- [ ] C1 (F-01) Create `docs/ai-use-log.yaml` disclosing how this package was produced: AI-assisted entries (tool/model/purpose) with raw planning sessions committed, or a signed retention statement with location and export commitment; if manual, `tool: none (manual)` entries.
- [ ] C2 (F-02) Repo admin: enable main branch protection (≥1 non-author approving review), enable Issues, open the tracking issue for E's six CI blocking gates; record the interim manual gate here.
- [ ] C3 (F-03) Register INV-02 and INV-03 in `requirements.yaml` with fit criteria mapped to existing tests (INV-02→A-T08/09/17/21; INV-03→A-T02/06/12/18/19).
- [ ] C4 (F-05) Decide and document streaming-chat usage semantics: add a streaming arm to the measurement protocol and explicit `usage_unknown` UI copy for chat, or request `stream_options.include_usage` where supported.
- [ ] C5 (F-07) Fix INV-01 self-referential source; update README task checkbox with the fresh baseline evidence (61/231, 43/11, reviewer re-run 2026-11-12).

### Plan-test revision

- [ ] T1 Add A-T22: agent-derived translate/summarize under a limit obey the same reservation rule; Check with a limit counts its synthetic call as one slot; zero-call paths remain exempt. (Reviewer confirms the entry-point semantics per ADR:11 / plan:17.)
- [ ] T2 Per C4, extend the measurement protocol or the UI specification; keep per-provider results separate and label live evidence.

### Approved design decisions (recorded for the implementation gate)

Per-invocation budget with resume = new explicit invocation and reusable checkpoints; strict stop on unknown usage with the pre-implementation measurement gate (two providers, ≥95% primary-target threshold approved); the ADR error-classification table as written; v1 scope cuts per debt register A-D01–A-D03.

### Sign-off

**WANG Jiaqi** — Reviewer (GitHub: Wjq1024) — 2026-11-12 — Decision: **CHANGES REQUIRED**.
Full evidence report (commands, file:line citations, three-state tags) retained by the reviewer and available on request.