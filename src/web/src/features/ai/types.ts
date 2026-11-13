export interface AIPromptTemplate { system: string; user: string }
export interface AISettings { protocol: 'openai' | 'compatible'; baseUrl: string; model: string; chatModel?: string; enabledModels?: string[]; availableModels?: string[]; summaryModel?: string; summaryPrompt?: string; translationPrompt?: string; chatPrompt?: string; summaryTemplate?: AIPromptTemplate; translationTemplate?: AIPromptTemplate; chatTemplate?: AIPromptTemplate; defaultTemplates?: { summary: AIPromptTemplate; translation: AIPromptTemplate; chat: AIPromptTemplate }; summaryContextBudget?: number; summaryReady?: boolean; hasKey: boolean; ready: boolean; translating: boolean; revision: number }
export type AIBudgetOutcome = 'active' | 'completed' | 'budget_exhausted' | 'usage_unknown' | 'cancelled' | 'provider_error'
export interface AIUsageReport { prompt: number | null; output: number | null; total: number | null }
export interface AIBudgetReport {
  max: number; started: number; usageKnown: boolean
  perCall?: AIUsageReport[]; cumulative?: AIUsageReport; status: AIBudgetOutcome
}
export interface TranslationResult { key: string; contentId: string; language: string; targetLanguage: string; baseUrl: string; model: string; total: number; texts: string[]; updatedAt: number; budget?: AIBudgetReport }
export interface TranslationInspection { settings: AISettings; result: TranslationResult; active: boolean }
export type AITask = 'translation' | 'summary'
export interface AICheckResult { task: AITask; model: string; elapsedMs: number; revision: number; budget?: AIBudgetReport }
