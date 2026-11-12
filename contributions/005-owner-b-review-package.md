# 005 · Owner B review package: hybrid question-relevant retrieval

- **Commit:** `docs: prepare owner B retrieval review package`; see this file's
  Git history.
- **Task owner:** WANG Jiaqi (B).
- **Scope:** B baseline, draft FR-02/NFR-02/INV-04/INV-05, ADR-002, hybrid
  retrieval test plan, evaluation protocol, PDF extraction design, debt
  register, course test layout and AI-use disclosure.
- **Review:** Awaiting an independent non-author reviewer. No implementation,
  evaluation run or approval is claimed.

> Numbering note: this branch starts from `main`, where the highest contribution
> number is 001. The unmerged `owner/a-foundation` branch now uses 002, 003 and
> 004, so this package uses 005 to keep the eventual merge append-only instead
> of colliding with A's paths.

## What changed and why

The package makes B's retrieval work reviewable before any code is written:

- [context audit](../docs/evidence/b-context-audit.md) now covers the full
  component chain: settings, prompt templates, summary hooks, `AIInsights`,
  chat panel, reader workspaces, PDF reader, transport, AI service, summary
  evidence/caching, provider adapter, content/subtitle stores, SQLite and tests.
- [baseline](../docs/evidence/b-baseline.md) and
  [baseline log](../docs/evidence/b-baseline.log) reproduce the original
  head/tail loss on five fixed fixtures; all five lose a middle-only answer, and
  the four 120k cases also exceed the backend's 200,000-byte cap.
- [requirements](../docs/req/requirements.yaml) registers only B's FR-02,
  NFR-02, INV-04 and INV-05. FR-02/NFR-02 describe hybrid retrieval
  (lexical candidates + embedding rerank + optional text rerank),
  sentence/segment granularity, user-selectable local/custom/Bailian embedding
  providers (local-first default bge-m3/1024), configurable dimensions/weights,
  a B-only retrieval budget, server-side PDF text extraction with local
  PaddleOCR default/cloud opt-in and explicit fallback statuses.
- [ADR-002](../docs/architecture/adr/002-question-relevant-retrieval.md) compares
  baseline, content-level FTS/BM25, lexical-only, embedding-only and hybrid;
  selects hybrid for review; defines the D2.2 six-layer context, budget and
  compression/re-injection rules, sentence/segment passage contract, the
  namespaced `retrieval` wire envelope, PDF text-layer/PaddleOCR design,
  content-revision migration, B-only budget and invariants.
- [embedding/provider notes](../docs/evidence/b-embedding-provider-notes.md)
  records the public Aliyun Bailian embedding/rerank API facts, model/dimension
  limits and the local/custom/Bailian settings design. No live call was made.
- [PaddleOCR notes](../docs/evidence/b-paddleocr-notes.md) records the PDF OCR
  job flow, token redaction, statuses and fake-API test plan. No OCR job was
  submitted and no token is stored.
- [implementation plan](../docs/plans/b-retrieval-preimplementation.md) defines
  the hybrid interfaces, sentence/segment splitter, embedding/rerank ports,
  provider settings, B budget, PDF/PaddleOCR flow, content revision, C
  dependency, B-T01–B-T38 deterministic tests, negative coverage and phased
  verification.
- [evaluation protocol](../docs/evidence/b-eval-protocol.md) fixes the NFR-02
  dataset, gold YAML, baseline/lexical/hybrid comparison, frozen-vector
  determinism and report format. WANG Jiaqi prepares the evaluation set; no
  labels were invented here.
- [debt register](../docs/debt-register.md) records the deferred vector index,
  cross-language model, passage FTS, monitoring, local PaddleOCR runtime,
  query expansion/agents, revision backfill and cache policies with reopen
  conditions.
- [test layout](../test/README.md) adds the course-facing
  [offline](../test/offline/README.md) and [live](../test/live/README.md)
  entry points as documentation-only scaffolding.
- [AI-use log](../docs/ai-use-log.yaml) discloses this session.
- The root README B row now names WANG Jiaqi.

## Contribution to the project

This commit gives the group a concrete, testable B design without pretending the
feature exists. It separates the problem (frontend truncation and backend trust)
from the proposed solution (server-side hybrid retrieval), makes the NFR-02
comparison reproducible, and gives C a stable sentence/segment passage reference
contract. It also records the A embedding-budget, C content-revision and PDF
extraction dependencies that must be settled before code.

## Verification and limits

Actually run in this session:

- `go test ./... -count=1` — exit 0; 43 `ok` packages and 11 `[no test files]`
  packages; raw log in [b-go-full.log](../docs/evidence/b-go-full.log).
- `cd src/web && npm test` — 61 files / 231 tests passed; raw log in
  [b-vitest-full.log](../docs/evidence/b-vitest-full.log).
- `go test ./internal/adapters/sqlite -run 'TestLibrary(...)' -count=1` — PASS,
  2 tests.
- `go test -overlay=... ./internal/adapters/sqlite -run TestFTSProbe -count=1` —
  PASS; SQLite 3.53.4; FTS5 trigram + `bm25()` confirmed.
- `go test -overlay=... ./internal/ai -run TestContextByteLimitProbe -count=1` —
  PASS; 120,000 CJK characters = 360,000 bytes and `utf8ValidChat` returns
  false.
- `cd src/web && npm test -- src/features/ai/chatContext.test.ts` — PASS, 1 file
  / 2 tests.
- Node v24.20.0 baseline replay — 5 fixtures; see the log.
- YAML/JSON schema validation and relative-link checks — see the PR description;
  they are rerun before submission.

Limits:

- No retrieval implementation, no embedding/rerank call, no migration, no PDF
  extractor/OCR call, no frontend wiring, no provider call, no live evaluation
  and no browser/E2E run.
- The evaluation set and gold labels do not exist yet; WANG Jiaqi will prepare
  and confirm them.
- PDF extraction/OCR is a design proposal; local PaddleOCR is the default,
  cloud OCR is explicit opt-in. The native library and PaddleOCR/PaddleOCR-VL
  license, cloud terms, token handling and extraction quality are not yet
  reviewed.
- Embedding/rerank provider defaults (local bge-m3/1024, weights 0.35/0.65/0,
  top-K 100, B budget 16 requests / 60,000 chars), vector persistence and B
  budget defaults are proposed, not approved; the local runtime/model is not
  bundled.
- Content revision is adopted in the design as `content_revision` +
  `content_hash`; the migration/backfill still needs C/E review, and duplicate
  migrations must be avoided at merge.
- `test/offline` and `test/live` are documentation-only entry points; actual
  test files await approval.
- A PaddleOCR token pasted in chat was not used or committed; it must be
  rotated. Do not treat the pasted snippet as a secret store.
- The root README B owner cell is updated to WANG Jiaqi; no other task owner is
  claimed.

## Presentation explanation

“The original chat path truncates in the browser and sends free text the server
never verifies. I reproduced four middle-answer failures and found a second
problem: the 120k frontend limit is counted in UTF-16 units while the backend
rejects over 200k UTF-8 bytes. I prepared a local-first server-side hybrid
design — lexical candidates, local embedding by default plus user-selectable
custom/Bailian embeddings, optional text rerank and configurable weights at
sentence/segment granularity — with a B-only retrieval budget, a server-side PDF
text-layer/local-PaddleOCR store with cloud OCR opt-in, a namespaced retrieval
envelope for C, and a frozen-vector evaluation protocol. No code or evaluation
is claimed; the next step is independent review, provider/OCR license decisions,
and human gold labels.”