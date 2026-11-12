# 006 · Owner B review package: hybrid question-relevant retrieval

- **Commit:** `docs: prepare owner B retrieval review package`; see this file's
  Git history.
- **Task owner:** WANG Jiaqi (B).
- **Scope:** B baseline, draft FR-02/NFR-02/INV-04/INV-05, ADR-002, hybrid
  retrieval test plan, evaluation protocol, PDF extraction design, debt
  register, course test layout and AI-use disclosure.
- **Review:** Four rounds of CHANGES REQUIRED so far: the first on `55ec38b`
  (four findings and two consistency nits), the second on `78efdca` (the sweep
  trigger could stay silent when lexical distractors fill the shortlist), the
  third on `66fc582` (the share arithmetic could go negative or overflow, and
  character pressure could starve the protected sweep), and the fourth on
  `062a21d` (the uncapped proportional split could exceed the configured
  `sweepCandidates` quota at high request budgets). All four are addressed below
  and in the follow-up commits; a fifth review is pending. No implementation,
  evaluation run or approval is claimed.

> Numbering note: this package was written as 005 while only 001 existed on
> `main` and A's 002–004 were still unmerged. A's records have since merged, so
> this package was renumbered to 006 to keep the shared index append-only.
> `main` is now fully included in this branch.

## What changed and why

The package makes B's retrieval work reviewable before any code is written:

- [context audit](../docs/evidence/b-context-audit.md) now covers the full
  component chain: settings, prompt templates, summary hooks, `AIInsights`,
  chat panel, reader workspaces, PDF reader, transport, AI service, summary
  evidence/caching, provider adapter, content/subtitle stores, SQLite and tests.
- [baseline](../docs/evidence/b-baseline.md) and
  [baseline log](../docs/evidence/b-baseline.log) reproduce the original
  head/tail loss on five fixed fixtures; all five lose a middle-only answer, and
  the four 120k cases also exceed the backend's 200,000-byte cap.
- [requirements](../docs/req/requirements.yaml) registers only B's FR-02,
  NFR-02, INV-04 and INV-05. FR-02/NFR-02 describe hybrid retrieval
  (lexical candidates + embedding rerank + optional text rerank),
  sentence/segment granularity, user-selectable local/custom/Bailian embedding
  providers (local-first default bge-m3/1024), configurable dimensions/weights,
  a B-only retrieval budget, server-side PDF text extraction with local
  PaddleOCR default/cloud opt-in and explicit fallback statuses.
- [ADR-002](../docs/architecture/adr/002-question-relevant-retrieval.md) compares
  baseline, content-level FTS/BM25, lexical-only, embedding-only and hybrid;
  selects hybrid for review; defines the D2.2 six-layer context, budget and
  compression/re-injection rules, sentence/segment passage contract, the
  namespaced `retrieval` wire envelope, PDF text-layer/PaddleOCR design,
  content-revision migration, B-only budget and invariants.
- [embedding/provider notes](../docs/evidence/b-embedding-provider-notes.md)
  records the public Aliyun Bailian embedding/rerank API facts, model/dimension
  limits and the local/custom/Bailian settings design. No live call was made.
- [PaddleOCR notes](../docs/evidence/b-paddleocr-notes.md) records the PDF OCR
  job flow, token redaction, statuses and fake-API test plan. No OCR job was
  submitted and no token is stored.
- [implementation plan](../docs/plans/b-retrieval-preimplementation.md) defines
  the hybrid interfaces, sentence/segment splitter, embedding/rerank ports,
  provider settings, B budget, PDF/PaddleOCR flow, content revision, C
  dependency, B-T01–B-T39 deterministic tests, negative coverage and phased
  verification.
- [evaluation protocol](../docs/evidence/b-eval-protocol.md) fixes the NFR-02
  dataset, gold YAML, baseline/lexical/hybrid comparison, frozen-vector
  determinism and report format. WANG Jiaqi prepares the evaluation set; no
  labels were invented here.
- [debt register](../docs/debt-register.md) records the deferred vector index,
  cross-language model, passage FTS, monitoring, local PaddleOCR runtime,
  query expansion/agents, revision backfill and cache policies with reopen
  conditions.
- [test layout](../test/README.md) adds the course-facing
  [offline](../test/offline/README.md) and [live](../test/live/README.md)
  entry points as documentation-only scaffolding.
- [AI-use log](../docs/ai-use-log.yaml) discloses this session.
- The root README B row now names WANG Jiaqi.

## Contribution to the project

