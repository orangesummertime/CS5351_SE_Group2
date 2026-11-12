# B · PaddleOCR PDF fallback notes

- **Status:** design notes only; no OCR job was submitted. No token is stored
  here.
- **Owner:** WANG Jiaqi (B).
- **Default:** local PaddleOCR runtime, used only when a PDF page has no usable
  native text layer.
- **Optional:** PaddleOCR cloud job API, only after the user explicitly selects
  it and supplies a token. It is never the default and never a silent fallback.

## 1. Default settings

| Setting | Default | Notes |
| --- | --- | --- |
| `ocrProvider` | `local` | `local` or `paddleocr_cloud`; `off` also allowed |
| `ocrRuntime` | user-installed/bundled local PaddleOCR | no token required |
| `ocrModel` | `PaddleOCR-VL-1.6` | local model path/ID; user-editable |
| `ocrEndpoint` | empty unless cloud | cloud default `https://paddleocr.aistudio-app.com/api/v2/ocr/jobs` |
| `ocrToken` | empty | only for cloud; masked/env, never logged |
| `ocrMaxJobsPerDocument` | `1` | idempotent per document hash |
| `ocrTimeoutMs` | bounded | local and cloud |
| payload flags | `false` | orientation/unwarping/chart recognition |

If the local runtime/model is missing, the status is `pdf_ocr_unavailable`; B
must not silently switch to the cloud API or upload the PDF.

Local setup guidance:

1. Install the PaddleOCR/PaddleOCR-VL runtime for the user's platform (CPU or
   GPU) and record its path in `ocrRuntimePath`.
2. Point `ocrModelPath` at the local model directory/file; the recommended model
   ID is `PaddleOCR-VL-1.6`, but the path is user-selected.
3. Click "test OCR" on a fixed small PDF; Tangentry validates runtime/model and
   page-count parsing without uploading.
4. If the runtime/model is absent or fails to start, show
   `pdf_ocr_unavailable` and setup guidance; never fall back to cloud silently.
5. Bundling a runtime/model is a later packaging decision; it requires license,
   size and platform review (B-D05). The default is user-installed.

## 2. Job flow

### Local runtime

1. Invoke the local PaddleOCR runtime/SDK with the stored PDF file and model
   path; no network and no token.
2. Parse the local result into per-page text (Markdown/text), validate the page
   count, and store it with `extractor=local-paddleocr`.
3. A missing runtime/model is `pdf_ocr_unavailable`; a crash/timeout is
   `pdf_extract_failed`.

### Cloud (opt-in only)

1. Submit a local PDF file (multipart) or a public URL to the configured job
   endpoint, default:
   `https://paddleocr.aistudio-app.com/api/v2/ocr/jobs`.
2. Send `model` (default `PaddleOCR-VL-1.6`) and optional payload flags
   (`useDocOrientationClassify`, `useDocUnwarping`, `useChartRecognition`).
3. Poll the job ID until `done` or `failed`; surface `pending`/`running`
   progress to the UI.
4. On `done`, download `resultUrl.jsonUrl` and parse each JSONL line's
   `result.layoutParsingResults[].markdown.text`.
5. Store per-page text with the document hash, extractor version, OCR job ID and
   OCR state. Validate parsed page count against the PDF page count.
6. Optional result images are not needed for retrieval; download only if a
   future reader feature needs them.

## 3. Security

- The token is a user-owned secret from local settings or an environment
  variable. It is never hardcoded, returned by an API, logged or committed.
- The settings API reports only `hasOcrToken`.
- Provider error bodies may contain private text; store a fixed error code and a
  redacted message, not the raw body.
- A token pasted into chat/docs is compromised; rotate it before use.
- Prefer a local PaddleOCR runtime for private documents; the cloud job API is
  used only with explicit user configuration and terms review.
- Cloud is off by default. Before enabling, show that PDF pages leave the
  device, the endpoint/region, the data-retention/terms link and a consent
  checkbox; store `ocrCloudConsentVersion`/timestamp. Keep the cloud option
  feature-flagged off if legal/terms review is incomplete.

## 4. Statuses and failure behavior

| Status | Meaning | User-visible action |
| --- | --- | --- |
| `pdf_extracting` | Native extraction or local/cloud OCR running | Retry later; show progress |
| `pdf_ocr_unavailable` | Local OCR runtime/model is not installed or cannot start | Explain how to install/configure local OCR; do not upload |
| `pdf_no_text` | No native text and OCR found no usable text | Explain scanned/empty PDF; offer OCR setup |
| `pdf_extract_failed` | Native failure, OCR crash/timeout, page-count mismatch | Show retry/setup; never fabricate text |

At most one active OCR job per document hash. Polling is bounded with backoff;
no unbounded automatic retries. A changed document hash invalidates old page
rows and old passage references (`stale_source`). Cloud OCR is used only after
an explicit provider switch and token consent; local OCR failure never triggers
a silent cloud upload.

## 5. License / terms review

Before shipping, confirm:

- PaddleOCR/PaddleOCR-VL model license and redistribution terms;
- cloud API terms, data-retention/region behavior and cost;
- default is user-installed local runtime; bundling requires a separate
  license/size/platform review;
- OCR quality on the fixed PDF fixtures and the page-count mapping;
- token redaction and no private document text in logs.

## 6. Test design

Local mode uses a fake local OCR runtime; cloud mode uses a fake `httptest` job
API with fixed JSONL:

- default config selects local; no cloud POST and no token access;
- local runtime missing -> `pdf_ocr_unavailable`, no cloud fallback;
- local success -> per-page text + page-count validation;
- cloud explicitly selected -> submit -> pending -> running -> done -> JSONL
  download;
- cloud failed job/timeout/page-count mismatch -> `pdf_extract_failed`;
- no-text result -> `pdf_no_text`;
- token header present but never logged/returned;
- idempotent repeat for the same document hash uses one job;
- document hash change starts a new job and invalidates old pages.

These map to B-T34/B-T37/B-T27/B-T28 in
[`docs/plans/b-retrieval-preimplementation.md`](../plans/b-retrieval-preimplementation.md).