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
