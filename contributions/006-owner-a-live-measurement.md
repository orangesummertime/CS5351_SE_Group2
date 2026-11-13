# 006 · Owner A live usage measurement

- **Commit:** `docs: record owner A live usage measurement`; see this file's Git history.
- **Task owner:** Kefan CHEN.
- **Scope:** 46 live provider requests, redacted evidence, A plan status, and a neutral test-fixture update.
- **Review:** Independent group review of the live evidence is pending before implementation.

## What changed and why

The measurement checked whether the two selected providers actually return token usage in the response shapes A must handle. CityU DeepSeek V4 Flash and Mistral-hosted GLM 5.3 Flash each returned valid usage for 20/20 non-streaming completions. In the three streaming samples per provider, CityU returned no usage and Mistral returned valid usage. The separate arms keep the reviewed 95% primary-target non-streaming criterion from being confused with streaming behavior.

## Contribution and verification

The results justify proceeding with the reviewed strict-stop design: a stream with no valid usage must end as `usage_unknown` and must not start another request within that invocation. The per-request JSON records status and usage fields without response content or credentials. A provider-name fixture in `modelPicker.test.ts` was changed to a neutral example without altering its test cases; its focused Vitest run passed (9/9). The original OpenCode session is retained outside the repository and indexed in `docs/ai-use-log.yaml`. This contribution does not implement or test the budget feature. The next step is independent human review of the evidence before the course-specified implementation workflow.

## Presentation explanation

“I checked the usage data returned by two real providers before coding the budget feature. Both gave complete token counts for all 20 ordinary requests. CityU's stream gave none in three trials, so our design explicitly stops further requests when streaming usage is unknown. The result is a measured input to the design, not a claim that the feature already works.”
