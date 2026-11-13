package ai

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// MaxBudgetedCalls is the largest accepted explicit completion-call limit.
const MaxBudgetedCalls = 100

var (
	// ErrBudgetExhausted reports that the invocation has no remaining
	// reservation. A completion POST must not start.
	ErrBudgetExhausted = errors.New("AI provider call budget exhausted")
	// ErrUsageUnknown reports that a completed provider response carried
	// missing, partial, malformed or inconsistent usage. No later completion
	// POST may start in the same invocation.
	ErrUsageUnknown = errors.New("AI provider usage is unavailable")
	// ErrInvocationFailed reports that a started call already terminated in a
	// provider failure or cancellation. The invocation cannot start another
	// POST; the user must begin a fresh explicit invocation.
	ErrInvocationFailed = errors.New("AI invocation already failed")
)

// Outcome is the explicit terminal result of a service invocation. It is set by
// the service once, independent of how many calls the ledger started, so a
// finished one-call task under limit 1 reports completed rather than
// budget_exhausted. The zero value reports an invocation that has not yet
// reached an explicit terminal outcome.
type Outcome int

const (
	OutcomeActive Outcome = iota
	OutcomeCompleted
	OutcomeBudgetExhausted
	OutcomeUsageUnknown
	OutcomeCancelled
	OutcomeProviderError
)

func (o Outcome) String() string {
	switch o {
	case OutcomeCompleted:
		return "completed"
	case OutcomeBudgetExhausted:
		return "budget_exhausted"
	case OutcomeUsageUnknown:
		return "usage_unknown"
	case OutcomeCancelled:
		return "cancelled"
	case OutcomeProviderError:
		return "provider_error"
	default:
		return "active"
	}
}

// Usage is a validated provider completion usage triple. Valid values are
// nonnegative integers with Total == Prompt + Output.
type Usage struct {
	Prompt int
	Output int
	Total  int
}

func (u Usage) valid() bool {
	return u.Prompt >= 0 && u.Output >= 0 && u.Total >= 0 && u.Total == u.Prompt+u.Output
}

// TerminalState is the invocation outcome relevant to the call budget. The
// zero value reports an invocation that has not reached a terminal budget
// state and still has known usage.
type TerminalState int

const (
	TerminalActive TerminalState = iota
	TerminalCompleted
	TerminalBudgetExhausted
	TerminalUsageUnknown
	TerminalFailed
)

func (s TerminalState) String() string {
	switch s {
	case TerminalCompleted:
		return "completed"
	case TerminalBudgetExhausted:
		return "budget_exhausted"
	case TerminalUsageUnknown:
		return "usage_unknown"
	case TerminalFailed:
		return "failed"
	default:
		return "active"
	}
}

// BudgetSnapshot is a consistent copy of a ledger at one instant.
type BudgetSnapshot struct {
	Unbounded    bool
	Maximum      int
	Started      int
	Remaining    int
	Terminal     TerminalState
	Outcome      Outcome
	UsageKnown   bool
	Calls        []Usage
	Cumulative   Usage
	UsageUnknown bool
}

type ledgerKey struct{}

type invocationLedger struct {
	mu           sync.Mutex
	maximum      int
	started      int
	usage        []Usage
	cumulative   Usage
	usageKnown   bool
	usageUnknown bool
	outcome      Outcome
	// failed records that the invocation has a terminal failure (provider
	// error or cancellation). A failed invocation never starts another POST and
	// a later valid completion cannot overwrite the failed terminal outcome.
	failed bool
}

// NewBudgetContext attaches a fresh per-invocation call ledger to ctx. A
// maximum of 1..MaxBudgetedCalls caps started completion POSTs. Any other value,
// including 0 and negatives, is rejected before the context is derived so no
// provider request can start. Omitting this function keeps upstream behavior:
// no ledger means unlimited calls.
func NewBudgetContext(ctx context.Context, maximum int) (context.Context, error) {
	if maximum < 1 || maximum > MaxBudgetedCalls {
		return ctx, fmt.Errorf("%w: call limit must be between 1 and %d", ErrInvalid, MaxBudgetedCalls)
	}
	ledger := &invocationLedger{maximum: maximum, usageKnown: true}
	return context.WithValue(ctx, ledgerKey{}, ledger), nil
}

