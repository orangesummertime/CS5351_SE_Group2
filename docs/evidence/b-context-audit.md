# B · Full-chain context and retrieval audit

- **Status:** full-chain read completed; no product code changed.
- **Owner:** WANG Jiaqi (B).
- **Baseline:** `45611ad0e214a2d9de6f0ec5a5aa84fc493d76f2`.
- **Branch:** `owner/b-retrieval`, created from `main` at `d8245ee`.
- **Date:** 2026-11-12.

This report records what the checked-out code actually does across the full
reader → summary → chat → transport → AI service → provider chain, plus the
content, subtitle, PDF, SQLite and test boundaries. Every factual claim has a
`file:line` reference. Items that were not read or not verified are listed in
§12 rather than guessed.

## 1. Full-chain map

```text
Reader workspace
  ContentReadingWorkspace / ReadingWorkspace / YouTubeReadingWorkspace / PDFReader
    -> summaryText / captions / PDF page text
    -> useSummary -> /api/ai/summaries/{inspect,generate}
    -> AIInsights -> buildChatContext (browser truncation)
    -> AIChatPanel -> POST /api/ai/chat {contentId, context, messages, model, revision}
      -> transport decode/limits
      -> ai.Service.chat
        -> stored chat history by contentId
        -> provider Chat/ChatStream (context becomes the system message)
          -> /chat/completions or /responses
            -> SSE deltas / final result
```

The important seam is that **the browser decides what the model sees**, while
the backend only checks size and NUL bytes. B's retrieval must move selection and
verification to the server.

## 2. Frontend chain (all AI components read)

### 2.1 Settings and prompt templates

`AISettings` owns provider URL/key, model selection, prompt templates and the
summary context budget:

