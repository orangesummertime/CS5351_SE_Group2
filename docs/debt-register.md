# Deferred design and technical debt

This register records the group's deliberate v1 scope cuts, by owner. None is
implemented or approved as a permanent exclusion. Entries are appended per
owner; do not replace or renumber another owner's row.

## A · provider call budget

| ID | Deferred item | Why deferred for A v1 | Reopen condition | Owner |
| --- | --- | --- | --- | --- |
| A-D01 | Persistent per-request usage ledger | A v1 reports per-invocation usage without a SQLite migration; process restart loses the ledger. | Auditable history, crash recovery, cross-session reporting or course acceptance requires durable accounting. | A |
| A-D02 | Account-wide/global budget | Per-invocation isolation needs no cross-user coordinator and cannot cap total spending across simultaneous actions. | Product requires a common cost ceiling, multiple accounts/devices, or aggregate quota. | A + group architecture review |
| A-D03 | Hard token/currency ceiling | Provider usage usually arrives after completion and may be missing, so a pre-request hard ceiling cannot be guaranteed from it. | Provider offers preflight reservation/guaranteed maximum tokens or a reliable metering proxy; requirement explicitly demands this guarantee. | A + group architecture review |

## B · question-relevant retrieval

| ID | Deferred item | Why deferred for B v1 | Reopen condition | Owner |
| --- | --- | --- | --- | --- |
| B-D01 | Durable passage-level embedding index | v1 proposes a content-version-keyed vector cache, but a full ANN/vector index adds storage, rebuild and migration work; the hybrid path can start with a bounded top-K cache. | The evaluation set grows beyond the tested per-content limits, latency makes recomputation unacceptable, or multiple contents must be searched together. | B |
| B-D02 | Cross-language embedding model | v1 can use a multilingual model, but a model that reliably handles every source/target language pair adds model size, license and evaluation work. | The product requires cross-language chat and the fixed set shows a material cross-language miss that the chosen model cannot recover. | B + group architecture review |
| B-D03 | Passage-level FTS5/BM25 index | The current `library_search` row is per content item with concatenated body/subtitles and no offsets ([063_unified_search.sql:10-28](../src/internal/adapters/sqlite/migrations/063_unified_search.sql#L10-L28)); v1 generates lexical candidates in memory. | In-memory candidate generation misses the latency/size target, the corpus grows, or the team wants BM25 as a second lexical signal. | B |
| B-D04 | Online retrieval-quality monitoring | The course/offline scope has no telemetry pipeline; collecting queries, selected passages or embeddings raises privacy and storage questions. | A privacy-reviewed local metrics store or opt-in evaluation exists; the team wants drift detection after content/model changes. | B + E |
| B-D05 | Local/offline PaddleOCR runtime and OCR quality monitoring | v1 uses a native text-layer extractor and a user-configured PaddleOCR job API for scanned pages; bundling a local PaddleOCR runtime/model and measuring OCR quality is additional packaging, license and CPU work. | Offline/private OCR is required, the cloud API terms are unacceptable, or OCR page accuracy is a measured bottleneck. | B + group architecture review |
| B-D06 | LLM query expansion or agentic retrieval | Optional text rerank is in v1, but LLM query expansion/agents add provider calls, latency, nondeterminism and a prompt-injection surface. | Hybrid + rerank recall is close to the threshold and misses are dominated by vocabulary mismatch; B's budget covers the calls and a reviewed prompt/agent exists. | B |
| B-D07 | Historical content-revision backfill beyond the migration | v1 adds `content_revision`/`content_hash`, but edits made before the migration cannot be reconstructed into a full history. | C/E needs an audit trail of past edits, or imported content has an unreliable original hash. | B + C + E |
| B-D08 | Retrieval-result and embedding cache eviction policy | A cache keyed by content/question/budget/model adds invalidation, storage growth and privacy questions; v1 needs a bounded policy first. | The cache grows beyond the local storage budget, stale vectors are observed, or multiple models/content versions coexist. | B |
| B-D09 | Remote embedding/rerank provider failover and rate-limit policy | v1 can fail closed/fallback to lexical; multi-provider failover adds credentials, B-budget accounting and consistency questions. | The chosen remote provider is unreliable in practice, or the group requires a second provider/region. | B |

Reopening any item requires an updated requirement, ADR, negative tests and
independent review. This register is a planning artifact, not evidence of
implementation.

## D · Open comparison quality findings

The [retained evaluation](evidence/d-nfr-full-repeat-20261125-002.json) meets the
NFR thresholds. It still contains the following errors; passing the threshold
does not close them. Owner for each item: D.

- **D-D01 · Numerical precision.** Video/video round 1 D-20 interprets unclear
  ASR `12.1` as exact `12点1分`. Preserve uncertainty in generated facts.
- **D-D02 · Game identity uncertainty.** Video/article round 2 D-11 has a correct
  uncertain answer but a source fact asserting the game is not GTA 6 without
  evidence. Preserve uncertainty in both findings and the answer.
- **D-D04 · Entity roles.** Video/video round 1 D-04 and video/article D-02
  confuse AI discovery with verification in their answers or source facts.
  Video/article round 1 D-07 overgeneralizes code publication to both accounts.
- **D-D05 · Source-state completion.** Video/article round 1 D-13 leaves a
  ready unused article at `limited_search`. Distinguish unrelated scope from
  unavailable evidence while retaining actual failures.
- **D-D06 · Sentence presentation.** Four retained answers contain multiple
  sentences instead of the one-sentence FR contract. This is separately
  reported rather than silently waived or converted into a semantic failure. PR 12 follow-up adds a surface sentence-boundary guard and negative tests; the new full measurement meets NFR thresholds, while independent acceptance remains pending. Nine current requests return format errors rather than completed answers.
- **D-D08 · Answer completeness and scope.** Video/article D-18 omits requested
  common ground; article/article D-20 round 1 omits concrete film numbers in
  the answer; duration answers overgeneralize multiple runtime entries to all
  works. Keep answers complete and bounded by their evidence.

### Current PR12 measurement residuals

The [new complete evaluation](evidence/d-nfr-pr12-20261125-001.json) uses `openai/gpt-5.4`, revision 8, and the sentence guard. It reaches every per-round NFR threshold while retaining nine HTTP format errors. Article/article D-09 omits the GTA reader-fit inference in both rounds; video/video D-13 changes an explicit rune combination in both answers, and round 2 D-01 repeats that in a source finding; video/article round 2 D-12 and D-14 over-abstain despite explicit game-review and rating labels. These seven additional scored failure cases and all original observations remain in the evidence bundle. Passing the threshold does not close these findings or resolve historical retention and approval issues.

## E · chat evidence freshness and CI governance

| ID | Deferred item | Why deferred for E v1 | Reopen condition | Owner |
| --- | --- | --- | --- | --- |
| E-D01 | Freshness for notes saved from chat answers | Notes do not currently store a content snapshot, and changing the note schema is a separate migration and UI change. FR-05 as registered covers stored chat answers and citations. | The group requires note expiry, or notes start carrying citation/version snapshots. | E + notes owner |
| E-D02 | GitHub-enforced required checks and Issue #3 completion | In-repo workflows cannot change admin branch-protection settings. Public observation on 2026-11-20 showed required status checks off. | The repository admin enables the settings in `docs/plans/e-ci-gates.md` and a passing required run exists. | Repo admin (`orangesummertime`) + E |
| E-D03 | Backfilling snapshots for pre-E chat history | Legacy messages have no captured revision/hash; v1 already fails closed and will not display them as currently valid. Reconstructing a historical snapshot is best-effort and could be wrong. | Product or review requires a reconstructed history, or a dedicated evidence table. | E |
| E-D04 | Official OpenCode + DeepSeek V4 Flash annotated transcripts | The PR #11 implementation was prepared with Grok in Grok Bot and remains disclosed in E-AI-001; it was not produced in OpenCode. On 2026-11-20 (Hong Kong time) an official OpenCode + CityU gateway DeepSeek V4 Flash review/verification session was recorded as E-AI-002 (session `ses_e64cc8050ffe1Zrfm7Snor6D2e`, sha256 `ad8bbe11add4ac36b2789b2e54447d510c363d699abb6226c40c463d406af64b`). The raw export is retained owner-local and is not in git. This row was updated on 2026-11-22. The session is not a pre-implementation approval. | The student records official sessions and commits or retains the raw exports. The review/verification export is retained owner-local. | 刘若轩 (E) |
| E-D05 | Freshness for D comparison results | Comparison records are owned by D and may not yet carry the same snapshots. Reusing `freshness.Assess` is straightforward once those records exist. | D persists long-lived comparison evidence that users might treat as current. | D + E |
