# A · Integration contract inventory for the provider-call budget

**Status:** inventory reviewed and merged in PR #5; live evidence reviewed and merged in PR #4. Implementation follows group confirmation of the existing A plan on 2026-11-13 and remains subject to its own PR review. This document derives the shared boundary from the [A plan](a-budget-preimplementation.md), [ADR-001](../architecture/adr/001-provider-call-budget.md), and current source.

## Existing entry points and effects

| Public entry point | Current service path | Provider path | Budget consequence from ADR-001 |
| --- | --- | --- | --- |
| `POST /api/ai/subtitles/translate` | `ai.Service.Translate` | `aichat.Client.Translate` → `complete` | Reserve before each completion POST, including recovery batches and retries. |
| `POST /api/ai/summaries/generate` | `ai.Service.SummaryGenerate` | `aichat.Client.Summarize` → `complete` | Reserve before each leaf/reduce completion POST; preserve valid partial checkpoints when stopped. |
| `POST /api/ai/chat` with JSON response | `ai.Service.Chat` | `aichat.Client.Chat` → `complete` | One reservation per started POST, including compatibility fallback. |
| `POST /api/ai/chat` with `Accept: text/event-stream` | `ai.Service.ChatStream` | `aichat.Client.ChatStream`; possible `/responses` or non-stream fallback | Reserve separately for every started POST; do not emit success `event: done` on a blocked terminal. |
| `POST /api/ai/check` | `ai.Service.Check` | Synthetic translation or summary call | Count a completion POST if one starts; zero-call rejection consumes none. |
| `POST /api/ai/models` | `ai.Service.ListModels` | Provider model discovery | No completion reservation for `GET /models`. |
| Cache/history reads and clear | `Cached`, `SummaryCached`, `ChatHistory`, `Chat` clear | No completion POST | No reservation. |

Source anchors: [routes](../../src/internal/transport/http/routes_ai.go), [chat service](../../src/internal/ai/chat.go), [summary service](../../src/internal/ai/summary_generate.go), [translation service](../../src/internal/ai/translation.go), [Check](../../src/internal/ai/check.go), and [provider transport](../../src/internal/adapters/aichat/protocol.go). `protocol.go` has four completion POST paths (`ChatStream`, `responsesStream`, `completeResponses`, and `complete`), in addition to the model-list GET. `chat.go` may fall back from an empty stream to a non-stream completion. The current `app/agent_provider.go` reports AI capability but contains no direct provider completion call; A-T22 must also cover any later Agent-derived AI entry point.

## Shared boundary for A and other owners

1. `internal/ai` owns a per-invocation, concurrency-safe ledger and the terminal state. The adapter must reserve before transport starts. A service-layer count of planned batches is insufficient because one batch may retry or fall back.
2. Existing JSON/SSE shapes remain the compatibility baseline when the optional call limit is omitted. The implementation adds optional `maxProviderCalls` (integer 1–100) to task inputs and an optional `budget` report to limited results/errors. The implementation PR reviews these additive fields; this inventory's earlier approval did not approve them in advance.
3. A result with missing, partial, malformed or inconsistent usage is `usage_unknown`, never a known zero. In that invocation, no later completion POST starts. The [CityU stream sample](../evidence/a-live-usage-presence.md) shows why this state must be visible.
4. Budget exhaustion, unknown usage, cancellation, provider failure and completed output must remain distinguishable through service, HTTP/SSE and UI. A partial summary or translation checkpoint must not be exposed as complete.
5. B, C and D may add AI-backed operations. Before any such path is implemented, its owner and A should map its invocation boundary and every provider completion POST to the same reservation rule. E's source invalidation work does not change the budget semantics by itself.

## Parallel review questions

- Confirm each new AI-backed operation's user-triggered invocation boundary before it is wired to the shared ledger.
- Confirm whether Agent-derived operations gain any new completion path; current capability reporting alone starts none.
- Review the final response and SSE field names together with the focused HTTP/UI tests, while retaining upstream behavior when the limit is omitted.

The inventory and live evidence are now merged. B, C and D must map any future completion entry points to this same boundary before integration; the current implementation does not claim to verify features that those owners have not implemented yet.
