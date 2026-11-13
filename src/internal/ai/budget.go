package ai

import (
	"context"
	"errors"
)

// BudgetReport is the optional JSON budget report attached to limited
// operations. It is omitted entirely on unbudgeted paths. Unknown usage is
// represented by null counts (nil pointers), never by a known zero.
type BudgetReport struct {
	Max        int           `json:"max"`
	Started    int           `json:"started"`
	UsageKnown bool          `json:"usageKnown"`
	PerCall    []UsageReport `json:"perCall,omitempty"`
	Cumulative UsageReport   `json:"cumulative"`
	Status     string        `json:"status"`
}

// UsageReport is one validated usage triple, or nulls when usage is unknown.
type UsageReport struct {
	Prompt *int `json:"prompt"`
	Output *int `json:"output"`
	Total  *int `json:"total"`
}

func knownUsageReport(usage Usage) UsageReport {
	prompt, output, total := usage.Prompt, usage.Output, usage.Total
	return UsageReport{Prompt: &prompt, Output: &output, Total: &total}
}

func newBudgetReport(snapshot BudgetSnapshot) *BudgetReport {
	status := snapshot.Outcome
	if status == OutcomeActive {
		// Before the service sets an explicit outcome, expose the budget/usage
		// terminal so an unknown, failed or exhausted invocation is never shown
		// active.
		switch snapshot.Terminal {
		case TerminalUsageUnknown:
			status = OutcomeUsageUnknown
		case TerminalBudgetExhausted:
			status = OutcomeBudgetExhausted
		case TerminalFailed:
			status = OutcomeProviderError
		}
	}
	report := &BudgetReport{Max: snapshot.Maximum, Started: snapshot.Started, UsageKnown: snapshot.UsageKnown, Status: status.String()}
	// Known per-call triples are always surfaced, even when the whole-task
	// cumulative count is unknown (an in-flight, failed or unknown call). The
	// cumulative block stays null in that case so a partial total is never
	// shown as a known whole-task count.
	if len(snapshot.Calls) > 0 {
		report.PerCall = make([]UsageReport, len(snapshot.Calls))
		for i, usage := range snapshot.Calls {
			report.PerCall[i] = knownUsageReport(usage)
		}
	}
	if snapshot.UsageKnown {
		report.Cumulative = knownUsageReport(snapshot.Cumulative)
	}
	return report
}

// budgetReportFor returns the report attached to ctx, or nil for unbudgeted
// invocations so callers can omit the field entirely.
func budgetReportFor(ctx context.Context) *BudgetReport {
	if !BudgetAttached(ctx) {
		return nil
	}
	return newBudgetReport(BudgetStatus(ctx))
}

// withBudget attaches a fresh per-invocation ledger when limit is nil. An
// explicit limit outside 1..MaxBudgetedCalls is rejected before deriving the
// context so no provider request can start. A nil limit preserves the parent
// context unchanged, keeping upstream behavior.
func withBudget(ctx context.Context, limit *int) (context.Context, error) {
	if limit == nil {
		return ctx, nil
	}
	return NewBudgetContext(ctx, *limit)
}

// validBudgetLimit reports whether an optional limit is acceptable.
func validBudgetLimit(limit *int) bool {
	return limit == nil || (*limit >= 1 && *limit <= MaxBudgetedCalls)
}

// fallbackAllowed reports whether a provider error permits the chat service's
// non-stream fallback. On a budgeted invocation only a clearly empty stream
// (ErrFormat with no forwarded content) may fall back; ambiguous transport,
// 5xx, usage-unknown and cancellation must not start a further request.
func fallbackAllowed(ctx context.Context, err error) bool {
	if BudgetAttached(ctx) {
		// A terminal failure or unknown usage blocks the fallback; only a
		// demonstrably empty, non-generated stream may fall back, and it must
		// take a fresh reservation later. Malformed/truncated JSON,
		// ambiguous EOF, transport failures, 5xx and cancellation never fall back.
		return errors.Is(err, ErrEmptyCompletion)
	}
	// Upstream behavior: empty stream and provider failures both fall back.
	return errors.Is(err, ErrFormat) || errors.Is(err, ErrProvider)
}

// terminalOutcome classifies a service error into an explicit terminal outcome.
// Budget and unknown-usage states stay distinct from cancellation and generic
// provider failures.
func terminalOutcome(err error) Outcome {
	switch {
	case err == nil:
		return OutcomeActive
	case errors.Is(err, ErrUsageUnknown):
		return OutcomeUsageUnknown
	case errors.Is(err, ErrBudgetExhausted):
		return OutcomeBudgetExhausted
	case errors.Is(err, ErrInvocationFailed):
		return OutcomeProviderError
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return OutcomeCancelled
	default:
		return OutcomeProviderError
	}
}
