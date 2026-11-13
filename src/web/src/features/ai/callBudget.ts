import type { AIBudgetReport, AIUsageReport } from './types'

// The largest explicit provider-call limit accepted by the backend.
export const MAX_PROVIDER_CALLS = 100

// parseCallLimit validates the optional per-task provider-call limit entered in
// the UI. An empty or blank value means "no call limit" and omits the field so
// the backend keeps its unbudgeted behavior. Any other value must be a whole
// integer between 1 and 100; anything else is rejected before the request.
export function parseCallLimit(raw: string): { ok: true; value?: number } | { ok: false; message: string } {
  const text = raw.trim()
  if (text === '') return { ok: true }
  if (!/^[0-9]+$/.test(text)) return { ok: false, message: '调用上限必须是 1–100 的整数，或留空表示不限制。' }
  const value = Number(text)
  if (!Number.isInteger(value) || value < 1 || value > MAX_PROVIDER_CALLS) {
    return { ok: false, message: '调用上限必须是 1–100 的整数，或留空表示不限制。' }
  }
  return { ok: true, value }
}

// callLimitField returns a request body fragment for a validated limit. An
// omitted limit yields an empty object so the JSON body is byte-compatible with
// the unbudgeted contract.
export function callLimitField(value?: number): { maxProviderCalls?: number } {
  return value === undefined ? {} : { maxProviderCalls: value }
}

// budgetUsageText renders a usage triple, showing an explicit unknown marker
// rather than a misleading zero when the provider did not report usage.
export function budgetUsageText(usage?: AIUsageReport): string {
  if (!usage || usage.total === null || usage.total === undefined) return '用量不可用'
  return `${usage.total} tokens（输入 ${usage.prompt ?? '?'} / 输出 ${usage.output ?? '?'}）`
}

// budgetStatusLabel maps the machine status to a user-facing string.
export function budgetStatusLabel(status?: string): string {
  switch (status) {
    case 'completed': return '已完成'
    case 'budget_exhausted': return '已达到调用上限'
    case 'usage_unknown': return '用量不可用'
    case 'cancelled': return '已取消'
    case 'provider_error': return '服务暂时不可用'
    default: return '进行中'
  }
}

// budgetSummaryText produces the compact counter/outcome line shown in the UI.
export function budgetSummaryText(report?: AIBudgetReport): string {
  if (!report) return ''
  return `${budgetStatusLabel(report.status)} · 已发起 ${report.started}/${report.max} 次 · ${budgetUsageText(report.cumulative)}`
}

// The approved copy for a streamed answer whose usage is unavailable.
export const USAGE_UNKNOWN_COPY = 'The answer was received, but usage data is unavailable. This task cannot make another AI request. Start a new task to continue.'

// budgetBlocker returns a user-facing notice when a non-completed limited run
// must not be presented as a finished answer, or an empty string otherwise.
export function budgetBlocker(report?: AIBudgetReport): string {
  if (!report) return ''
  if (report.status === 'usage_unknown') return USAGE_UNKNOWN_COPY
  if (report.status === 'budget_exhausted') return '本次任务已达到调用上限，已完成内容已保留。请开始新任务并设置新的调用上限继续。'
  return ''
}

// Parsed chat SSE data-frame. The budget report is optional because unbudgeted
// frames keep the original shape.
export interface ChatStreamFrame {
  delta?: string
  error?: string
  errorCode?: string
  status?: string
  budget?: AIBudgetReport
  partial?: string
}

// classifyChatStreamFrame interprets one decoded SSE data frame so the request
// hook and tests share one parser. It returns the appended delta text (if any),
// whether a terminal error/failure was reached, and the approved notice copy to
// show. A completed limited frame carries a budget report and is success.
export function classifyChatStreamFrame(frame: ChatStreamFrame): { delta: string; terminal: boolean; status?: string; partial?: string; budget?: AIBudgetReport; message?: string } {
  const status = frame.status
  if (status === 'completed') {
    // Done only on actual success; never a failure terminal.
    return { delta: '', terminal: false, status, budget: frame.budget }
  }
  const failure = frame.error || status === 'usage_unknown' || status === 'budget_exhausted' || status === 'cancelled' || status === 'provider_error'
  if (failure) {
    const message = status === 'usage_unknown' ? USAGE_UNKNOWN_COPY : frame.error ?? '对话暂时不可用。'
    return { delta: '', terminal: true, status, partial: frame.partial, budget: frame.budget, message }
  }
  return { delta: frame.delta ?? '', terminal: false, status, budget: frame.budget }
}

// buildTranslationRequest composes the real /ai/subtitles/translate body. An
// omitted limit produces no maxProviderCalls key (byte-compatible with the
// unbudgeted contract); regenerate is included only when requested.
export function buildTranslationRequest(base: Record<string, unknown>, limit: number | undefined, regenerate: boolean): Record<string, unknown> {
  return { ...base, ...(regenerate ? { regenerate } : {}), ...callLimitField(limit) }
}

// buildSummaryRequest composes the real /ai/summaries/generate body from the
// existing input plus revision and the optional limit.
export function buildSummaryRequest(input: Record<string, unknown>, revision: number, limit: number | undefined, regenerate: boolean): Record<string, unknown> {
  return { ...input, revision, ...(regenerate ? { regenerate } : {}), ...callLimitField(limit) }
}

// budgetReportFromErrorPayload extracts the additive budget report from an
// error response payload, or undefined when absent (unbudgeted behavior).
export function budgetReportFromErrorPayload(payload: unknown): AIBudgetReport | undefined {
  if (payload && typeof payload === 'object' && 'budget' in payload) {
    const report = (payload as { budget?: AIBudgetReport }).budget
    if (report && typeof report === 'object') return report
  }
  return undefined
}

// Limited-chat terminal resolution shared by the real send path and tests.
// A limited run is complete only when a completed frame arrived. At clean EOF
// without one, partial output must not be presented as a finished answer.
export type LimitedChatTerminal =
  | { kind: 'complete' }
  | { kind: 'interrupted' }
  | { kind: 'incomplete' }
  | { kind: 'cancelled' }
  | { kind: 'unbudgeted-complete' }

export interface LimitedChatCompletionState {
  limited: boolean
  sawCompletedFrame: boolean
  answer: string
  aborted: boolean
}

// resolveLimitedChatTerminal decides the outcome of a chat stream. Unbudgeted
// runs keep the upstream behavior (existing when answer present). For limited
// runs a completed frame is required; else EOF with partial text is
// "interrupted", EOF with no text is "incomplete", and user abort is
// "cancelled". No token or count value is invented here.
export function resolveLimitedChatTerminal(state: LimitedChatCompletionState): LimitedChatTerminal {
  if (!state.limited) {
    return state.answer ? { kind: 'unbudgeted-complete' } : { kind: 'incomplete' }
  }
  if (state.sawCompletedFrame) return { kind: 'complete' }
  if (state.aborted) return { kind: 'cancelled' }
  if (state.answer) return { kind: 'interrupted' }
  return { kind: 'incomplete' }
}

// User-facing notices for a limited run that did not complete cleanly. These
// never assert token/count values.
export const LIMITED_INTERRUPTED_NOTICE = '回答流在完成之前中断，已保留可见的部分输出。本次任务未确认完成。'
export const LIMITED_INCOMPLETE_NOTICE = '回答流未正常完成，本次任务未确认完成。请重新提问。'
export const LIMITED_CANCELLED_NOTICE = '已取消生成，已保留可见的部分输出。本次任务未确认完成。'
