# B · Evaluation protocol for NFR-02 (hybrid retrieval)

- **Status:** protocol only. **Not executed in this session.** No provider,
  credential, network or browser was used.
- **Owner:** WANG Jiaqi (B).
- **Metric:** `gold_passage_recall_at_budget`.
- **Threshold:** ≥ 0.80 and not lower than the original head/tail baseline at
  the same character budget.
- **Evaluation set:** WANG Jiaqi will prepare the documents, questions and gold
  sentence/segment labels. The agent does not invent or approve labels.
- **Report produced later:** `docs/evidence/b-eval-report.md` (implementation
  phase 7). This file is not that report.

## 1. What is measured

For each pre-annotated long-document question, a **hit** means the exact
annotated gold sentence/segment appears in the selected context. Recall is:

```text
gold_passage_recall = number of questions with a hit / number of answerable questions
```

The primary set has at least 20 answerable questions. The result is reported as
a numerator/denominator, not only as a percentage. Every question is listed with
its hit/miss decision, selected passage IDs, retrieval mode and embedding model
hash, so a reader can audit the aggregate.

The report has three paths:

1. **baseline** — original `buildChatContext` head/tail;
2. **lexical-only** — deterministic sentence/segment candidates, no embedding
   call;
3. **hybrid** — lexical candidates + embedding rerank, plus optional text
   rerank if the configuration enables it.

NFR-02 passes only on the **hybrid** result. Lexical-only is a fallback and a
diagnostic; it is not presented as hybrid success.

This metric measures **sentence/segment recall**, not answer correctness, answer
quality or citation support. C's semantic citation verification is separate; D's
comparison quality is separate.

## 2. Dataset requirements

WANG Jiaqi prepares and confirms:

- at least 20 answerable question/document pairs;
- documents long enough to exceed the tested context budget (default 120,000
  characters; stress 8,000);
- most gold answers in the middle third; include a small number of head and tail
  controls so a method cannot pass by always taking the beginning or end;
- source coverage for article, saved subtitle and PDF page text. PDF text-layer
  pages are the primary comparison; a separate **local PaddleOCR** subset is
  labeled and reported independently. Cloud OCR is a separate opt-in run and is
  never the default. A `pdf_no_text`/`pdf_ocr_unavailable` negative case is not
  counted as a hit;
- repeated sentences/cues and at least one question whose terms appear in a
  heading but whose answer is elsewhere, to catch title-only scoring;
- at least two **zero-overlap paraphrase** pairs, where the gold middle passage
  shares no query term with the question: one same-language paraphrase and one
  cross-language pair. These are the fixtures behind AC-FR-02-19 / B-T39; they
  must be labeled before the run and must be reported separately so a lexical
  miss is not hidden inside the aggregate recall;
- at least two **distractor-saturated** documents: each contains 20 or more
  passages that do contain the query terms, so the lexical shortlist is full and
  its best candidate has nonzero coverage, plus the only correct answer as a
  zero-overlap middle passage. These are the AC-FR-02-20 / B-T40 fixtures. One
  of them must place the gold segment on a sampled index and one off it, so both
  the reachable and the coverage-limited outcome are measured;
- at least one document large enough that the deterministic semantic sweep is
  clamped by the candidate budget; the run must report `semantic_sweep_limited`
  and record the sampled index set and its size;
- at least one character-bound fixture where `maxEmbeddingChars` binds before the
  request slots; the sweep must still receive at least one passage of capacity,
  and the report must show which share shrank;
- no real user data, credentials, cookies or private URLs; use existing fixtures
  or manually constructed text.

A separate negative set covers no-match, malicious text, empty input, invalid
budget, forged locator, PDF no-text, extraction failure, embedding failure and
model mismatch. Those cases are reported separately and are not mixed into the
recall denominator.

## 3. Gold annotation format

Each label is a YAML record. A human must confirm every label before it is used.
The annotator name and date are mandatory.

```yaml
version: 1
questions:
  - question_id: BQ-001
    doc_id: fixture:article-mid
    question: "文档中段提到的检索目标是什么？"
    answerable: true
    gold_source: article
    gold_locator: "article:block-index:sentence-range"
    gold_passage_text: "【B-BASE-01 黄金答案】中段检索目标：答案位于文档中部。"
    gold_passage_sha256: "<64 lowercase hex characters>"
    gold_source_version: "content-revision:7 or text-sha256:<hex>"
    annotator: "WANG Jiaqi"
    annotated_at: "2026-11-12"
    notes: "Middle-third control; baseline is known to miss it."
```

