package httptransport

import (
	"context"
	"errors"
	"net/http"
	"time"

	"tangentry/internal/ai"
)

const aiWriteTimeout = 46 * time.Minute

func aiLongWriteDeadline() time.Time { return time.Now().Add(aiWriteTimeout) }

func aiFail(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	status, body := classifyAIError(err)
	write(w, status, body)
}

func aiErrorMessage(err error) string {
	switch {
	case errors.Is(err, ai.ErrNotReady):
		return "请先在工具设置选择对话模型。"
	case errors.Is(err, ai.ErrUsageUnknown):
		return "The answer was received, but usage data is unavailable. This task cannot make another AI request. Start a new task to continue."
	case errors.Is(err, ai.ErrBudgetExhausted):
		return "本次任务已达到 AI 调用上限。请开始新任务并设置新的调用上限继续。"
	case errors.Is(err, ai.ErrAuth):
		return "上游拒绝访问，请检查 API 密钥。"
	case errors.Is(err, ai.ErrRate):
		return "上游提示额度或请求频率受限，请稍后重试。"
	case errors.Is(err, ai.ErrRequestRejected):
		return "上游拒绝了请求参数，请检查模型名称与接口协议。"
	case errors.Is(err, ai.ErrFormat):
		return "上游返回格式无法识别，请检查接口地址是否指向 OpenAI 兼容 API。"
	case errors.Is(err, context.DeadlineExceeded):
		return "等待上游响应超时，请稍后重试。"
	default:
		return "上游服务暂时不可用，请检查接口、模型与网络。"
	}
}

func chatFail(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	status, body := classifyChatError(err)
	write(w, status, body)
}

func summaryFail(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	status, body := classifySummaryError(err)
	write(w, status, body)
}

// classifyAIError returns the status and apiError for translation-style AI
// failures without writing, so callers can compose a budgeted error body.
func classifyAIError(err error) (int, apiError) {
	switch {
	case errors.Is(err, ai.ErrInvalid):
		return 400, apiError{"invalid_ai_input", "请检查接口、模型与字幕。更换接口时需填写新密钥，或明确留空使用本地服务。"}
	case errors.Is(err, ai.ErrConflict):
		return 409, apiError{"ai_config_changed", "AI 配置已更新，请重新读取设置后再开始。"}
	case errors.Is(err, ai.ErrNotReady):
		return 503, apiError{"ai_not_ready", "请先在工具设置选择翻译接口和模型。"}
	case errors.Is(err, ai.ErrBusy):
		return 409, apiError{"ai_busy", "已有 AI 操作正在进行，请等待完成或在原页面取消。"}
	case errors.Is(err, ai.ErrNotFound):
		return 404, apiError{"ai_translation_not_found", "尚未保存这份译文。"}
	case errors.Is(err, ai.ErrUsageUnknown):
		return 502, apiError{"ai_usage_unknown", "服务已完成请求，但无法获取用量数据，本任务不能再次请求 AI。请开始新任务继续；已完成内容已保留。"}
	case errors.Is(err, ai.ErrBudgetExhausted):
		return 429, apiError{"ai_call_budget_exhausted", "本次任务已达到 AI 调用上限，已完成内容已保留。请开始新任务并设置新的调用上限继续。"}
	case errors.Is(err, ai.ErrAuth):
		return 502, apiError{"ai_auth_failed", "翻译服务拒绝访问，请检查密钥与模型权限。已完成译文仍保留。"}
	case errors.Is(err, ai.ErrRequestRejected):
		return 502, apiError{"ai_request_rejected", "翻译接口拒绝了请求参数（HTTP 400/422）。请检查模型名称、协议类型、JSON 模式支持与输入长度。已完成译文仍保留。"}
	case errors.Is(err, ai.ErrJSONModeUnsupported):
		return 502, apiError{"ai_json_mode_unsupported", "当前模型或接口不支持 JSON 输出参数（response_format）。请更换支持 JSON 模式的模型。已完成译文仍保留，不会自动重试。"}
	case errors.Is(err, ai.ErrTranslationLanguage):
		return 502, apiError{"ai_translation_language", "服务无法识别目标翻译语言（HTTP 400/422）。当前模型的翻译协议不兼容，请选择支持 Chat Completions 与 JSON 输出的模型。已完成译文仍保留。"}
	case errors.Is(err, ai.ErrGatewayTimeout):
		return 504, apiError{"ai_gateway_timeout", "翻译服务网关等待超时（HTTP 504）。已完成译文已保留，可稍后手动继续；请检查网关超时设置与模型响应速度。"}
	case errors.Is(err, ai.ErrRate):
		return 429, apiError{"ai_rate_limited", "翻译服务提示额度或请求限制，请稍后手动继续。已完成译文仍保留。"}
	case errors.Is(err, ai.ErrIncomplete):
		return 502, apiError{"ai_incomplete_output", "这批译文漏了部分字幕，已完成部分已保存。可手动继续，或换一个翻译模型。"}
	case errors.Is(err, ai.ErrTruncated):
		return 502, apiError{"ai_output_truncated", "模型输出达到长度上限，这批译文未保存。已完成部分仍保留，可手动继续或更换模型。"}
	case errors.Is(err, ai.ErrRefused):
		return 502, apiError{"ai_output_refused", "模型拒绝了这次翻译。已完成部分仍保留，请更换模型后再试。"}
	case errors.Is(err, ai.ErrFormat):
		return 502, apiError{"ai_invalid_output", "这批译文的格式或编号不符，已完成部分仍保留。可手动继续，或更换翻译模型。"}
	case errors.Is(err, ai.ErrStorage):
		return 503, apiError{"ai_storage_failed", "译文或 AI 设置无法读写，请检查本机数据目录。原字幕仍保留。"}
	case errors.Is(err, context.DeadlineExceeded):
		return 504, apiError{"ai_timeout", "翻译等待超时，已完成译文保留，可手动继续。"}
	default:
		return 502, apiError{"ai_provider_failed", "翻译服务暂不可用，请检查接口、模型与网络。已完成译文仍保留。"}
	}
}

