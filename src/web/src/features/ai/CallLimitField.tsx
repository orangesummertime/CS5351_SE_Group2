import { budgetSummaryText } from './callBudget'
import type { AIBudgetReport } from './types'

// CallLimitField is the optional per-task provider-call limit input. A blank
// value is labelled "No call limit" and omits the field from the request. An
// invalid value blocks the request and shows an inline error.
export function CallLimitField({ value, onChange, error, budget, id = 'ai-call-limit' }: {
  value: string
  onChange: (value: string) => void
  error?: string
  budget?: AIBudgetReport
  id?: string
}) {
  return <div className="ai-call-limit">
    <label htmlFor={id}>调用上限</label>
    <input
      id={id}
      type="text"
      inputMode="numeric"
      pattern="[0-9]*"
      value={value}
      placeholder="不限制调用次数"
      aria-describedby={`${id}-hint`}
      aria-invalid={Boolean(error)}
      onChange={event => onChange(event.target.value)}
    />
    <span id={`${id}-hint`} className="ai-call-limit-hint">{error || (value.trim() === '' ? 'No call limit · 留空表示不限制 AI 调用次数（1–100 可自定义）。' : '每个任务最多发起的 AI 调用次数。')}</span>
    {budget && <span className="ai-call-limit-budget" role="status">{budgetSummaryText(budget)}</span>}
  </div>
}
