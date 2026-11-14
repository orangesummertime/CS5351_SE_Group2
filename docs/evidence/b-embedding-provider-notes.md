# B · Embedding and rerank provider notes

- **Status:** documentation/design evidence only; no provider call was made.
- **Owner:** WANG Jiaqi (B).
- **Accessed:** 2026-11-12.
- **Sources:**
  - [Aliyun Bailian text embedding synchronous API](https://docs.bailian.console.aliyun.com/zh/model-studio/text-embedding-synchronous-api)
  - [Aliyun Bailian text rerank API](https://docs.bailian.console.aliyun.com/zh/model-studio/text-rerank-api)

This note records only public documentation facts used to design the B
embedding/rerank settings. It does not contain an API key or a live result.

## 1. Text embedding

### 1.1 API surfaces

The docs describe two equivalent surfaces:

| Surface | Endpoint / base URL | Notes |
| --- | --- | --- |
| OpenAI-compatible | `POST https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/compatible-mode/v1/embeddings` | Base URL ends in `/compatible-mode/v1`; request uses `model`, `input`, `dimensions`, optional `encoding_format` |
| DashScope | `POST https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/api/v1/services/embeddings/text-embedding/text-embedding` | `dimension`, `text_type`, `output_type`, `instruct` are DashScope parameters |

B should support **both** as provider modes, because a user may have either an
OpenAI-compatible endpoint or a DashScope endpoint. The UI must not hardcode one
workspace ID or key.

### 1.2 Documented models and dimensions

| Model | Documented dimensions | Batch/row limits | Per-row token limit |
| --- | --- | --- | --- |
| `qwen3.7-text-embedding` | 2560, 2048, 1536, 1024 (default), 768, 512, 256 | 20 rows | 128,000 |
| `qwen3.7-text-embedding-flash` | 1024 (default), 768, 512, 256 | 20 rows | 128,000 |
| `text-embedding-v4` | 2048, 1536, 1024 (default), 768, 512, 256, 128, 64 | 10 rows | 8,192 |
| `text-embedding-v3` | 1024 (default), 768, 512, 256, 128, 64 | (per docs) | (per docs) |
| `text-embedding-v2` | 1536 | 25 rows | 2,048 |
| `text-embedding-v1` | (per docs) | (per docs) | (per docs) |

Design consequence: the settings UI must treat **dimensions as a per-model
choice**, not a global constant. A custom endpoint/model may use any dimension;
B validates the returned vector length against the configured dimension and
fails closed on mismatch.

### 1.3 Parameters worth exposing

| Parameter | Surface | Why the user may customize it |
| --- | --- | --- |
| `model` | both | Local/custom/Bailian model choice |
| `input` | both | Single string or batch of strings |
| `dimensions` (OpenAI-compatible) / `dimension` (DashScope) | both | Storage size, cost, model capability; must be validated |
| `encoding_format` (`float`/`base64`) | OpenAI-compatible | Transport size; B must decode both or require `float` |
| `text_type` (`query`/`document`) | DashScope | Asymmetric retrieval quality; B sends `query` for the question and `document` for passages |
| `instruct` | DashScope | Optional query instruction; user-customizable, off by default |
| `output_type` (`dense`/`sparse`/`dense&sparse`) | DashScope | v1 uses dense only; sparse is a deferred option |
| batch size | B adapter | Provider row limits differ (10/20/25); B must respect the chosen model |
| timeout / retries | B adapter | Local servers can be slow; remote calls need bounded waits |

The response includes `data[].embedding`, `data[].index`, `model`, `usage`
(`total_tokens`; some models also return `prompt_tokens`). B records usage when
present and treats missing usage as `embedding_usage_unknown` (no further
embedding calls in that operation).

### 1.4 Default local preset

The stable default should be **local-first**, with cloud/custom opt-in:

| Setting | Default |
| --- | --- |
| `providerKind` | `local` |
| `baseUrl` | `http://127.0.0.1:11434/v1` (Ollama-style local OpenAI-compatible server) |
| `model` | `bge-m3` (recommended multilingual preset; user can change) |
| `dimensions` | `1024` (validate the returned vector length) |
| `batchSize` | `8` (bounded; user can change within the server limit) |
| `textType` / `instruct` | empty |
| `encodingFormat` | `float` |
| rerank | `off` |

This is a recommendation, not a hardcoded model: the user can point at a
different local model/server or switch to a custom/Bailian provider. B must not
silently download a model; if the local server/model is unavailable, the status
is `embedding_unavailable` and retrieval falls back to `lexical_only`.

Local setup guidance (recommended path):

1. Install a local OpenAI-compatible server. For Ollama:
   `ollama pull bge-m3` and start the Ollama service. The OpenAI-compatible base
   URL is `http://127.0.0.1:11434/v1`.
2. In Tangentry settings choose `local`, base URL
   `http://127.0.0.1:11434/v1`, model `bge-m3`, dimensions `1024`, batch `8`.
3. Click "test connection". Tangentry sends one fixed harmless string, validates
   the returned vector length and reports the model/dimension; it does not send
   a source document.
4. For LM Studio / llama.cpp / another local server, use its OpenAI-compatible
   base URL, model ID and the model's actual dimension; the values are not
   hardcoded.
5. No model is silently downloaded. If the server/model is missing, the status
   is `embedding_unavailable` and retrieval falls back to `lexical_only`.

A bundled/pinned local model is allowed only after packaging, license and model
hash review; `bge-m3` is a recommendation, not a redistribution claim. The
repository should not ship model weights without that review.

Local mode must validate that the endpoint is loopback or explicitly allowed
(the existing AI endpoint validator allows `https` or loopback `http`; see
`settings.go:171-181`). A non-loopback custom endpoint uses the remote mode with
an explicit warning.

## 2. Text rerank (optional third stage)

The docs describe:

| Surface | Endpoint | Notes |
| --- | --- | --- |
| DashScope | `POST https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank` | `input.query` + `input.documents`; `parameters.top_n`, `return_documents`, `instruct` |
| OpenAI-compatible | `POST https://{WorkspaceId}.cn-beijing.maas.aliyuncs.com/compatible-api/v1/reranks` | Flat `query`, `documents`, `top_n`, `instruct`, `return_documents` |

Documented models include `qwen3.7-text-rerank`, `qwen3-rerank`,
`gte-rerank-v2` and `qwen3-vl-rerank`; B v1 only needs text rerank. The response
contains `output.results[]` with `index` and `relevance_score` (0–1), plus
`usage`. Scores are request-relative and must not be compared across requests.

B should expose rerank as **optional**:

- `off` (default): no rerank POST;
- `bailian`: preset DashScope/OpenAI-compatible rerank;
- `custom`: user-supplied OpenAI-compatible rerank endpoint/key/model;
- `topN`, `instruct`, `returnDocuments` are user-configurable;
- rerank failure degrades to embedding/lexical ranking with an explicit status.

## 3. Security and secret handling

- The API key/token belongs in local settings or an environment variable, never
  in source, docs, fixtures or logs.
- The settings response must only report `hasKey`, never the key.
- Provider error bodies must be redacted; do not log `Authorization` headers.
- A pasted token in chat or a document is treated as compromised: revoke/rotate
  it, do not commit it.
- The docs above are public; copying their endpoint examples is fine, copying a
  real workspace ID or key is not.

## 4. B budget implication

Embedding and rerank are provider POSTs. B will use its **own** budget, separate
from A's completion budget:

- max embedding requests per retrieval operation;
- max rerank requests per retrieval operation;
- max characters/tokens sent to embeddings (when known);
- statuses: `completed`, `embedding_budget_limited`,
  `embedding_budget_exhausted`, `rerank_budget_exhausted`,
  `embedding_usage_unknown`, `embedding_unavailable`;
- no A ledger mutation; A's completion budget stays independent.

Default B budget: `maxEmbeddingChars=100000`, `maxRerankRequests=0` (rerank off),
`topK=100`, `sweepCandidates=90`, batch `8`, `lexicalWeight=0.35`,
`embeddingWeight=0.65`, `rerankWeight=0`. `maxEmbeddingRequests` is mode-specific:
**24 in `shared` mode, 25 in `split` mode**, so the 90 sweep + 100 lexical quota is
delivered by default in both modes.

**Batching mode by provider:** a local OpenAI-compatible server without
`text_type` is `shared` (question and passages batch together); DashScope/Bailian
with `text_type=query` for the question and `text_type=document` for passages is
`split` (different request-level configs cannot share a batch, so one request is
reserved for the query). A custom remote endpoint is `shared` unless it requires a
per-role `text_type`, in which case it is `split`. The adapter reports this via
`BatchingMode()` and `Embed` takes the role.

The slot budget is derived from the mode: `shared` `max(0, R*B - 1)` (question
shares a batch), `split` `max(0, (R-1)*B)` (query request reserved). With the
defaults: `shared` R=24 → `24*8-1 = 191` slots; `split` R=25 → `(25-1)*8 = 192`
slots. The character budget is `maxEmbeddingChars` minus the question,
mode-independent. Both are split by the ratio `sweepCandidates / (topK +
sweepCandidates)` = 90/190, then capped at the configured counts:
`sweepSlots = min(sweepCandidates, floor(slotBudget * 90/190))` and
`lexicalSlots = min(topK, slotBudget - sweepSlots)`, giving 90 sweep / 100 lexical
in both default modes; the character budget splits about 47,300 sweep / 52,600
lexical. Shares are nonnegative, capped at `sweepCandidates` and `topK`, and sum
to at most the slot budget with any remainder left spare. Cold-cache request count
for C embedded candidates: `shared` `ceil((1+C)/B)`, `split` `1 + ceil(C/B)`. A
small `shared` configuration such as R=1 yields 3 sweep / 4 lexical out of 7 in 1
request; a small `split` configuration R=1 yields 0 candidate slots (no query or
document request, `lexical_only`); a large `shared` R=25 (199 slots) still yields
90 sweep / 100 lexical with 9 spare, never the uncapped 94 sweep. Character
pressure shrinks both shares proportionally; it never takes the sweep's allowance,
and if the sweep share cannot hold one passage the result reports
`semantic_sweep_limited` with reason `sweep_zero_slots`.
These are conservative defaults; the user can raise/lower them, and the
evaluation report must record the actual values. If the candidate text exceeds
`maxEmbeddingChars`, the selector deterministically reduces the effective
candidate count and reports `embedding_budget_limited`. If the user enables
rerank while `maxRerankRequests=0`, the UI must require a positive rerank budget
(suggest 2) or keep rerank off.

Local embedding/rerank does not consume provider request budget but still
consumes CPU/memory and model-artifact budget; the model hash is recorded.