func ledgerFrom(ctx context.Context) *invocationLedger {
	if ctx == nil {
		return nil
	}
	ledger, _ := ctx.Value(ledgerKey{}).(*invocationLedger)
	return ledger
}

// BudgetStatus reports the ledger state attached to ctx. A context without a
// ledger reports Unbounded with known usage so callers can preserve upstream
// behavior.
func BudgetStatus(ctx context.Context) BudgetSnapshot {
	ledger := ledgerFrom(ctx)
	if ledger == nil {
		return BudgetSnapshot{Unbounded: true, Terminal: TerminalActive, UsageKnown: true}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.snapshotLocked()
}

func (l *invocationLedger) snapshotLocked() BudgetSnapshot {
	// Whole-task usage is known only when no call is unknown and every started
	// call has reconciled a valid usage triple. An in-flight, failed or
	// cancelled started call leaves usage unknown, so a cumulative zero is never
	// reported as known. Known per-call counts are still surfaced separately.
	wholeKnown := l.usageKnown && !l.usageUnknown && len(l.usage) == l.started
	return BudgetSnapshot{
		Maximum:      l.maximum,
		Started:      l.started,
		Remaining:    l.maximum - l.started,
		Terminal:     l.terminalLocked(),
		Outcome:      l.outcomeLocked(),
		UsageKnown:   wholeKnown,
		Calls:        append([]Usage(nil), l.usage...),
		Cumulative:   l.cumulative,
		UsageUnknown: l.usageUnknown,
	}
}

// terminalLocked derives only the budget/usage terminal state. Service task
// completion is tracked separately in outcome and is never inferred from the
// number of started calls.
func (l *invocationLedger) terminalLocked() TerminalState {
	if l.failed {
		return TerminalFailed
	}
	if l.usageUnknown {
		return TerminalUsageUnknown
	}
	if l.started >= l.maximum {
		return TerminalBudgetExhausted
	}
	return TerminalActive
}

// outcomeLocked reports the explicit service outcome, or a failure/unknown
// derivation when no completed outcome was set. A failed invocation always
// reports its failed outcome, never completed.
func (l *invocationLedger) outcomeLocked() Outcome {
	if l.outcome == OutcomeCompleted && (l.failed || l.usageUnknown) {
		return l.failureOutcomeLocked()
	}
	if l.outcome != OutcomeActive {
		return l.outcome
	}
	if l.failed || l.usageUnknown {
		return l.failureOutcomeLocked()
	}
	return OutcomeActive
}

func (l *invocationLedger) failureOutcomeLocked() Outcome {
	if l.outcome == OutcomeCancelled || l.outcome == OutcomeProviderError {
		return l.outcome
	}
	if l.usageUnknown {
		return OutcomeUsageUnknown
	}
	return OutcomeProviderError
}

// reserve atomically claims the next slot. It never refunds: a claimed slot is
// consumed for the life of the invocation. A blocked start reports the cause so
// an unknown-usage terminal is never misreported as budget exhaustion. After a
// terminal failure no new POST starts; a fresh explicit invocation is required.
func (l *invocationLedger) reserve() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failed {
		return ErrInvocationFailed
	}
	if l.usageUnknown {
		return ErrUsageUnknown
	}
	if l.started >= l.maximum {
		return ErrBudgetExhausted
	}
	l.started++
	return nil
}

// setOutcome records the explicit service terminal result. A terminal failure
// (provider error or cancellation) is sticky: a later valid completion cannot
// clear it, and a completed result cannot overwrite an unknown-usage or failed
// terminal. A completion may still replace an earlier budget-exhausted report,
// since exhaustion is not a failure of an already-started call.
func (l *invocationLedger) setOutcome(outcome Outcome) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if outcome == OutcomeCompleted || outcome == OutcomeBudgetExhausted {
		if l.failed || l.usageUnknown {
			return
		}
	}
	if outcome == OutcomeProviderError || outcome == OutcomeCancelled {
		l.failed = true
	}
	l.outcome = outcome
}

// markFailed records a terminal provider failure or cancellation without
// deriving it from a specific error value. It is used when a started POST ends
// in an ambiguous failure: no further POST may start and the failure is sticky.
func (l *invocationLedger) markFailed(outcome Outcome) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if outcome != OutcomeProviderError && outcome != OutcomeCancelled {
		return
	}
	l.failed = true
	if l.outcome != OutcomeCancelled && l.outcome != OutcomeProviderError {
		l.outcome = outcome
	}
}