`gold_passage_text` is the common denominator for all three paths: the baseline
has no passage IDs, while the new paths do. A hit requires the **exact
normalized gold text** to appear in the selected output (baseline) or in a
selected passage's exact text (lexical/hybrid). Normalization is limited to line
endings and whitespace collapse; it must be identical for all paths.

### Granularity

Gold passages are annotated at **sentence/segment level**:

- one self-contained answer sentence, or a short inseparable segment of 1–3
  adjacent sentences;
- for subtitles, a cue group that preserves cue IDs and time range;
- for PDF, a page-local sentence/segment.

The same splitter and segment grouping used by retrieval must be used to produce
the gold candidates, or the comparison is not valid. The reviewer must freeze
the gold set and granularity **before** running the new paths. Moving a gold
boundary after seeing a miss invalidates the run.

## 4. Same-budget comparison

All three paths receive:

- the same normalized document text and source version;
- the same optional summary material (each path applies its documented cap:
  baseline 35%, new 24,000-character optional summary);
- the same question;
- the same total character budget (default 120,000; separately 8,000);
- zero provider calls in the frozen offline run.

The original baseline is the exact `buildChatContext` behavior
([chatContext.ts:1-13](../../src/web/src/features/ai/chatContext.ts#L1-L13)).
The new paths use the six-layer assembly in
[ADR-002](../architecture/adr/002-question-relevant-retrieval.md). The total cap
is the same even though internal allocation differs; that is the "same context
character limit" requirement. The summary-cap difference is recorded so it
cannot hide a recall difference. For PDF, both paths use the **same server-side
page-text snapshot** for the comparison; the baseline is replayed on that
snapshot, not on a browser-specific extraction, so extractor differences do not
masquerade as retrieval gains.

### Unit fairness

The original function counts JavaScript UTF-16 code units; the new backend
counts Unicode code points and UTF-8 bytes. For the evaluation set:

- use BMP characters only (ASCII, Chinese, common punctuation) so UTF-16 and
  code-point counts agree;
- keep astral characters in the separate boundary test (B-T09/B-T17/B-T29);
- verify the harness records the unit it used and the resulting lengths.

If a future evaluation includes astral characters, the baseline must be replayed
with the same code-point counting rule, not the raw `String.length` value.

## 5. Embedding model and determinism

The stable default configuration is local-first: local OpenAI-compatible
embedding (`bge-m3`/1024 recommended), top-K 100, weights 0.35/0.65/0, rerank
off, B budget 24 embedding requests / 100,000 characters (191 candidates: 100
lexical + 90 sweep + question), local PaddleOCR. The
evaluation may use a different approved configuration, but it must record the
deviation.

The hybrid run must use one of:

- a pinned local embedding model, with model file hash, dimensions and runtime
  version recorded; or
- precomputed frozen vectors for the fixed evaluation set, with a vector-file
  hash; or
- an explicitly authorized remote embedding provider, with model name, endpoint
  family, request count and usage recorded (live section only).

Determinism rules:

- No network in the frozen offline run.
- The same question/candidate set + same vectors + same weights produce
  byte-identical per-question JSON.
- Fake/hash vectors are allowed only for unit tests, never for the NFR-02 claim.
- A model or dimension change invalidates the vector cache and requires a rerun.
- Record `providerKind` (local/custom/Bailian), endpoint family (not the key),
  `embeddingModel`, `dimensions`, `batchSize`, `textType`, `instruct`,
  `modelHash`, `vectorHash`, `topK` (default 100), `sweepCandidates`
  (default 90), `sweepRatio`, the derived `embeddingSlotBudget` and
  `embeddingCharBudget` and the effective lexical and sweep shares, `hybridWeights`, rerank model/`topN`/weight
  (if enabled) and the B budget limits in the report.
- If a PDF subset uses PaddleOCR, record the extractor version and OCR job
  configuration; do not pool OCR and native text pages without labeling them.
- Record `sweepCandidates`, `sweepRatio`, the derived `embeddingSlotBudget` and
  `embeddingCharBudget`, the resulting `sweepSlots`/`lexicalSlots` and
  `sweepChars`/`lexicalChars`, the spare slot count, the sampled index set,
  `stride`, `sweepCoverage`, and any zero-coverage reason (`sweep_zero_slots`,
  `sweep_disabled_by_config`). Assert and report the cap invariants
  `sweepSlots <= sweepCandidates` and `lexicalSlots <= topK`. A run with
  `sweepCoverage < 1` must not be presented as an unbounded paraphrase-recall
  claim: report the zero-overlap recall **at that coverage**, and report the
  distractor-saturated subset separately from the degenerate-lexical subset.
- Run at least one **budget-starved configuration** (for example
  `maxEmbeddingRequests=1`), one **character-bound configuration**, and one
  **high-budget configuration** (for example `maxEmbeddingRequests=25` with
  `topK=100`, `sweepCandidates=90`), and report the resulting shares. These runs
  exist to prove the shares stay nonnegative and within budget, that the sweep
  never exceeds `sweepCandidates` even when the request budget is large, and that
  character pressure does not silently take the sweep's allowance; they are not
  part of the 120k recall denominator.

## 6. Report format

The implementation phase writes `docs/evidence/b-eval-report.md` with:

1. Environment (OS, Go/Node versions, commit, command).
2. Dataset version/hash, annotator names/dates, gold granularity.
3. Provider kind, embedding/rerank model identity/hash, dimensions, batch,
   text type, instruct, vector hash, top-K, hybrid weights, rerank settings and
   B budget limits.
4. Per-question table: question ID, doc ID, budget, baseline/lexical/hybrid
   hit, selected passage IDs, mode and reason for a miss.
5. Aggregate table: numerator/denominator/recall for baseline, lexical-only and
   hybrid; difference; threshold pass/fail; hybrid-vs-lexical comparison.
6. Separate tables for 120k and 8k budgets; results are not pooled.
7. Negative-set table: no-match, malicious, empty, invalid budget, forged
   locator, PDF no-text, extraction failure, embedding failure, model mismatch.
8. Provider call/usage table for any remote embedding run, with credentials
   redacted.
9. Limitations and a clear statement that this is sentence/segment recall, not
   answer correctness.

## 7. Decision rule

The hybrid path passes NFR-02 only if all of the following hold:

- aggregate hybrid recall ≥ 0.80 on the primary 120k set;
- hybrid recall ≥ baseline recall on the same set and budget;
- hybrid recall is not lower than lexical-only on the same set; if it is, the
  result is a regression to investigate, not a pass;
- the 8k stress result is recorded and is not below the 8k baseline;
- every question is reported, with no post-hoc removal of misses;
- two frozen runs are byte-identical;
- the model/vector hashes and hybrid weights are recorded.

If the threshold is not met, the ADR must be reopened. Do not "fix" the result by
changing the gold labels, dropping hard questions, changing the budget without
reporting it, or switching from hybrid to lexical-only while calling it hybrid.

## 8. Supplementary measurements (not part of NFR-02)

- **Provider call cost:** lexical candidate generation adds zero completion
  POSTs; remote embedding adds `/embeddings` POSTs and optional rerank adds
  rerank POSTs. Record them separately under B's own retrieval budget; A's
  completion ledger is untouched. OCR jobs are recorded separately and are not
  pooled with embedding/rerank calls; local OCR has no provider call, cloud OCR
  is an explicit opt-in run.
- **Answer quality:** a small human rubric on a subset (does the answer use the
  selected evidence? unsupported claims? does it say insufficient when no
  passage exists?). Not pooled with recall.
- **Local latency/CPU:** optional; record if in-memory selection or vector
  computation becomes a bottleneck. Do not use it to excuse a recall regression.
- **Token usage:** record provider-reported usage if available; the total
  character budget is the controlled variable, not a promise of identical
  tokenization.

## 9. Live provider evaluation (future, separate)

If the hybrid path uses a remote embedder or LLM reranker:

- obtain separate explicit authorization for provider, model, credentials,
  spend ceiling and network use;
- record provider/model/endpoint, timestamp, status, request count and usage per
  call;
- redact embedding/rerank keys, PaddleOCR tokens, cookies and user data from
  retained logs;
- keep live results in a separate labeled section, never mixed with the frozen
  offline evidence;
- count embedding/reranking calls under B's separate retrieval budget; A's
  completion ledger is untouched;
- do not use live output to relabel the offline gold set.

## 10. Explicit non-claims

This document does not claim that:

- the 20+ question set exists yet;
- any recall number has been measured;
- the hybrid retrieval path is implemented;
- an embedding model or provider was called;
- PDF extraction or PaddleOCR is implemented;
- a PaddleOCR job/token was used;
- the baseline comparison has been run;
- lexical-only or hybrid is better than the other before measurement.

It is the measurement contract that must be approved before the implementation
phase and executed only after that approval.