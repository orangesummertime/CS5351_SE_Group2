export interface SummaryPoint { text: string; indices: number[] }
export interface SummaryOutline { title: string; level: number; indices?: number[]; start?: number; end?: number; page?: number; anchor?: string; children?: SummaryOutline[] }
export interface SummaryCitation { index: number; start?: number; end?: number; page?: number; quote: string }

/** Evidence is intentionally source-shaped so the reader can locate it without
 * knowing whether the source is a transcript, article, or PDF. */
export interface SummaryEvidence {
  id?: string
  index?: number
  sentenceId?: string
  quote: string
  start?: number
  end?: number
  page?: number
  cueIds?: string[]
  segmentIndexes?: number[]
}

export interface SummarySentence {
  id?: string
  text: string
  evidence?: SummaryEvidence[]
  start?: number
  end?: number
  page?: number
}

export interface SummaryUnit {
  unitId: string
  type?: string
  role?: string
  topic?: string
  takeaway?: string
  key?: string
  reason?: string
  full?: string
  sentences?: SummarySentence[]
  briefSentenceIds?: string[]
  evidence?: SummaryEvidence[]
  source?: SummaryEvidence[]
}

export interface SummarySection {
  sectionId: string
  title: string
  level?: number
  topic?: string
  takeaway?: string
  start?: number
  end?: number
  page?: number
  source?: SummaryEvidence[]
  evidence?: SummaryEvidence[]
  units?: SummaryUnit[]
  children?: SummarySection[]
}

export interface SummaryDocument {
  formatVersion?: number | string
  contentId?: string
  source?: string
  model?: string
  provenance?: Record<string, unknown>
  levels?: string[]
  topic?: string
  sections: SummarySection[]
}

export interface SummaryProgress {
  completedCalls: number
  totalCalls: number
  completedSections?: number
  totalSections?: number
  currentCall?: number
  phase?: 'document' | 'chapter' | 'merge' | 'checkpoint' | 'error' | string
  startedAt?: number
  error?: string
  complete: boolean
}

export interface SummaryResult {
  key: string; contentId: string; source: string; baseUrl: string; model: string
  totalCalls: number; completedCalls: number; complete: boolean
  overview?: string; points?: SummaryPoint[]; citations?: SummaryCitation[]; outline?: SummaryOutline[]; updatedAt: number
  formatVersion?: number | string; document?: SummaryDocument; progress?: SummaryProgress
  budget?: AIBudgetReport
}
export interface SummaryInspection { settings: AISettings; result: SummaryResult; active: boolean }
import type { AISettings, AIBudgetReport } from './types'
