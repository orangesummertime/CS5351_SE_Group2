# A · Live provider usage-presence measurement

Measured on 2026-11-13 for the reviewed [A protocol](../plans/a-budget-preimplementation.md). The machine sent 46 sequential completion requests with the same harmless prompt, “Reply with OK.” Each request capped output at 64 tokens. No retry followed a failure. The [redacted per-request record](a-live-usage-presence.json) contains timestamps, provider/model, HTTP status, response path, usage field names and token counts; it contains no prompt response text or credential.

| Provider and model | Response path | HTTP 200 | Valid prompt/output/total usage | Observation |
| --- | --- | ---: | ---: | --- |
| CityU gateway / `CS5351/DeepSeek-V4-Flash` | Non-streaming Chat Completions | 20/20 | 20/20 | Primary target meets the reviewed ≥95% presence threshold in this short-prompt sample. |
| Mistral via Magpie / `zai-glm-5-3-flash` | Non-streaming Chat Completions | 20/20 | 20/20 | Separate provider returned complete usage in every sample. |
| CityU gateway / `CS5351/DeepSeek-V4-Flash` | Streaming Chat Completions | 3/3 | 0/3 | No SSE event carried usage when the request omitted `stream_options`. |
| Mistral via Magpie / `zai-glm-5-3-flash` | Streaming Chat Completions | 3/3 | 3/3 | A streamed SSE event carried complete usage in each sample. |

Validity here means non-negative integer `prompt_tokens`, `completion_tokens` and `total_tokens`, with total equal to their sum. Streaming results are reported separately from the 20-request non-streaming arm. The CityU stream result supports the planned `usage_unknown` terminal state and stopping further requests in that invocation; it does not show that every CityU streaming mode lacks usage. This sample tests usage presence, not answer quality, cost enforcement or the budget implementation. No budget implementation code exists yet.

The primary-target non-streaming threshold is met. Independent human review of this live evidence and the implementation plan remains the next gate before coding.