This commit gives the group a concrete, testable B design without pretending the
feature exists. It separates the problem (frontend truncation and backend trust)
from the proposed solution (server-side hybrid retrieval), makes the NFR-02
comparison reproducible, and gives C a stable sentence/segment passage reference
contract. It also records the A embedding-budget, C content-revision and PDF
extraction dependencies that must be settled before code.

## Verification and limits

Actually run in this session:

- `go test ./... -count=1` — exit 0; 43 `ok` packages and 11 `[no test files]`
  packages; raw log in [b-go-full.log](../docs/evidence/b-go-full.log).
- `cd src/web && npm test` — 61 files / 231 tests passed; raw log in
  [b-vitest-full.log](../docs/evidence/b-vitest-full.log).
- `go test ./internal/adapters/sqlite -run 'TestLibrary(...)' -count=1` — PASS,
  2 tests.
- `go test -overlay=... ./internal/adapters/sqlite -run TestFTSProbe -count=1` —
  PASS; SQLite 3.53.4; FTS5 trigram + `bm25()` confirmed.
- `go test -overlay=... ./internal/ai -run TestContextByteLimitProbe -count=1` —
  PASS; 120,000 CJK characters = 360,000 bytes and `utf8ValidChat` returns
  false.
- `cd src/web && npm test -- src/features/ai/chatContext.test.ts` — PASS, 1 file
  / 2 tests.
- Node v24.20.0 baseline replay — 5 fixtures; see the log.
- YAML/JSON schema validation and relative-link checks — see the PR description;
  they are rerun before submission.

Limits:

- No retrieval implementation, no embedding/rerank call, no migration, no PDF
  extractor/OCR call, no frontend wiring, no provider call, no live evaluation
  and no browser/E2E run.
- The evaluation set and gold labels do not exist yet; WANG Jiaqi will prepare
  and confirm them.
- PDF extraction/OCR is a design proposal; local PaddleOCR is the default,
  cloud OCR is explicit opt-in. The native library and PaddleOCR/PaddleOCR-VL
  license, cloud terms, token handling and extraction quality are not yet
  reviewed.
- Embedding/rerank provider defaults (local bge-m3/1024, weights 0.35/0.65/0,
  top-K 100, sweep 90, B budget 24 requests / 100,000 chars), vector persistence
  and B budget defaults are proposed, not approved; the local runtime/model is
  not bundled.
- Content revision is adopted in the design as `content_revision` +
  `content_hash`; the migration/backfill still needs C/E review, and duplicate
  migrations must be avoided at merge.
- `test/offline` and `test/live` are documentation-only entry points; actual
  test files await approval.
- A PaddleOCR token pasted in chat was not used or committed; it must be
  rotated. Do not treat the pasted snippet as a secret store.
- The root README B owner cell is updated to WANG Jiaqi; no other task owner is
  claimed.

## Review response (PR #2 CHANGES REQUIRED)

The reviewer asked for four changes on `55ec38b`. Each was answered in its own
commit so it can be checked line by line:

1. **Semantic recall.** The pipeline previously kept only the lexical top-K
   before embedding, so a paraphrase or cross-language question with no lexical
   overlap could never reach the embedder. ADR-002 now defines a deterministic
   **bounded semantic sweep** (decision step 1b): when an embedder is configured
   and the lexical shortlist is smaller than `minLexicalCandidates` (default 20)
   or has zero distinct-query-term coverage, a position-stratified sample of the
   whole document is unioned into the candidate set, bounded by
   `maxSweepCandidates` (default 200) and the remaining B embedding budget. The
   recall claim is narrowed to "paraphrases reachable inside the sweep bound",
   `semantic_sweep_limited` reports when the budget cannot cover the document,
   AC-FR-02-19 and B-T39 cover a zero-overlap pair (same-language and
   cross-language) plus a sweep-clamped fixture, and the evaluation protocol
   requires both.
2. **Zero-request invariant vs. ordering.** Embedding and rerank do run before
   assembly, so INV-04 now has two halves backed by an explicit ordering
   contract: preflight-detectable impossibility (invalid budget, budget below the
   mandatory minimum, mandatory layers alone over the byte cap) starts zero chat,
   zero embedding and zero rerank requests; a failure that appears only during
   assembly starts zero chat requests and reports the embedding/rerank requests
   already spent, with no retry. B-T21 and B-T24 assert both halves with exact
   per-endpoint counts, and new AC-INV-04-5 binds the request-count assertions.