// classifySummaryError mirrors summaryFail without writing.
func classifySummaryError(err error) (int, apiError) {
	switch {
	case errors.Is(err, ai.ErrInvalid):
		return 400, apiError{"invalid_ai_summary", "请检查摘要来源和正文内容。"}
	case errors.Is(err, ai.ErrConflict):
		return 409, apiError{"ai_config_changed", "AI 配置已更新，请重新读取设置后再开始摘要。"}
	case errors.Is(err, ai.ErrNotReady):
		return 503, apiError{"ai_not_ready", "请在工具设置填写支持指令与 JSON 输出的摘要模型。专用翻译模型可继续用于翻译。"}
	case errors.Is(err, ai.ErrBusy):
		return 409, apiError{"ai_busy", "已有 AI 操作正在进行，请等待完成或在原页面取消。"}
	case errors.Is(err, ai.ErrNotFound):
		return 404, apiError{"ai_summary_not_found", "尚未保存这份摘要。"}
	case errors.Is(err, ai.ErrUsageUnknown):
		return 502, apiError{"ai_usage_unknown", "摘要服务已完成请求，但无法获取用量数据，本任务不能再次请求 AI。请开始新任务继续；已完成部分已保留。"}
	case errors.Is(err, ai.ErrBudgetExhausted):
		return 429, apiError{"ai_call_budget_exhausted", "本次摘要已达到 AI 调用上限，已完成部分已保留。请开始新任务并设置新的调用上限继续。"}
	case errors.Is(err, ai.ErrAuth):
		return 502, apiError{"ai_auth_failed", "摘要服务拒绝访问，请检查密钥与模型权限。已完成摘要仍保留。"}
	case errors.Is(err, ai.ErrRequestRejected):
		return 502, apiError{"ai_request_rejected", "摘要接口拒绝了请求参数（HTTP 400/422）。请检查模型名称、协议类型、JSON 模式支持与输入长度。已完成摘要仍保留。"}
	case errors.Is(err, ai.ErrJSONModeUnsupported):
		return 502, apiError{"ai_json_mode_unsupported", "当前摘要模型或接口不支持 JSON 输出参数（response_format）。请更换支持 JSON 模式的摘要模型。已完成摘要仍保留，不会自动重试。"}
	case errors.Is(err, ai.ErrTranslationLanguage):
		return 502, apiError{"ai_translation_language", "服务无法识别目标语言（HTTP 400/422）。当前翻译模型不兼容摘要请求，请选择支持指令与 JSON 输出的模型。已完成摘要仍保留。"}
	case errors.Is(err, ai.ErrGatewayTimeout):
		return 504, apiError{"ai_gateway_timeout", "摘要服务网关等待超时（HTTP 504）。已完成部分已保留，可稍后手动继续；请检查网关超时设置与模型响应速度。"}
	case errors.Is(err, ai.ErrRate):
		return 429, apiError{"ai_rate_limited", "摘要服务提示额度或请求限制，请稍后手动继续。已完成部分仍保留。"}
	case errors.Is(err, ai.ErrTruncated):
		return 502, apiError{"ai_output_truncated", "模型输出达到长度上限，这段摘要未保存。已完成部分仍保留，可手动继续或更换模型。"}
	case errors.Is(err, ai.ErrRefused):
		return 502, apiError{"ai_output_refused", "模型拒绝了这次摘要。已完成部分仍保留，请更换摘要模型后再试。"}
	case errors.Is(err, ai.ErrFormat):
		return 502, apiError{"ai_invalid_output", "这段摘要的格式或原文引用不符，已完成部分仍保留。可手动继续，或更换翻译模型。"}
	case errors.Is(err, ai.ErrStorage):
		return 503, apiError{"ai_storage_failed", "摘要缓存无法读写，原文仍保留。"}
	case errors.Is(err, context.DeadlineExceeded):
		return 504, apiError{"ai_timeout", "摘要等待超时，已完成部分保留，可手动继续。"}
	default:
		return 502, apiError{"ai_provider_failed", "摘要服务暂不可用，请检查接口、模型与网络。"}
	}
}

// classifyChatError mirrors chatFail without writing.
func classifyChatError(err error) (int, apiError) {
	status := http.StatusBadGateway
	if errors.Is(err, ai.ErrInvalid) {
		status = http.StatusBadRequest
	}
	if errors.Is(err, ai.ErrNotReady) {
		status = http.StatusServiceUnavailable
	}
	if errors.Is(err, ai.ErrAuth) {
		status = http.StatusBadGateway
	}
	if errors.Is(err, ai.ErrRate) {
		status = http.StatusTooManyRequests
	}
	if errors.Is(err, ai.ErrUsageUnknown) || errors.Is(err, ai.ErrBudgetExhausted) {
		status = http.StatusTooManyRequests
	}
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	return status, apiError{"ai_chat_failed", aiErrorMessage(err)}
}
