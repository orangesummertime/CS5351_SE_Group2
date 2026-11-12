# test/live · gated provider and embedding tests

This directory is the course-facing entry point for **live** tests. It is
documentation-only in the current docs branch; no live test was run and no
provider/credential was used.

## Gate

Live tests must not run by default, not run in CI without explicit approval, and
not be mixed with offline evidence. A live run requires:

- explicit human authorization for provider, model, endpoint and purpose;
- a credentials source (environment or local secret store), never committed;
- a spend/request ceiling and a stop condition;
- redaction of keys, cookies, private source text and user identifiers;
- a separate report section labelled **live**, not pooled with `test/offline`.

## Scope

| Test | What it may call | What it must not do |
| --- | --- | --- |
| Embedding presence/shape | One approved local/custom/Bailian `/embeddings` endpoint | Send private articles or user data |
| Rerank presence/shape | One approved Bailian/custom rerank endpoint | Compare relevance scores across requests |
| Hybrid recall smoke | Fixed synthetic documents only | Relabel the offline gold set |
| Provider chat smoke | Fixed synthetic question/context | Store the key or raw provider body |
| PaddleOCR cloud smoke (opt-in) | One fixed PDF fixture and an explicitly configured user token | Commit/log the token; send private PDFs without approval |
| PaddleOCR local smoke | Fixed PDF fixtures and a local runtime | Upload the PDF or require a token |
| PDF extractor quality (if local) | Fixed PDF fixtures | Send the PDF to a third party without approval |

Every remote embedding call is a provider POST. It must be counted before it
starts under A's extended ledger or a separate embedding budget. A live run that
cannot account for the calls must not start.

## Command shape (after approval and test files exist)

```bash
# Example gate; the exact tag/flag is chosen during implementation review.
TANGENTRY_LIVE=1 \
TANGENTRY_LIVE_PROVIDER=approved-provider \
TANGENTRY_LIVE_MODEL=approved-embedding-model \
TANGENTRY_LIVE_DIMENSIONS=1024 \
TANGENTRY_LIVE_RERANK_MODEL=approved-rerank-model \
# Local OCR is the default; no token.
# Cloud OCR token only if cloud OCR is explicitly approved:
# TANGENTRY_LIVE_PADDLEOCR_TOKEN=<from secret store, never committed>
go test -tags live ./test/live/... -count=1 -v
```

Do not run this command in the docs-only branch. The environment variable names
above are illustrative; the implementation review must fix the real gate.

## Evidence rules

- Record provider/model, endpoint family, timestamp, HTTP status, request count
  and usage per call.
- Redact keys, authorization headers, cookies, private source text and local
  paths before committing logs.
- Keep live logs under `docs/evidence/` with a `live-` prefix; never merge them
  into the offline recall table.
- If a live result disagrees with the frozen offline result, report both; do not
  overwrite the frozen evidence.

## Current status

No live test file exists. The offline protocol and frozen-vector evaluation are
in [`docs/evidence/b-eval-protocol.md`](../../docs/evidence/b-eval-protocol.md).