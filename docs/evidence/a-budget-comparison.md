# A · Fixed-input provider-call comparison

Status: **fresh deterministic measurement** on 2026-11-13, recorded with the scripted-provider harness in this directory. It compares the budget implementation checkout against the untouched upstream checkout `/home/kyle/cs5351-tangentry-baseline` (read-only). Redacted results are in [`a-budget-comparison.json`](a-budget-comparison.json). No real provider was contacted.

## What is measured (and what is not)

This is a **scripted-provider workflow measurement**, not a model-quality, prompt-decoding, or real-provider evaluation.

* The real `ai.Service.SummaryGenerate` and `ai.Service.Translate` run unmodified, including grouping, batching, checkpointing and — on the current code — the per-invocation budget.
* Each provider operation performs **one real `aichat.Client.Chat` HTTP POST** to a deterministic OpenAI-compatible `httptest` upstream, so the adapter's reservation and usage reconciliation (current) or its absence (original) run for real. The actual HTTP POST count is what is recorded.
* The fixed response is standard chat content `"ok"` with usage `prompt=2 / completion=3 / total=5`. After that real POST, a scripted wrapper returns a fixed, valid summary draft or translated segments derived from the request, exactly like the proven fakes in `src/internal/ai/budget_service_test.go` (`budgetSummarizer`, `budgetTranslator`). Workflow grouping, batching and limits remain real.
* The test asserts deterministic call behavior only. It makes **no elapsed-time improvement claim**. Elapsed times are recorded but vary with the machine and are not compared.

## Fixed inputs

| Task | Fixture | Planned provider operations |
| --- | --- | --- |
| Summary | A-T02 fixture: four ~8000-byte segments ⇒ two leaf groups (3+1) + one reduction | 3 |
| Translation | A-T03 fixture: 16 short article cues ⇒ two article batches of 8 | 2 |

The harness hashes the input text; both runs recorded identical 16-hex SHA-256 prefixes (`summary` `749b0c7205f3c779`, `translation` `eb11ca80760d40ce`), confirming the same task inputs on both checkouts. The baseline AI service and adapter files were also compared read-only against pinned upstream commit `45611ad0e214a2d9de6f0ec5a5aa84fc493d76f2`; that diff was empty.

## Measured results

| Task | Requested limit | Current POSTs | Current outcome | Upstream POSTs | Upstream outcome |
| --- | --- | --- | --- | --- | --- |
| summary | omitted | 3 | completed (3/3) | 3 | completed (3/3) |
| translation | omitted | 2 | completed (16/16) | 2 | completed (16/16) |
| summary | 1 | **1** | budget_exhausted, partial 1/3 | 3 | completed (3/3) |
| translation | 1 | **1** | budget_exhausted, partial 8/16 | 2 | completed (16/16) |
| summary | 3 | 3 | completed (3/3) | 3 | completed (3/3) |
| translation | 3 | 2 | completed (16/16) | 2 | completed (16/16) |

Observations:

* Omitted limit is wire/behavior compatible: both versions run every planned call and complete.
* The upstream checkout has no `MaxProviderCalls` field. The harness sets this property by reflection, which is a no-op when it is absent: the upstream's **applied limit is always omitted**. Its rows labelled 1 and 3 are repeated unbounded controls for the same task inputs, not actual upstream budget settings. The original JSON API rejects the unknown limit field; this measurement does not claim otherwise.
* Under limit 1 the current code starts exactly one POST and stops with a valid partial checkpoint (`budget_exhausted`); translation commits the first batch (8 of 16 texts).
* Limit 3 still completes both tasks because the plan needs ≤ 3 calls; a fully used limit is success, not exhaustion.

## Reproduce

The harness is one external test source, `a-budget-comparison_test.go.txt`, package `ai_test`. It uses only exported `ai`/`aichat`/`subtitles` API that exists in both versions, sets the optional limit by reflection on the common field name (a no-op where the field is absent), and reads the optional budget report back through a JSON map. It is compiled with `go -overlay` so no tracked file in either checkout changes.

```sh
GO=/home/kyle/.local/go1.27.1/go/bin/go
SRC=/path/to/Tangentry-group2-rebuild/src
BASE=/home/kyle/cs5351-tangentry-baseline/src
HARNESS=/path/to/Tangentry-group2-rebuild/docs/evidence/a-budget-comparison_test.go.txt

# Overlay maps a virtual file inside the existing internal/ai test package to
# the external harness source. The virtual file does not need to exist.
printf '{"Replace":{"%s/internal/ai/owner_a_compare_test.go":"%s"}}\n' "$SRC" "$HARNESS"  > /tmp/a-compare-current.json
printf '{"Replace":{"%s/internal/ai/owner_a_compare_test.go":"%s"}}\n' "$BASE" "$HARNESS" > /tmp/a-compare-baseline.json

cd "$SRC"  && OWNER_A_COMPARE_VERSION=current            $GO test -overlay /tmp/a-compare-current.json  -run '^TestOwnerAComparison$' -count=1 -v ./internal/ai
cd "$BASE" && OWNER_A_COMPARE_VERSION=baseline-upstream  $GO test -overlay /tmp/a-compare-baseline.json -run '^TestOwnerAComparison$' -count=1 -v ./internal/ai
```

Each run prints one `OWNER_A_COMPARE {...}` JSON line per (task, limit) case. Elapsed times are informational only.

## Limitations

* The frozen upstream checkout is read-only and was not modified; measurements ran through `go -overlay` virtual files only.
* The scripted provider is deterministic and makes no claim about either provider's real usage reporting. The earlier live usage-presence evidence remains the source for real provider behavior.
* No answer-quality or speed comparison is claimed.
