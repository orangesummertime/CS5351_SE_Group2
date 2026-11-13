package aichat

import (
	"context"
	"encoding/json"

	"tangentry/internal/ai"
)

// rawUsage is the shape shared by OpenAI-compatible chat completions and
// Responses envelopes. Pointer fields distinguish an absent value from a
// literal zero so a missing count is never mistaken for a known zero.
type rawUsage struct {
	PromptTokens     *int `json:"prompt_tokens"`
	CompletionTokens *int `json:"completion_tokens"`
	TotalTokens      *int `json:"total_tokens"`
	InputTokens      *int `json:"input_tokens"`
	OutputTokens     *int `json:"output_tokens"`
}

// usageError reports a completed provider response whose usage is missing,
// partial, negative, non-integer or inconsistent. It wraps ErrUsageUnknown so
// callers can distinguish it from transport and format failures.
type usageError struct{}

func (usageError) Error() string { return ai.ErrUsageUnknown.Error() }
func (usageError) Is(target error) bool {
	return target == ai.ErrUsageUnknown
}

// parseUsage validates a raw usage envelope. It returns ok=false for missing,
// partial, negative or inconsistent counts; invalid never becomes a known zero.
func parseUsage(raw *rawUsage) (ai.Usage, bool) {
	if raw == nil {
		return ai.Usage{}, false
	}
	prompt, output, total := raw.PromptTokens, raw.CompletionTokens, raw.TotalTokens
	if prompt == nil {
		prompt = raw.InputTokens
	}
	if output == nil {
		output = raw.OutputTokens
	}
	if prompt == nil || output == nil || total == nil {
		return ai.Usage{}, false
	}
	usage := ai.Usage{Prompt: *prompt, Output: *output, Total: *total}
	if usage.Prompt < 0 || usage.Output < 0 || usage.Total < 0 || usage.Total != usage.Prompt+usage.Output {
		return ai.Usage{}, false
	}
	return usage, true
}

// reconcileUsage records the usage of a POST that definitely completed. A
// missing or invalid usage marks the ledger usage_unknown and returns a
// usageError so callers do not treat the completed response as success. When
// no ledger is attached the upstream behavior is preserved: usage is not
// required and no error is reported.
func reconcileUsage(ctx context.Context, raw *rawUsage) error {
	if !ai.BudgetAttached(ctx) {
		return nil
	}
	usage, ok := parseUsage(raw)
	if !ok {
		ai.MarkUsageUnknown(ctx)
		return usageError{}
	}
	// Reconcile reports a ledger-only failure (for example a cumulative integer
	// overflow) so a completed response is never emitted as success when the
	// usage accounting itself failed.
	if err := ai.ReconcileCompletionUsage(ctx, usage); err != nil {
		return usageError{}
	}
	return nil
}

// parseRawUsage parses a raw usage object. Any type mismatch (for example a
// string count) is reported as an empty rawUsage so it becomes usage_unknown.
func parseRawUsage(data []byte) *rawUsage {
	var raw rawUsage
	if json.Unmarshal(data, &raw) != nil {
		return &rawUsage{}
	}
	return &raw
}

// reserveCompletion claims one completion-POST slot immediately before
// transport starts. Without an attached ledger it is a no-op so unbudgeted
// behavior is preserved.
func reserveCompletion(ctx context.Context) error { return ai.ReserveCompletion(ctx) }