3. **Operation-log accuracy.** The log listed `git push` and the GitHub API PR
   create/update and then claimed no network write. The notes now say no AI
   provider was called and no browser was started, that network writes did occur
   and are listed, and what did not happen (no embedding/rerank/OCR/completion
   endpoint, no upload). A `network_writes` field was added to both B entries so
   the list and the prose cannot diverge again.
4. **Candidate limit.** The scoring section said default 100 while the pipeline
   section said top-K 200. Both now use top-K default 100 (accepted 20–1,000),
   and top-K and `maxSweepCandidates` are explicitly named as separate knobs in
   the ADR, the plan and the evaluation protocol.

Also in this revision: `main` up to `726af33` is merged in, and this record was
renumbered from 005 to 006 because A's `005-owner-a-follow-up.md` merged first.
The shared `contributions/README.md`, `docs/debt-register.md` and
`docs/req/requirements.yaml` were resolved into one owner-indexed shape: the
requirement registry now holds A's FR-01/NFR-01/INV-01–03 and B's
FR-02/NFR-02/INV-04–05 in a single `scope: group` file, with both sets unchanged
entry by entry.

## Second review round (on `78efdca`)

The reviewer accepted the zero-request, operation-log and candidate-limit fixes
and raised one remaining gap: the sweep was conditional, so in a long document
with twenty or more lexical distractors neither "shortlist too small" nor "zero
coverage" would hold, and a zero-overlap answer would still be excluded before
embedding. B-T39 only exercised the degenerate-lexical case.

- **The trigger is gone.** The sweep now runs whenever an embedder is
  configured, with no other condition. There is no `minLexicalCandidates`
  threshold left to satisfy.
- **The sweep quota is protected, not residual.** The embedding candidate budget
  is derived as `maxEmbeddingRequests * batchSize - 1` (the question), and the
  effective shares are `min(topK, budget - sweepCandidates)` and
  `min(sweepCandidates, budget - lexicalShare)`. Defaults are now 24 requests,
  100,000 characters, top-K 100 and 90 sweep candidates, giving 191 candidates =
  100 lexical + 90 sweep + question. Previously the sweep took whatever the
  lexical stage left, which is exactly the failure mode under review.
- **The claim is now a measured coverage, not a guarantee.** Every selection
  reports `stride`, `sweepCoverage = S/N` and `semantic_sweep_limited` when
  `S < N`; the requirement, the evaluation report and the user-facing copy must
  present coverage instead of claiming that any paraphrase is recoverable.
- **New AC-FR-02-20 / B-T40** uses 20+ query-term distractors plus a zero-overlap
  middle answer, and covers both outcomes: gold on a sampled index (hybrid
  selects it, lexical-only misses it) and gold off the sample (reported as a
  coverage limitation, never `no_match` or an embedder failure).
- The budget default change (16/60,000 → 24/100,000) is required by the
  arithmetic; the old default could not fund a protected sweep alongside top-K
  100.

## Third review round (on `66fc582`)

The reviewer accepted the unconditional sweep and B-T40 and found two arithmetic
defects in the share rule:

1. **Negative or overflowing shares.** With `maxEmbeddingRequests=1`,
   `batchSize=8` and `sweepCandidates=90`, the budget is 7 and
   `min(topK, 7 - 90)` is `-83`; the next formula then assigned 90 sweep
   candidates to 7 slots. A character clamp could produce the same shape.
2. **Character pressure starving the sweep.** The plan reduced sweep candidates
   before lexical ones on character overflow, which could take the "protected"
   sweep to zero while lexical distractors remained — restoring the failure the
   previous revision fixed.

Both are now fixed by replacing the subtraction with a proportional split of
**two** budgets:

```text
embeddingSlotBudget = max(0, maxEmbeddingRequests * batchSize - 1)
embeddingCharBudget = max(0, maxEmbeddingChars - questionChars)
sweepRatio          = sweepCandidates / (topK + sweepCandidates)
sweepSlots          = floor(embeddingSlotBudget * sweepRatio)
lexicalSlots        = embeddingSlotBudget - sweepSlots
sweepChars          = floor(embeddingCharBudget * sweepRatio)
lexicalChars        = embeddingCharBudget - sweepChars
```

Every share is nonnegative and the two shares sum exactly to their budget for
every configuration. The reviewer's example now yields 3 sweep / 4 lexical out of
7. Because each side stops at its own character allowance, character pressure
shrinks both sides proportionally instead of consuming the sweep, so lexical
distractors can never starve it. When the sweep share cannot hold one passage the
result reports `semantic_sweep_limited` with reason `sweep_zero_slots` and
`sweepCoverage = 0`; a deliberate `sweepCandidates=0` is a separate
`sweep_disabled_by_config` reason and makes no zero-overlap claim.

