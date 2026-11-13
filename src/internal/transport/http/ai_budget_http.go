package httptransport

import (
	"context"
	"errors"
	"net/http"

	"tangentry/internal/ai"
)

// aiBudgetError is the additive JSON error body for limited AI operations. It
// keeps the existing code/message contract and adds the budget report plus any
// valid partial progress so a client can render counters and partial output
// without losing the derived ledger. For unbudgeted operations budget is nil
// and the body stays byte-compatible with apiError.
type aiBudgetError struct {
	apiError
	Budget   *ai.BudgetReport  `json:"budget,omitempty"`
	Progress *aiPartialState   `json:"progress,omitempty"`
}

// aiPartialState is the renderable partial state carried on AI task errors.
type aiPartialState struct {
	Texts          []string         `json:"texts,omitempty"`
	Complete       bool             `json:"complete"`
	CompletedCalls int              `json:"completedCalls,omitempty"`
	TotalCalls     int              `json:"totalCalls,omitempty"`
	Answer         string           `json:"answer,omitempty"`
	Messages       []ai.ChatMessage `json:"messages,omitempty"`
}

// writeAIBudgetError writes the budgeted error body when the result carries a
// limited-run report, otherwise it falls back to the plain apiError so
// unbudgeted wire behavior is unchanged.
func writeAIBudgetError(w http.ResponseWriter, status int, err apiError, budget *ai.BudgetReport, progress *aiPartialState) {
	if budget == nil {
		write(w, status, err)
		return
	}
	write(w, status, aiBudgetError{apiError: err, Budget: budget, Progress: progress})
}

func translationPartial(result ai.Result) *aiPartialState {
	if len(result.Texts) == 0 {
		return nil
	}
	return &aiPartialState{Texts: result.Texts, Complete: result.Total > 0 && len(result.Texts) == result.Total}
}

func summaryPartial(result ai.SummaryResult) *aiPartialState {
	if result.CompletedCalls == 0 && !result.Complete {
		return nil
	}
	return &aiPartialState{Complete: result.Complete, CompletedCalls: result.CompletedCalls, TotalCalls: result.TotalCalls}
}

func chatPartial(result ai.ChatResult) *aiPartialState {
	if len(result.Messages) == 0 {
		return nil
	}
	return &aiPartialState{Messages: result.Messages}
}

// aiBudgetedChatResult wraps a chat result with its completion status for the
// limited SSE done frame; the embedded fields keep the existing wire shape.
type aiBudgetedChatResult struct {
	ai.ChatResult
	Status string `json:"status"`
}

// terminalStatusFromError maps a stream error to a stable status string for the
// SSE error terminal, consistent with BudgetReport.status values.
func terminalStatusFromError(err error) string {
	switch {
	case errors.Is(err, ai.ErrUsageUnknown):
		return "usage_unknown"
	case errors.Is(err, ai.ErrBudgetExhausted):
		return "budget_exhausted"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "cancelled"
	default:
		return "provider_error"
	}
}