- summary context budget defaults to 28,000 and is bounded by the input control
  to 2,000–64,000
  ([AISettings.tsx:72](../../src/web/src/features/ai/AISettings.tsx#L72),
  [AISettings.tsx:148](../../src/web/src/features/ai/AISettings.tsx#L148));
- translation, summary and chat models are selected separately
  ([AISettings.tsx:139-143](../../src/web/src/features/ai/AISettings.tsx#L139-L143));
- prompt templates are edited/validated in the browser
  ([AISettings.tsx:146-149](../../src/web/src/features/ai/AISettings.tsx#L146-L149),
  [promptTemplates.ts:57-85](../../src/web/src/features/ai/promptTemplates.ts#L57-L85));
- the save request includes `chatTemplate` and `summaryContextBudget`
  ([AISettings.tsx:94](../../src/web/src/features/ai/AISettings.tsx#L94)).

The 28,000 summary budget is **not** the chat context budget. Chat uses the
separate 120,000-character `buildChatContext` default. B must not conflate the
two.

### 2.2 Summary path

`useSummary` converts a subtitle track or text into summary input:

- subtitle cues become indexed segments with start/end/text
  ([useSummary.ts:13-16](../../src/web/src/features/ai/useSummary.ts#L13-L16));
- text becomes a single text input
  ([useSummary.ts:18-20](../../src/web/src/features/ai/useSummary.ts#L18-L20));
- `inspect` posts to `/ai/summaries/inspect`
  ([useSummary.ts:35-49](../../src/web/src/features/ai/useSummary.ts#L35-L49));
- `generate` posts to `/ai/summaries/generate` with the current AI revision
  ([useSummary.ts:87-121](../../src/web/src/features/ai/useSummary.ts#L87-L121));
- it polls `/ai/summaries/{key}` while the server reports active work
  ([useSummary.ts:103-108](../../src/web/src/features/ai/useSummary.ts#L103-L108)).

`SummaryControl` only controls summary generation/retry/cancel; it has no chat
or retrieval state ([SummaryControl.tsx:31-56](../../src/web/src/features/ai/SummaryControl.tsx#L31-L56)).

### 2.3 `AIInsights` and chat

`AIInsights` has two tabs: outline/fold and chat
([AIInsights.tsx:471-479](../../src/web/src/features/ai/AIInsights.tsx#L471-L479)).
The chat path:

- builds `documentText` from section title/topic/takeaway and unit
  topic/takeaway/key/full
  ([AIInsights.tsx:474-477](../../src/web/src/features/ai/AIInsights.tsx#L474-L477));
- calls `buildChatContext(context, documentText || overview, points)`
  ([AIInsights.tsx:477-478](../../src/web/src/features/ai/AIInsights.tsx#L477-L478));
- sends `{contentId, context, messages, model, revision}` to `/api/ai/chat`
  ([AIInsights.tsx:391](../../src/web/src/features/ai/AIInsights.tsx#L391));
- sends at most the last 39 non-empty messages
  ([AIInsights.tsx:391](../../src/web/src/features/ai/AIInsights.tsx#L391));
- limits the composer to 12,000 characters
  ([AIInsights.tsx:462](../../src/web/src/features/ai/AIInsights.tsx#L462));
- renders assistant Markdown with `react-markdown`/GFM
  ([AIInsights.tsx:448-454](../../src/web/src/features/ai/AIInsights.tsx#L448-L454));
- has no passage/retrieval-status display and no citation object
  ([AIInsights.tsx:442-468](../../src/web/src/features/ai/AIInsights.tsx#L442-L468)).

The summary document `unit.full` is source text, so the 35% summary material can
contain a large duplicate of the source rather than a compressed summary. This
can crowd out the head/tail source even before the 120k split.

`chatPresentation` only remembers the selected chat model in local storage
([chatPresentation.ts:8-25](../../src/web/src/features/ai/chatPresentation.ts#L8-L25));
`modelPicker` filters translation-only models out of chat
([modelPicker.ts:50-63](../../src/web/src/features/ai/modelPicker.ts#L50-L63)).
`summaryMarkdown` renders summary evidence with time/page links and exact quotes
([summaryMarkdown.ts:32-80](../../src/web/src/features/ai/summaryMarkdown.ts#L32-L80),
[summaryMarkdown.ts:87-135](../../src/web/src/features/ai/summaryMarkdown.ts#L87-L135)),
which is a useful precedent for C's citation UI.

### 2.4 Reader context sources

| Reader | Context source | Reference |
| --- | --- | --- |
| Article / image / discussion | `discussionText` or `item.description` or joined `articleBlocks` text/alt or image alt; joined with saved caption cue text | [ContentReadingWorkspace.tsx:66-72](../../src/web/src/app/ContentReadingWorkspace.tsx#L66-L72) |
| YouTube | Saved caption cues joined by newline, otherwise item description | [YouTubeReadingWorkspace.tsx:48-51](../../src/web/src/app/YouTubeReadingWorkspace.tsx#L48-L51) |
| Bilibili / local video | Caption cues joined by newline, otherwise video description | [ReadingWorkspace.tsx:41-56](../../src/web/src/app/ReadingWorkspace.tsx#L41-L56) |
| PDF | Browser-extracted page texts joined by newline | [PDFReader.tsx:34](../../src/web/src/features/documents/PDFReader.tsx#L34), [PDFReader.tsx:51-57](../../src/web/src/features/documents/PDFReader.tsx#L51-L57), [PDFReader.tsx:148](../../src/web/src/features/documents/PDFReader.tsx#L148) |

PDF also builds `summarySegments` from page text and passes them to `useSummary`
([PDFReader.tsx:141-143](../../src/web/src/features/documents/PDFReader.tsx#L141-L143)).
The PDF text is not persisted on the server; a server-authoritative PDF passage
needs the extraction/store design in ADR-002.

The reader composition (`MediaReader`, `ReadingNotebook`, `SubtitleReader`,
`ArticleReader`, `FullTextArticle`) was also read. It passes the selected
content/track/PDF to the AI panel, but it does not select passages; that remains
the backend's job. `ArticleReader` builds `summaryBlocks` from `ArticleBlock`s
([ArticleReader.tsx:76-85](../../src/web/src/features/reader/ArticleReader.tsx#L76-L85)),
and `FullTextArticle` can fetch an `ArticleDocument` with `sourceHash` and
`revision` ([FullTextArticle.tsx:54-63](../../src/web/src/features/reader/FullTextArticle.tsx#L54-L63));
the chat path does not use that document revision today.

### 2.5 Frontend test coverage

`chatContext.test.ts` has only two tests: summary-before-source and a bounded
head/tail marker check
([chatContext.test.ts:4-15](../../src/web/src/features/ai/chatContext.test.ts#L4-L15)).
It does not test a middle answer, a CJK/emoji boundary, or the backend byte cap.

## 3. Backend AI chain

### 3.1 Transport

- `POST /api/ai/chat` and `GET /api/ai/chat` are registered in
  [routes_ai.go:19-20](../../src/internal/transport/http/routes_ai.go#L19-L20).
- `aiChat` decodes `ai.ChatRequest` with a 512 KiB body cap
  ([ai_summary.go:112-130](../../src/internal/transport/http/ai_summary.go#L112-L130)).
- The strict decoder requires `application/json`, rejects unknown fields and
  trailing data, and maps `MaxBytesError` to 413
  ([ai_common.go:24-66](../../src/internal/transport/http/ai_common.go#L24-L66)).
- SSE sets headers, flushes, emits `data: {"delta":...}` events, then
  `event: done` with the final result; an error after headers is sent as a
  `data: {"error":...}` event
  ([ai_summary.go:132-168](../../src/internal/transport/http/ai_summary.go#L132-L168)).
- `chatFail` maps `ErrInvalid` to 400 but uses the generic chat error message;
  `ErrInvalid` is not listed in `aiErrorMessage`, so a byte-cap rejection can
  read as "上游服务暂时不可用"
  ([ai_errors.go:59-99](../../src/internal/transport/http/ai_errors.go#L59-L99)).

### 3.2 Chat service

`ChatRequest` is `{contentId, context, messages, revision, model, clear}`
([translation_types.go:32-39](../../src/internal/ai/translation_types.go#L32-L39)).
`Chat`/`ChatStream` call the same `chat` method
([chat.go:36-46](../../src/internal/ai/chat.go#L36-L46)):

- validates contentID, context, message count
  ([chat.go:46-49](../../src/internal/ai/chat.go#L46-L49));
- checks `revision` against the AI configuration revision
  ([chat.go:61](../../src/internal/ai/chat.go#L61));
- uses `contentId` for the history key
  ([chat.go:14-16](../../src/internal/ai/chat.go#L14-L16),
  [chat.go:68](../../src/internal/ai/chat.go#L68));
- loads stored history, replaces it with client messages when provided, and
  requires the last message to be a user message
  ([chat.go:75-86](../../src/internal/ai/chat.go#L75-L86));
- streams via `StreamChatter`, falls back to non-stream `Chat` if the stream
  produced no content and the error is `ErrFormat`/`ErrProvider`
  ([chat.go:87-117](../../src/internal/ai/chat.go#L87-L117));
- stores the completed assistant message and returns it
  ([chat.go:121-126](../../src/internal/ai/chat.go#L121-L126));
- `utf8ValidChat` enforces context ≤200,000 bytes, no NUL, at most 40 messages,
  each message non-empty and ≤12,000 bytes
  ([chat.go:138-147](../../src/internal/ai/chat.go#L138-L147)).

The service never loads the content item to verify that `context` came from it.
History is keyed by contentID only; model and retrieval mode are not part of the
key, so a future retrieval-mode toggle can mix old and new contexts unless the
history schema or key changes.

### 3.3 Summary service and evidence precedent

Summary is a mature, relevant precedent for B:

- `SummarySegment` already carries `Index`, `Start`, `End`, `Page`, `Kind`,
  `Heading`, `SegmentIndexes`, `CueIDs` and `Text`
  ([summary_types.go:10-20](../../src/internal/ai/summary_types.go#L10-L20));
- `normalizeSummaryInput` validates segments, cue IDs, page/time ranges and
  total bytes ([summary_validation.go:13-111](../../src/internal/ai/summary_validation.go#L13-L111));
- `summaryKey` hashes the source, segments, prompt, user prompt and budget
  ([summary_validation.go:117-142](../../src/internal/ai/summary_validation.go#L117-L142));
- `summaryPlan` groups segments by byte/count and reduces in groups of six
  ([summary_validation.go:177-204](../../src/internal/ai/summary_validation.go#L177-L204));
- `buildCitations` maps model indices back to exact segment text/start/end/page
  ([summary_validation.go:269-287](../../src/internal/ai/summary_validation.go#L269-L287));
- `validSummaryEvidence` requires the quote to equal the source segment text and
  the start/end/page to match
  ([summary_validation.go:388-396](../../src/internal/ai/summary_validation.go#L388-L396));
- `summaryDocument` projects model output into deterministic IDs derived from
  source positions; `sourceRangeID` builds `start-end`/disjoint ranges
  ([summary_document.go:12-15](../../src/internal/ai/summary_document.go#L12-L15),
  [summary_document.go:392-439](../../src/internal/ai/summary_document.go#L392-L439));
- summary generation is resumable and checkpoints leaf/reduce drafts
  ([summary_generate.go:94-252](../../src/internal/ai/summary_generate.go#L94-L252),
  [summary_generate.go:380-515](../../src/internal/ai/summary_generate.go#L380-L515));
- the summary cache is keyed by text/prompt/budget, not content ID
  ([summary_store.go:23-60](../../src/internal/ai/summary_store.go#L23-L60)).

This is the strongest existing evidence that Tangentry can validate exact source
ranges. B should reuse the **idea** of deterministic source-derived IDs and exact
quote validation, but not the summary cache as a chat source.

### 3.4 Settings and model selection

- `validConfig` validates endpoint, models, prompts, summary budget and
  revision ([settings.go:186-199](../../src/internal/ai/settings.go#L186-L199));
- `chatModel` returns only an explicitly configured, enabled model
  ([settings.go:205-212](../../src/internal/ai/settings.go#L205-L212));
- summary budget is 2,000–64,000 with a 28,000 default
  ([settings.go:231-235](../../src/internal/ai/settings.go#L231-L235));
- `summaryConfig` swaps in the summary model
  ([settings.go:237-242](../../src/internal/ai/settings.go#L237-L242));
- status exposes `SummaryReady`, `Ready`, `Revision` and templates
  ([settings.go:200-203](../../src/internal/ai/settings.go#L200-L203)).

### 3.5 Provider adapter

- `Chat`/`ChatStream` build the message list with the source-dependent system
  message first ([protocol.go:100-125](../../src/internal/adapters/aichat/protocol.go#L100-L125)).
- `chatSystemMessage` renders `templates.Chat.User` with `content=source` and
  prefixes `templates.Chat.System`
  ([protocol.go:169-175](../../src/internal/adapters/aichat/protocol.go#L169-L175)).
- `ChatStream` uses `/chat/completions`, falls back to `/responses` on 404, and
  disables redirects/cookie jars
  ([protocol.go:116-167](../../src/internal/adapters/aichat/protocol.go#L116-L167)).
- `complete` uses `/chat/completions`, falls back to `/responses` on 404,
  classifies 400/401/403/422/429/504, caps the response body at 256 KiB, and
  treats `finish_reason=length`/refusal as errors
  ([protocol.go:354-437](../../src/internal/adapters/aichat/protocol.go#L354-L437)).
- The four completion POST sites are at lines 127, 181, 250 and 356
  ([protocol.go:127](../../src/internal/adapters/aichat/protocol.go#L127),
  [protocol.go:181](../../src/internal/adapters/aichat/protocol.go#L181),
  [protocol.go:250](../../src/internal/adapters/aichat/protocol.go#L250),
  [protocol.go:356](../../src/internal/adapters/aichat/protocol.go#L356)).
- There is no `/embeddings` call in the current adapter.

### 3.6 Test coverage

- `TestChatContractExplicitModelPrivateHeadersAndBoundedRequests` covers the
  provider chat contract and no private cookie/key leak
  ([client_test.go:352-385](../../src/internal/adapters/aichat/client_test.go#L352-L385)).
- `TestListModelsAndResponsesFallback` covers Chat's `/responses` fallback
  ([protocol_stream_test.go:14-39](../../src/internal/adapters/aichat/protocol_stream_test.go#L14-L39)).
- `ai_test.go` covers summary/translation/check HTTP behavior, but there is no
  `/api/ai/chat` HTTP test and no `ai.Service.Chat` unit test in this checkout
  (repository search; `grep -rn "ai/chat" src/internal/transport/http/*_test.go`
  returns no match).
- `chatContext.test.ts` covers only two pure-function cases
  ([chatContext.test.ts:4-15](../../src/web/src/features/ai/chatContext.test.ts#L4-L15)).

This is a real gap for B: the chat path has less test coverage than summary and
translation.

## 4. Byte, rune and Unicode findings

### 4.1 Character budget vs byte cap

The frontend counts JavaScript `String.length` (UTF-16 code units). The backend
counts Go `len(string)` (UTF-8 bytes). A probe compiled against the pinned
`internal/ai` package:

```text
cjk_chars=120000 cjk_bytes=360000 valid=false
ascii_chars=120000 ascii_bytes=120000 valid=true
```

Source: [b-context-byte-probe.log](b-context-byte-probe.log). The CJK string is
rejected solely because of [chat.go:139](../../src/internal/ai/chat.go#L139).

### 4.2 Surrogate split

A separate Unicode probe found another boundary defect:
`buildChatContext('😀'.repeat(200), '', [], 79)` returns a string for which
JavaScript's `String.prototype.isWellFormed()` is `false`; the head/tail `slice`
split a surrogate pair. The exact result depends on budget parity (80 was
well-formed in the probe, 79/81/82/83 were not). See
[b-unicode-probe.log](b-unicode-probe.log).

### 4.3 Message byte cap

The composer allows 12,000 characters
([AIInsights.tsx:462](../../src/web/src/features/ai/AIInsights.tsx#L462)), but
the backend caps each message at 12,000 **bytes**
([chat.go:143](../../src/internal/ai/chat.go#L143)). A 12,000-character CJK
message is up to 36,000 bytes and is rejected. The new design should validate
UTF-8 bytes before sending and show a specific error.

## 5. Content structures and locators

### 5.1 Article and discussion

`content.Item` carries `ArticleBlocks []ArticleBlock` and an optional
`Discussion *Discussion` ([content_types.go:3-35](../../src/internal/content/content_types.go#L3-L35)).
`ArticleBlock` has `Kind`, `Level`, `Group`, `Ordered`, `Text`, `Summary`, `URL`,
`Alt`, dimensions and inline images
([content_types.go:86-98](../../src/internal/content/content_types.go#L86-L98)).
There is no block ID, no byte/rune offset and no content revision in the struct.

`Discussion` contains posts, and each post contains `Blocks []ArticleBlock`
([content_types.go:60-81](../../src/internal/content/content_types.go#L60-L81)).
A stable locator can be `(contentId, "article", blockIndex, sentenceIndex,
startRune, endRune)` or `(contentId, "discussion", postIndex, blockIndex,
sentenceIndex, startRune, endRune)`.

Limits relevant to sizing and tests:

- at most 4,000 article blocks per item
  ([content_validation.go:26-28](../../src/internal/content/content_validation.go#L26-L28));
- paragraph/quote/code/table/list block text at most 20,000 bytes
  ([content_validation.go:86-94](../../src/internal/content/content_validation.go#L86-L94));
- description at most 20,000 bytes, except RSS articles up to 1 MiB
  ([content_validation.go:10-14](../../src/internal/content/content_validation.go#L10-L14));
- discussion posts at most 200, each with at most 4,000 blocks
  ([content_validation.go:29-43](../../src/internal/content/content_validation.go#L29-L43)).

### 5.2 Subtitles

`Item.Transcripts` is only link metadata: `URL`, `Type`, `Language`
([content_types.go:108-112](../../src/internal/content/content_types.go#L108-L112)).
It does **not** contain cues.

Actual cues live in `subtitles.Track.Cues`:

- `Cue{Start float64, End float64, Text string}`
  ([subtitles.go:24-28](../../src/internal/subtitles/subtitles.go#L24-L28));
- `Track{Language, Label, Automatic, Source, Translated, SourceLanguage, Mismatch, Cues}`
  ([subtitles.go:30-39](../../src/internal/subtitles/subtitles.go#L30-L39));
- saved as JSON in `saved_subtitles.payload` under `$.track.cues`
  ([saved.go:22-28](../../src/internal/subtitles/saved.go#L22-L28));
- table primary key `(content_id, language)`
  ([006_baseline.sql:13](../../src/internal/adapters/sqlite/migrations/006_baseline.sql#L13));
- save validation allows at most 10,000 cues, 8,000 bytes per cue and 2 MiB
  total text ([saved.go:53-70](../../src/internal/subtitles/saved.go#L53-L70));
- the saved row revision increments on update and a unique partial index allows
  one selected track per content
  ([saved_subtitles.go:54-58](../../src/internal/adapters/sqlite/saved_subtitles.go#L54-L58),
  [081_selected_subtitle_index.sql:1](../../src/internal/adapters/sqlite/migrations/081_selected_subtitle_index.sql#L1));
- delete keeps a revision tombstone with empty payload
  ([saved_subtitles.go:71-85](../../src/internal/adapters/sqlite/saved_subtitles.go#L71-L85)).

A subtitle source version is therefore available: `(content_id, language,
revision)`. B should use it instead of `saved_at`.

### 5.3 PDF

PDF page text is produced in the browser from pdf.js
([PDFReader.tsx:51-57](../../src/web/src/features/documents/PDFReader.tsx#L51-L57))
and passed to summary/chat as joined text
([PDFReader.tsx:141-143](../../src/web/src/features/documents/PDFReader.tsx#L141-L143),
[PDFReader.tsx:148](../../src/web/src/features/documents/PDFReader.tsx#L148)).
There is no server-side PDF page-text table. The B design adds `pdf_page_text`,
a native text-layer extractor and a **local PaddleOCR** fallback for pages
without a usable text layer. Cloud PaddleOCR is an explicit opt-in: the user
must switch provider, supply a token and accept the cloud terms; a missing local
runtime returns `pdf_ocr_unavailable` and never silently uploads. The token is
user-supplied and redacted; a token pasted into chat/docs is treated as
compromised. OCR jobs are bounded and page-count-validated; failures return
`pdf_no_text`/`pdf_extract_failed`, never fabricated text. See ADR-002 and
[PaddleOCR notes](b-paddleocr-notes.md).

### 5.4 Content version

`content.Service.Save` sets `SavedAt` before the repository call
([content.go:40-49](../../src/internal/content/content.go#L40-L49)). The SQLite
upsert updates only `payload` on conflict:

```sql
INSERT INTO saved_content(id,payload,saved_at) VALUES(?,?,?)
ON CONFLICT(id) DO UPDATE SET payload=excluded.payload
```

([store.go:91-103](../../src/internal/adapters/sqlite/store.go#L91-L103)).
`saved_at` is therefore the original insertion time, not the last content
revision. `content.Item` has no revision field. A separate `ArticleDocument`
model has `sourceHash` and `revision`, and `FullTextArticle` uses them for
article full-text fetching/conflict checks
([shared/api/types.ts:37-47](../../src/web/src/shared/api/types.ts#L37-L47),
[FullTextArticle.tsx:54-63](../../src/web/src/features/reader/FullTextArticle.tsx#L54-L63)),
but that is a different article-extraction model, not `content.Item`, and the
chat path does not use it. It could inform a per-source version for RSS articles,
but it is not a universal content revision.

C's long-lived citation needs a source version. B adopts the recommended method:
add `content_revision` and `content_hash` to `saved_content`, increment the
revision and replace the hash on every successful update, backfill legacy rows,
expose both in `content.Item`, and use `content-revision:<n>` as the passage
`sourceVersion`. A text hash remains only as a legacy/PDF/OCR fallback. This is
a cross-owner migration with C and E.

## 6. Search infrastructure

### 6.1 Schema

The current search table is built by migration 063:

- FTS5 virtual table `library_search` with columns `id UNINDEXED`, `title`,
  `author`, `description`, `body`, `subtitles`
  ([063_unified_search.sql:10-18](../../src/internal/adapters/sqlite/migrations/063_unified_search.sql#L10-L18));
- tokenizer `trigram` (same lines);
- one row per saved content item (`id` is the content ID);
- `body` is `group_concat` of `$.articleBlocks[*].text`
  ([063_unified_search.sql:26](../../src/internal/adapters/sqlite/migrations/063_unified_search.sql#L26));
- `subtitles` is `group_concat` of all `$.track.cues[*].text` across saved
  subtitle rows, without language separation
  ([063_unified_search.sql:27](../../src/internal/adapters/sqlite/migrations/063_unified_search.sql#L27));
- triggers keep the row in sync on content and subtitle changes
  ([063_unified_search.sql:30-64](../../src/internal/adapters/sqlite/migrations/063_unified_search.sql#L30-L64)).

The earlier migration 028 created a title/author/description-only trigram table
([028_library_search.sql:3-16](../../src/internal/adapters/sqlite/migrations/028_library_search.sql#L3-L16));
063 drops and rebuilds it. The latest migration in this checkout is
[081_selected_subtitle_index.sql](../../src/internal/adapters/sqlite/migrations/081_selected_subtitle_index.sql).
A new PDF/embedding table would need migration 082/083 and the append-only rule
in [ARCHITECTURE.md:120-125](../ARCHITECTURE.md#L120-L125).

### 6.2 Query behavior

`libraryFilter` splits the query on whitespace:

- terms with at least 3 runes are quoted and sent to `library_search MATCH`
  ([library.go:89-93](../../src/internal/adapters/sqlite/library.go#L89-L93),
  [library.go:98-100](../../src/internal/adapters/sqlite/library.go#L98-L100));
- shorter terms fall back to `LIKE` over `title`, `author`, `description`,
  `body` and `subtitles`
  ([library.go:94-97](../../src/internal/adapters/sqlite/library.go#L94-L97));
- all terms are ANDed ([library.go:98-101](../../src/internal/adapters/sqlite/library.go#L98-L101)).

The repository contains no `bm25()` call; only `MATCH` is used in
`library.go` and `subscription_inbox.go`. A probe against the pinned SQLite
3.53.4 build confirmed that FTS5 trigram and `bm25()` work:

```text
sqlite_version=3.53.4
match="文档中段答案" bm25=-0.28095409307129493
```

Source: [b-fts-probe.log](b-fts-probe.log).

### 6.3 Can the existing index score passages?

No. It stores one concatenated row per content item, with no sentence index,
cue index, language or offset. `bm25()` would score the whole content item, not a
sentence/segment. The v1 hybrid design therefore generates lexical candidates
in memory from the normalized source; a passage-level FTS index remains a
documented fallback in the [debt register](../debt-register.md).

## 7. Layering and placement

The dependency rules are documented in [ARCHITECTURE.md:32-45](../ARCHITECTURE.md#L32-L45)
and enforced by [layering_test.go:65-96](../../src/internal/layering/layering_test.go#L65-L96):

- `transport` may import domain types/interfaces, not `app`, `server` or
  `adapters`;
- `adapters` may import domain packages, not `app`, `server` or `transport`;
- domain packages may import other domain packages and the standard library,
  not outer layers;
- `app` is the composition root.

The proposed placement is valid:

- `internal/retrieval` defines `PassageSource`, `Embedder`, `Reranker`,
  `PDFTextExtractor` and the pure selector/assembler;
- `internal/adapters/sqlite` implements the source/page/vector stores;
- `internal/adapters/aichat` (or a small `embedding` adapter) implements
  local/custom/Bailian `/embeddings` and optional rerank behind the ports;
- `internal/adapters/pdftext` implements native PDF page extraction;
- `internal/adapters/paddleocr` implements the PaddleOCR job submit/poll/JSONL
  adapter;
- `internal/ai` calls the retrieval domain;
- `internal/app` wires the adapters and settings;
- the frontend only displays status/references and edits provider settings.

The current AI service has no content/embedding dependency: `ai.New` takes a
result store, translator and optional summarizers
([translation.go:16-20](../../src/internal/ai/translation.go#L16-L20)), and the
service struct has no content, subtitle or embedding field
([translation_types.go:84-98](../../src/internal/ai/translation_types.go#L84-L98)).
New ports can be injected without importing an adapter. The app wiring already
constructs `savedCaptions` and `contentService`
([feature_services.go:75-76](../../src/internal/app/feature_services.go#L75-L76))
and the AI service at [feature_services.go:138](../../src/internal/app/feature_services.go#L138).

Putting retrieval in `chatContext.ts` would violate the "service verifies actual
source" task in [README.md:64-69](../../README.md#L64-L69), because the backend
would still trust free text.

## 8. Test conventions and `test/offline` / `test/live`

### 8.1 Existing conventions

| Layer | Convention | Evidence |
| --- | --- | --- |
| Go unit/integration | Colocated `*_test.go`, `go test ./...`, `httptest` for HTTP/provider fakes | [ARCHITECTURE.md:78-86](../ARCHITECTURE.md#L78-L86), [protocol_stream_test.go:14-39](../../src/internal/adapters/aichat/protocol_stream_test.go#L14-L39), [client_test.go:352](../../src/internal/adapters/aichat/client_test.go#L352) |
| Frontend unit | Colocated `*.test.ts`, Vitest via `npm test` | [package.json:10](../../src/web/package.json#L10), [chatContext.test.ts:4-15](../../src/web/src/features/ai/chatContext.test.ts#L4-L15) |
| E2E | Playwright under `src/web/tests`, only core journeys | [ARCHITECTURE.md:88-107](../ARCHITECTURE.md#L88-L107) |
| Layering | `go test` walks production imports and fails violations | [layering_test.go:65-96](../../src/internal/layering/layering_test.go#L65-L96) |

### 8.2 Course layout

The course-required `test/offline` and `test/live` directories are added as
documentation-only scaffolding in this branch:

- `test/offline/README.md` maps deterministic Go/Vitest suites, fixtures and
  B-Txx tests; it requires no credentials or network;
- `test/live/README.md` documents the gated provider/embedding protocol,
  credentials, spend ceiling, redaction and separation from offline evidence.

Actual `.go`/`.ts` test files are intentionally not created in this docs-only
branch. After approval, offline tests can live in `test/offline` as thin wrappers
that call the colocated domain suites, or remain colocated with a `test/offline`
command index. Do not move upstream tests just to satisfy a directory shape.

## 9. Interfaces with A and C

### 9.1 A's budget and B's separate retrieval budget

A's proposed budget counts provider completion POST starts. The adapter has four
such POST sites
([protocol.go:127](../../src/internal/adapters/aichat/protocol.go#L127),
[protocol.go:181](../../src/internal/adapters/aichat/protocol.go#L181),
[protocol.go:250](../../src/internal/adapters/aichat/protocol.go#L250),
[protocol.go:356](../../src/internal/adapters/aichat/protocol.go#L356)).
The chat service can also fall back from an empty stream to a non-stream call
([chat.go:96-97](../../src/internal/ai/chat.go#L96-L97)), which is another
completion start and must be reserved under A's rule.

B's default is local-first: local OpenAI-compatible embedding (recommended
bge-m3/1024), top-K 100, weights 0.35/0.65/0, rerank off, B budget 16
embedding requests / 60,000 characters. *(Design updated after this audit: the
default is now 24 requests / 100,000 characters with a protected 90-candidate
semantic sweep, so that a zero-overlap paraphrase can reach the embedder; see
the [implementation plan](../plans/b-retrieval-preimplementation.md#relation-to-as-budget-b-has-a-separate-retrieval-budget)
and ADR-002. The audit findings above are unchanged by that update.)* A remote/custom/
Bailian provider is opt-in. B's hybrid path adds `/embeddings` and optional
rerank POSTs when a remote provider is used; those are **not** covered by A's
current completion-only invariant. To avoid touching A's branch, B uses its
**own** retrieval budget: max embedding/rerank requests and characters, separate
statuses/UI/tests, and no reads or writes of A's ledger. B must not enable remote
embedding/rerank until its own budget is approved. A local embedder/reranker
consumes no provider request budget but still needs a model artifact hash and
CPU/memory accounting. Provider choices, dimensions and API facts are in
[embedding/provider notes](b-embedding-provider-notes.md).

### 9.2 C: citation verification

The current chat message is only `{role, content, model?}`
([translation_types.go:27-31](../../src/internal/ai/translation_types.go#L27-L31));
the chat result is only `contentId`, `messages`, `updatedAt`
([translation_types.go:40-44](../../src/internal/ai/translation_types.go#L40-L44)).
There is no citation or passage field to reuse.

B's proposed passage reference (ADR-002) contains `passageId`, `contentId`,
source kind, locator, sentence/segment range, rune range, text hash, source
version and cue IDs. C can re-read the source and verify the range/hash without
trusting the model. B recommends a single namespaced response object,
`retrieval: {schemaVersion, status, mode, passages, embedding, rerank}`, so it
does not collide with A's `usage`/`budget`, C's future `citations` or D's
comparison fields. C consumes `retrieval.passages` and must still confirm the
final spelling.

### 9.3 Content revision for C and E

C needs a source version; E needs stale-state detection. The current content
store has no revision, and `saved_at` is not updated by the upsert
([store.go:91-103](../../src/internal/adapters/sqlite/store.go#L91-L103)).
B adopts the recommended `content_revision`/`content_hash` migration, including
legacy backfill and `content-revision:<n>` in the passage reference. This is a
cross-owner dependency with C and E; B implements the storage/version plumbing,
C/E consume it. Migration numbers are file ordering, not a runtime contract:
B's schema/backfill should land before or with C/E code that reads the field; if
C/E merge first, their code stays behind an interface/feature flag; if `main`
already uses B's numbers, B renumbers at merge and never edits a published
migration. B-T38 covers the old-DB upgrade/backfill.

## 10. Baseline anchor

The baseline is recorded in [b-baseline.md](b-baseline.md) and
[b-baseline.log](b-baseline.log). Five fixed cases were run against the exact
`buildChatContext` source:

| Case | Kind | Input chars | Answer offset | Budget | Output chars | Output bytes | Answer retained |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- |
| B-BASE-01 | article | 156,032 | 78,000 | 120,000 | 119,972 | 230,724 | no |
| B-BASE-02 | subtitles | 144,011 | 71,976 | 120,000 | 119,972 | 289,932 | no |
| B-BASE-03 | PDF pages | 160,039 | 79,121 | 120,000 | 119,972 | 358,022 | no |
| B-BASE-04 | article + summary | 132,032 | 66,000 | 120,000 | 119,973 | 207,237 | no |
| B-BASE-05 | article (8k stress) | 156,024 | 78,000 | 8,000 | 7,972 | 15,338 | no |

All four 120k cases would also exceed the backend's 200,000-byte cap before the
provider is called. The 8k case fits the byte cap but still loses the middle
answer. This is the comparison anchor for NFR-02.

## 11. Design consequences

1. **Server-side authority is required.** The current backend cannot verify that
   `context` matches `contentId`. Retrieval must load the source in the domain
   layer and re-read every selected range.
2. **Hybrid retrieval is chosen, local-first.** Lexical candidates are local
   and cheap; local embedding (recommended bge-m3/1024) recovers semantic
   matches; an optional text reranker can improve ordering. The user can switch
   to custom/Bailian, set dimensions/batch/text_type/instruct/encoding, and
   customize weights. The model/provider identity must be pinned/recorded; no
   model is silently downloaded.
3. **B uses a separate retrieval budget.** Remote embedding/rerank calls are
   counted by B's own limits/status/UI/tests; A's completion ledger is not
   touched. This avoids cross-branch interference.
4. **Sentence/segment granularity is feasible** with block/sentence/rune
   locators for articles and language/cue-range locators for subtitles.
5. **PDF needs server extraction and a page-text store.** A native text-layer
   extractor plus a **local PaddleOCR** default covers scanned pages; cloud OCR
   is explicit opt-in with a token/terms. Browser-only text cannot be
   authoritative for C.
6. **The byte cap is a real failure mode**, not theoretical; the assembler must
   count both runes and UTF-8 bytes.
7. **The existing FTS table is content-level**, not sentence/segment-level. v1
   should generate lexical candidates in memory; a passage index is a deferred
   optimization.
8. **The existing tests do not cover the chat HTTP path or the middle-answer
   case**, so the new suite must add both and preserve the baseline as a negative
   control.
9. **Content versioning is adopted** as `content_revision` + `content_hash`,
   with legacy backfill; C/E consume it, and text hashes are only a fallback.

## 12. Not read / not verified

- Other SQLite migrations beyond 006, 028, 063, 075, 080, 081 and
  `migrations.go` were not individually reviewed.
- No browser/E2E run, no live provider call, no embedding model, no PDF
  extractor/OCR call and no implementation code were used. The probes are
  synthetic and local. Public Aliyun Bailian embedding/rerank docs were read and
  summarized in [embedding/provider notes](b-embedding-provider-notes.md); no
  live request was made.
- The 200,000-byte rejection was demonstrated with a synthetic 120,000-character
  string, not a real user document; the code path is the same
  (`utf8ValidChat`), but the exact user impact depends on the document.
- The `owner/a-foundation` branch documents (A's ADR, requirements, review memo)
  were read read-only via `git show` for coordination, but they are not part of
  this branch and are not relied on as merged evidence.
- No actual `test/offline` or `test/live` test files exist yet; only the
  documented scaffolding is added in this docs-only branch.
- The evaluation set and gold labels do not exist yet; WANG Jiaqi will prepare
  them.
- A PaddleOCR token was pasted into the Chat input during this session. It was
  not used, copied into a file, logged or committed; it must be treated as
  compromised and rotated in the PaddleOCR console.

## 13. Commands run in this audit

Environment: Linux, Go 1.27.1, Node v24.20.0, Vitest 5.0.5, SQLite 3.53.4
(modernc.org/sqlite v1.60.1).

```text
git -C ... branch -a
git -C ... ls-remote --heads origin
git -C ... checkout main
git -C ... checkout -b owner/b-retrieval
git -C ... status --short --branch

go test ./... -count=1
# exit 0; 43 ok packages, 11 [no test files] packages
cd src/web && npm test
# 61 files / 231 tests passed

go test ./internal/adapters/sqlite -run 'TestLibrary(DatabaseCardProjectionPreservesFullReadsAndSearch|PageFiltersSearchesAndOmitsBodies)' -count=1
# PASS, 2 tests

go test -overlay=... ./internal/adapters/sqlite -run TestFTSProbe -count=1 -v
# PASS; sqlite_version=3.53.4; bm25() returned -0.28095409307129493

go test -overlay=... ./internal/ai -run TestContextByteLimitProbe -count=1 -v
# PASS; cjk 120000 chars / 360000 bytes -> valid=false; ascii -> valid=true

cd src/web && npm test -- src/features/ai/chatContext.test.ts
# PASS, 1 file / 2 tests

node /tmp/opencode/b_baseline.mjs
# 5 fixtures; see b-baseline.log

node -e "import('/home/xenoplore/CS5351-Final/CS5351_SE_Group2/src/web/src/features/ai/chatContext.ts').then(({buildChatContext})=>{for(const n of [79,80,81,82,83]){const out=buildChatContext('😀'.repeat(200),'',[],n);console.log(n,out.length,out.isWellFormed())}})"
# 79 51 false; 80 52 true; 81 53 false; 82 54 false; 83 55 false
```

The overlay probes compile temporary test files from `/tmp` against the checked
out packages; they do not modify repository files. The baseline script imports
the exact `chatContext.ts` source with Node's type stripping and writes the raw
JSON lines in [b-baseline.log](b-baseline.log). Public Aliyun Bailian
embedding/rerank pages were read with the web-fetch tool; no provider endpoint
was called.