// recordValid reconciles a successfully completed POST's usage.
func (l *invocationLedger) recordValid(usage Usage) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recordValidLocked(usage)
}

func (l *invocationLedger) recordValidLocked(usage Usage) {
	if l.usageUnknown || !usage.valid() {
		l.markUnknownLocked()
		return
	}
	cumulative, ok := addUsage(l.cumulative, usage)
	if !ok {
		// Overflow of the cumulative integer sum is malformed usage, never a
		// plausible large count.
		l.markUnknownLocked()
		return
	}
	l.usage = append(l.usage, usage)
	l.cumulative = cumulative
}

// addUsage sums two usage triples, reporting ok=false on integer overflow so a
// wrapped negative or bogus total is treated as malformed rather than known.
func addUsage(left, right Usage) (Usage, bool) {
	prompt, okP := safeAdd(left.Prompt, right.Prompt)
	output, okO := safeAdd(left.Output, right.Output)
	total, okT := safeAdd(left.Total, right.Total)
	if !okP || !okO || !okT || total != prompt+output || prompt < 0 || output < 0 || total < 0 {
		return Usage{}, false
	}
	return Usage{Prompt: prompt, Output: output, Total: total}, true
}

func safeAdd(left, right int) (int, bool) {
	sum := left + right
	if (right > 0 && sum < left) || (right < 0 && sum > left) {
		return 0, false
	}
	return sum, true
}

func (l *invocationLedger) markUnknown() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.markUnknownLocked()
}

func (l *invocationLedger) markUnknownLocked() {
	l.usageUnknown = true
	l.usageKnown = false
}

// BudgetAttached reports whether ctx carries an invocation ledger. Adapters use
// it to preserve upstream behavior when no limit was requested.
func BudgetAttached(ctx context.Context) bool { return ledgerFrom(ctx) != nil }

// ReserveCompletion claims one completion-POST slot from the ledger attached
// to ctx. It is called by provider adapters immediately before transport
// starts. When no ledger is attached it succeeds so upstream behavior is
// unchanged. A reserved slot is never refunded.
func ReserveCompletion(ctx context.Context) error {
	ledger := ledgerFrom(ctx)
	if ledger == nil {
		return nil
	}
	return ledger.reserve()
}

// RecordCompletionUsage reconciles the usage of a POST that definitely
// completed. Invalid triples and cumulative integer overflow never become a
// known zero: they mark the ledger as usage_unknown and stop further starts.
func RecordCompletionUsage(ctx context.Context, usage Usage) {
	ledger := ledgerFrom(ctx)
	if ledger == nil {
		return
	}
	ledger.recordValid(usage)
}

// ReconcileCompletionUsage records a completed POST's usage and reports a
// ledger-only failure. A cumulative overflow or invalid triple marks the
// ledger usage_unknown; it returns ErrUsageUnknown so a caller never emits a
// success for a response whose usage accounting failed.
func ReconcileCompletionUsage(ctx context.Context, usage Usage) error {
	ledger := ledgerFrom(ctx)
	if ledger == nil {
		return nil
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	before := ledger.usageUnknown
	ledger.recordValidLocked(usage)
	if !before && ledger.usageUnknown {
		return ErrUsageUnknown
	}
	return nil
}

// MarkUsageUnknown marks the invocation usage_unknown without recording a
// count. It is used when a POST completed but its usage could not be parsed.
func MarkUsageUnknown(ctx context.Context) {
	ledger := ledgerFrom(ctx)
	if ledger == nil {
		return
	}
	ledger.markUnknown()
}

// SetOutcome records the explicit terminal result of the invocation attached to
// ctx. It is a no-op without a ledger so unbudgeted behavior is unchanged.
func SetOutcome(ctx context.Context, outcome Outcome) {
	ledger := ledgerFrom(ctx)
	if ledger == nil {
		return
	}
	ledger.setOutcome(outcome)
}

// MarkInvocationFailed records a sticky provider failure or cancellation after
// a started POST failed ambiguously. It blocks any further start in the same
// invocation. It is a no-op without a ledger.
func MarkInvocationFailed(ctx context.Context, outcome Outcome) {
	ledger := ledgerFrom(ctx)
	if ledger == nil {
		return
	}
	ledger.markFailed(outcome)
}