Tests: B-T33 gains a low-budget matrix (`maxEmbeddingRequests=1` → 3/4 out of 7,
nonnegative, sums exact, one request observed; `maxEmbeddingRequests=0` →
`lexical_only` with `embedding_budget_exhausted`; `maxEmbeddingRequests=-1` →
`invalid_budget`; `sweepCandidates=0` → `sweep_disabled_by_config`; a
character-bound fixture with `maxEmbeddingChars=1040` → `sweep_zero_slots` with one
lexical passage and zero sweep candidates), and B-T40 gains Variant C where the
character budget binds before the slots: both shares shrink proportionally, the
sweep still receives at least one passage, and it is never zeroed while lexical
candidates remain embedded. The evaluation protocol requires both a budget-starved
and a character-bound run, reported outside the 120k recall denominator.

A self-audit of this revision also caught two wording/scope defects, fixed in the
same follow-up: (a) B-T40 Variant C and this record initially said "the lexical
share shrinks instead of the sweep share", which contradicted the proportional
rule — both shares shrink; the sweep is protected from being *zeroed*, not from
shrinking. (b) The request-budget rule "0 = off" was not previously defined and
could be read against AC-INV-04-2/B-T10 ("budget 0 → invalid_budget"); those are
now explicitly scoped to the *context assembly* budget, while the embedding/rerank
request budgets accept 0 as off and reject negative/non-integer values.

The example configurations were also evaluated numerically before the commit: for
`requests=1`, `requests=0`, `sweepCandidates=0` and a tight character budget, the
shares stayed nonnegative and summed to the derived budget in every case.

## Fourth review round (on `062a21d`)

The reviewer accepted the low-budget and character-pressure fixes and found one
remaining high-budget defect: the uncapped proportional split
`sweepSlots = floor(embeddingSlotBudget * sweepRatio)` can exceed the configured
`sweepCandidates`. With `topK=100`, `sweepCandidates=90`, `batchSize=8`,
`maxEmbeddingRequests=25`, the slot budget is 199 and the formula gives
`floor(199*90/190)=94` sweep slots — more than the requested quota of 90, and the
gap widens with a larger request budget.

The fix keeps `sweepCandidates` as a count quota (rather than redefining it as a
ratio weight) and caps the effective shares:

```text
sweepSlots   = min(sweepCandidates, floor(embeddingSlotBudget * sweepRatio))
lexicalSlots = min(topK, embeddingSlotBudget - sweepSlots)
```

The invariants are now `sweepSlots >= 0`, `lexicalSlots >= 0`,
`sweepSlots <= sweepCandidates`, `lexicalSlots <= topK`, and
`sweepSlots + lexicalSlots <= embeddingSlotBudget`. **Unused slots stay spare**;
they are not redistributed into extra candidates beyond the configured caps, so a
larger request budget adds headroom, never more candidates than the user asked
for. The reviewer's case now gives 90 sweep / 100 lexical with 9 spare (not 94
sweep); `maxEmbeddingRequests=100` (799 slots) still gives 90 / 100 with 609 spare.
The character split stays proportional, and the sweep's count cap is what bounds
it. The ADR, plan, requirements (AC-FR-02-20), evaluation protocol and provider
notes all state the cap; B-T33 gains a high-budget case asserting
`sweepSlots == sweepCandidates` at `maxEmbeddingRequests=25` and `=100`, and the
evaluation protocol now requires a high-budget run alongside the budget-starved
and character-bound runs.

The capped formula was evaluated numerically for `requests=25`, `requests=100`,
the defaults, `requests=1`, `requests=0` and `sweepCandidates=0`; in every case the
shares were nonnegative, within their configured caps, and summed to at most the
slot budget.

## Presentation explanation

“The original chat path truncates in the browser and sends free text the server
never verifies. I reproduced four middle-answer failures and found a second
problem: the 120k frontend limit is counted in UTF-16 units while the backend
rejects over 200k UTF-8 bytes. I prepared a local-first server-side hybrid
design — lexical candidates, local embedding by default plus user-selectable
custom/Bailian embeddings, optional text rerank and configurable weights at
sentence/segment granularity — with a B-only retrieval budget, a server-side PDF
text-layer/local-PaddleOCR store with cloud OCR opt-in, a namespaced retrieval
envelope for C, and a frozen-vector evaluation protocol. No code or evaluation
is claimed; the next step is independent review, provider/OCR license decisions,
and human gold labels.”