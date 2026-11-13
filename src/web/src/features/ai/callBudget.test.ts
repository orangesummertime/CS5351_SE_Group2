import { expect, test } from 'vitest'
import { MAX_PROVIDER_CALLS, USAGE_UNKNOWN_COPY, budgetBlocker, budgetReportFromErrorPayload, budgetStatusLabel, budgetSummaryText, budgetUsageText, buildSummaryRequest, buildTranslationRequest, callLimitField, classifyChatStreamFrame, parseCallLimit, resolveLimitedChatTerminal } from './callBudget'

test('an omitted call limit is accepted and omits the request field', () => {
  expect(parseCallLimit('')).toEqual({ ok: true })
  expect(parseCallLimit('   ')).toEqual({ ok: true })
  expect(callLimitField(undefined)).toEqual({})
})

test('whole limits 1..100 are accepted and included in the payload', () => {
  expect(parseCallLimit('1')).toEqual({ ok: true, value: 1 })
  expect(parseCallLimit(' 100 ')).toEqual({ ok: true, value: MAX_PROVIDER_CALLS })
  expect(callLimitField(3)).toEqual({ maxProviderCalls: 3 })
})

test('zero, negative, fractional and too-large limits are rejected', () => {
  for (const raw of ['0', '-1', '1.5', '101', 'abc', '1e3', '+5']) {
    expect(parseCallLimit(raw).ok).toBe(false)
  }
})

test('unknown usage is never rendered as a known zero', () => {
  expect(budgetUsageText({ prompt: null, output: null, total: null })).toBe('用量不可用')
  expect(budgetUsageText(undefined)).toBe('用量不可用')
  expect(budgetUsageText({ prompt: 2, output: 3, total: 5 })).toContain('5 tokens')
})

test('budget status labels and summary include counters', () => {
  expect(budgetStatusLabel('completed')).toBe('已完成')
  expect(budgetStatusLabel('usage_unknown')).toBe('用量不可用')
  const text = budgetSummaryText({ max: 3, started: 2, usageKnown: true, status: 'budget_exhausted', cumulative: { prompt: 1, output: 1, total: 2 } })
  expect(text).toContain('已达到调用上限')
  expect(text).toContain('2/3')
})

test('a non-completed limited run is blocked from looking finished', () => {
  expect(budgetBlocker({ max: 1, started: 1, usageKnown: false, status: 'usage_unknown' })).toBe(USAGE_UNKNOWN_COPY)
  expect(budgetBlocker({ max: 1, started: 1, usageKnown: true, status: 'budget_exhausted', cumulative: { prompt: 1, output: 1, total: 2 } })).toContain('已达到调用上限')
  expect(budgetBlocker({ max: 1, started: 1, usageKnown: true, status: 'completed', cumulative: { prompt: 1, output: 1, total: 2 } })).toBe('')
  expect(budgetBlocker(undefined)).toBe('')
})

// SSE chat frame classification: the request hook and tests share one parser.
test('a completed limited SSE frame is success, not failure', () => {
  const report = { max: 1, started: 1, usageKnown: true, status: 'completed' as const, cumulative: { prompt: 2, output: 3, total: 5 } }
  const frame = classifyChatStreamFrame({ status: 'completed', budget: report })
  expect(frame.terminal).toBe(false)
  expect(frame.budget).toEqual(report)
})

test('an unknown-usage SSE frame is a terminal failure with the approved copy', () => {
  const frame = classifyChatStreamFrame({ status: 'usage_unknown', budget: { max: 1, started: 1, usageKnown: false, status: 'usage_unknown' }, partial: '部分回答' })
  expect(frame.terminal).toBe(true)
  expect(frame.status).toBe('usage_unknown')
  expect(frame.partial).toBe('部分回答')
  expect(frame.message).toBe(USAGE_UNKNOWN_COPY)
})

test('budget-exhausted and provider-error SSE frames are terminal failures', () => {
  expect(classifyChatStreamFrame({ status: 'budget_exhausted' }).terminal).toBe(true)
  expect(classifyChatStreamFrame({ status: 'provider_error', error: '上游失败' }).terminal).toBe(true)
  expect(classifyChatStreamFrame({ status: 'cancelled' }).terminal).toBe(true)
  expect(classifyChatStreamFrame({ error: '上游失败' }).terminal).toBe(true)
})

test('a plain delta SSE frame is appended text, not terminal', () => {
  const frame = classifyChatStreamFrame({ delta: '你好' })
  expect(frame.terminal).toBe(false)
  expect(frame.delta).toBe('你好')
})

// Actual request body construction used by the translation and summary hooks.
test('translation request omits maxProviderCalls when no limit is set', () => {
  const body = buildTranslationRequest({ contentId: 'rss:a', revision: 3 }, undefined, false)
  expect(body).toEqual({ contentId: 'rss:a', revision: 3 })
  expect('maxProviderCalls' in body).toBe(false)
})

test('translation request includes the validated limit and regenerate flag', () => {
  const body = buildTranslationRequest({ contentId: 'rss:a', revision: 3 }, 3, true)
  expect(body.maxProviderCalls).toBe(3)
  expect(body.regenerate).toBe(true)
})

test('summary request includes the limit only when present', () => {
  const base = { contentId: 'rss:b', text: 'source' }
  expect(buildSummaryRequest(base, 1, undefined, false)).toEqual({ contentId: 'rss:b', text: 'source', revision: 1 })
  const limited = buildSummaryRequest(base, 2, 1, false)
  expect(limited.maxProviderCalls).toBe(1)
  expect(limited.revision).toBe(2)
})

test('invalid limit produces a user-facing error and blocks the request', () => {
  const parsed = parseCallLimit('0')
  expect(parsed.ok).toBe(false)
})

test('error payload budget report is extracted only when present', () => {
  const report = { max: 1, started: 1, usageKnown: false, status: 'usage_unknown' as const }
  expect(budgetReportFromErrorPayload({ code: 'x', budget: report })).toEqual(report)
  expect(budgetReportFromErrorPayload({ code: 'x' })).toBeUndefined()
  expect(budgetReportFromErrorPayload(null)).toBeUndefined()
})

// Limited-chat terminal resolution shared by the real send path and tests.
test('limited run with delta then clean EOF without a completed frame is interrupted', () => {
  expect(resolveLimitedChatTerminal({ limited: true, sawCompletedFrame: false, answer: '部分', aborted: false })).toEqual({ kind: 'interrupted' })
})

test('limited run that received a completed frame is complete', () => {
  expect(resolveLimitedChatTerminal({ limited: true, sawCompletedFrame: true, answer: '完整回答', aborted: false })).toEqual({ kind: 'complete' })
})

test('limited run with no text and no completed frame is incomplete', () => {
  expect(resolveLimitedChatTerminal({ limited: true, sawCompletedFrame: false, answer: '', aborted: false })).toEqual({ kind: 'incomplete' })
})

test('explicit abort of a limited run is cancelled, not complete', () => {
  expect(resolveLimitedChatTerminal({ limited: true, sawCompletedFrame: false, answer: '部分', aborted: true })).toEqual({ kind: 'cancelled' })
})

test('unbudgeted runs keep upstream behavior', () => {
  expect(resolveLimitedChatTerminal({ limited: false, sawCompletedFrame: false, answer: '回答', aborted: false })).toEqual({ kind: 'unbudgeted-complete' })
  expect(resolveLimitedChatTerminal({ limited: false, sawCompletedFrame: false, answer: '', aborted: false })).toEqual({ kind: 'incomplete' })
})
