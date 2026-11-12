# B · Baseline: original head/tail context behavior

- **Status:** evidence only; no implementation and no provider call.
- **Owner:** WANG Jiaqi (B).
- **Baseline commit:** `45611ad0e214a2d9de6f0ec5a5aa84fc493d76f2`.
- **Function under test:** `buildChatContext` from
  `src/web/src/features/ai/chatContext.ts`.
- **Date:** 2026-11-12.

## Purpose

NFR-02 requires the new retrieval path to be at least as good as the original
head/tail truncation at the same character budget. This document fixes that
baseline on synthetic long documents whose only answer is in the middle. It is
the negative control for B-T02/B-T13, not a product test and not a claim about
real user documents.

## Method

The exact TypeScript source was imported with Node v24.20.0's type stripping and
called with fixed inputs:

```text
buildChatContext(source, overview?, points?, budget?)
```

The executed script is reproduced in the appendix. For each case it records:

- the input length in JavaScript UTF-16 code units;
- the SHA-256 of the input and answer;
- the answer offset;
- the output length and SHA-256;
- whether the exact answer text is present;
- the output's UTF-8 byte length, because the Go backend rejects contexts over
  200,000 bytes ([chat.go:138-141](../../src/internal/ai/chat.go#L138-L141)).

Raw output: [b-baseline.log](b-baseline.log).

## Results

| Case | Kind | Input chars | Answer offset | Budget | Output chars | Output bytes | Answer retained | Backend byte cap |
| --- | --- | ---: | ---: | ---: | ---: | ---: | --- | --- |
| B-BASE-01 | Article | 156,032 | 78,000 | 120,000 | 119,972 | 230,724 | **no** | exceeded |
| B-BASE-02 | Subtitles | 144,011 | 71,976 | 120,000 | 119,972 | 289,932 | **no** | exceeded |
| B-BASE-03 | PDF pages | 160,039 | 79,121 | 120,000 | 119,972 | 358,022 | **no** | exceeded |
| B-BASE-04 | Article + summary | 132,032 | 66,000 | 120,000 | 119,973 | 207,237 | **no** | exceeded |
| B-BASE-05 | Article, 8k stress | 156,024 | 78,000 | 8,000 | 7,972 | 15,338 | **no** | within cap |

All five cases lose the only answer. In the four 120,000-character cases the
output is also over the backend's 200,000-byte limit, so the request would be
rejected as `ErrInvalid` before the provider call. The 8,000-character stress
case fits the byte cap but still returns a head/tail excerpt that omits the
answer.

### Why the answer is lost

For B-BASE-01, the source is 156,032 characters. With no summary material,
`buildChatContext` reserves 40 characters, splits the remaining 119,960 into
59,980 head and 59,980 tail characters, and joins them with the marker and
ellipsis. The answer at offset 78,000 is outside both ranges.

For B-BASE-04, the summary material occupies up to 35% of the budget
([chatContext.ts:8](../../src/web/src/features/ai/chatContext.ts#L8)); the
remaining head/tail ranges move further away from the middle, so the answer at
offset 66,000 is lost even though the source is shorter than B-BASE-01.

The existing unit test only checks that a long value stays within the limit and
contains the marker/ellipsis
([chatContext.test.ts:9-15](../../src/web/src/features/ai/chatContext.test.ts#L9-L15)).
It does not assert that a middle answer survives.

A separate boundary probe on the same function found that odd budgets can split
a surrogate pair: with `😀`.repeat(200) and budget 79, the output's
`isWellFormed()` is `false` (80 is `true`; 81-83 are `false`). The new assembler
must count code points and never emit a lone surrogate; see
[the context audit §2](b-context-audit.md) and
[b-unicode-probe.log](b-unicode-probe.log).

## Reproduction

From the repository root:

```bash
node /tmp/opencode/b_baseline.mjs
```

The recorded run used Node v24.20.0 and the exact source path
`/home/xenoplore/CS5351-Final/CS5351_SE_Group2/src/web/src/features/ai/chatContext.ts`.
The raw JSON lines are in [b-baseline.log](b-baseline.log). The script is
deterministic: it uses no randomness, clock, network or provider.

## Limits

- The fixtures are synthetic and intentionally adversarial; they do not estimate
  the prevalence of middle answers in real content.
- The character count is JavaScript `String.length`, matching the current
  frontend. The new design should use Unicode code points and a separate UTF-8
  byte cap; that difference is part of the audit.
- The run covers only `buildChatContext`; it does not test the HTTP transport or
  a real provider.
- The PDF fixture is a joined page-text string; the current browser extracts that
  text with pdf.js and does not persist it
  ([PDFReader.tsx:51-57](../../src/web/src/features/documents/PDFReader.tsx#L51-L57)).
  For the new NFR-02 comparison, both the baseline and the hybrid path must use
  the same server-side page-text snapshot, not two different extractors.
- This evidence is the baseline anchor for NFR-02, not proof that the new
  retrieval path meets the threshold. That requires the separate evaluation
  protocol and report.

## Appendix: executed harness

```js
import { createHash } from 'node:crypto'
import { buildChatContext } from '/home/xenoplore/CS5351-Final/CS5351_SE_Group2/src/web/src/features/ai/chatContext.ts'

const sha = value => createHash('sha256').update(value).digest('hex')
const repeat = (seed, count) => Array.from({ length: count }, (_, i) => `${seed} ${String(i).padStart(5, '0')} `).join('')

const cases = []

// 1. Article: answer at the midpoint of a >120k-character body.
{
  const answer = '【B-BASE-01 黄金答案】中段检索目标：答案位于文档中部。'
  const source = repeat('文章段落内容', 6000) + answer + repeat('文章段落内容', 6000)
  cases.push({ id: 'B-BASE-01', kind: 'article', docId: 'fixture:article-mid', source, answer, budget: 120000 })
}

// 2. Subtitle cues: answer at cue 3000 of 6000.
{
  const cues = Array.from({ length: 6000 }, (_, i) =>
    i === 2999 ? '【B-BASE-02 黄金答案】字幕中段目标：cue 索引 2999。' : `第 ${String(i).padStart(4, '0')} 条字幕内容，用于构造长转录文本。`
  )
  const source = cues.join('\n')
  cases.push({ id: 'B-BASE-02', kind: 'subtitles', docId: 'fixture:subtitle-mid', source, answer: '【B-BASE-02 黄金答案】字幕中段目标：cue 索引 2999。', budget: 120000 })
}

// 3. PDF pages: answer on page 90 of 180, each page padded to ~1000 characters.
{
  const pageFiller = '本页正文用于构造长文档，包含解释、例证与背景信息。'.repeat(35)
  const pages = Array.from({ length: 180 }, (_, i) =>
    i === 89 ? `【B-BASE-03 黄金答案】PDF 中段目标：第 90 页。\n${pageFiller}` : `第 ${String(i + 1).padStart(3, '0')} 页正文内容。\n${pageFiller}`
  )
  const source = pages.join('\n')
  cases.push({ id: 'B-BASE-03', kind: 'pdf', docId: 'fixture:pdf-mid', source, answer: '【B-BASE-03 黄金答案】PDF 中段目标：第 90 页。', budget: 120000 })
}

// 4. Summary material consumes 35% of the budget, pushing a mid-document answer out of both head and tail.
{
  const material = repeat('摘要素材', 5000)
  const answer = '【B-BASE-04 黄金答案】摘要挤压目标：答案位于文档中部。'
  const source = repeat('正文内容', 6000) + answer + repeat('正文内容', 6000)
  cases.push({ id: 'B-BASE-04', kind: 'article+summary', docId: 'fixture:article-summary-squeeze', source, overview: material, points: ['要点一', '要点二'], answer, budget: 120000 })
}

// 5. Stress budget: same article shape as B-BASE-01 at 8k characters.
{
  const answer = '【B-BASE-05 黄金答案】小预算中段目标。'
  const source = repeat('文章段落内容', 6000) + answer + repeat('文章段落内容', 6000)
  cases.push({ id: 'B-BASE-05', kind: 'article', docId: 'fixture:article-mid-8k', source, answer, budget: 8000 })
}

for (const item of cases) {
  const output = buildChatContext(item.source, item.overview, item.points, item.budget)
  const index = item.source.indexOf(item.answer)
  console.log(JSON.stringify({
    case: item.id,
    kind: item.kind,
    docId: item.docId,
    budget: item.budget,
    inputChars: item.source.length,
    inputSha256: sha(item.source),
    answerOffset: index,
    answerSha256: sha(item.answer),
    outputChars: output.length,
    outputBytes: Buffer.byteLength(output, 'utf8'),
    outputSha256: sha(output),
    answerInOutput: output.includes(item.answer),
    outputHasHead: output.slice(0, 12),
    outputHasEllipsis: output.includes('……'),
    outputTail: output.slice(-12),
  }))
}
```