package ai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"testing"
)

func TestLedgerOmittedLimitPreservesUpstreamBehavior(t *testing.T) {
	ctx := context.Background()
	if BudgetAttached(ctx) {
		t.Fatal("plain context must not carry a ledger")
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatalf("unbudgeted reservation must succeed: %v", err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatalf("unbudgeted reservation must stay unlimited: %v", err)
	}
	RecordCompletionUsage(ctx, Usage{Prompt: -1, Output: 2, Total: 1})
	MarkUsageUnknown(ctx)
	snapshot := BudgetStatus(ctx)
	if !snapshot.Unbounded || snapshot.Maximum != 0 || snapshot.Started != 0 || !snapshot.UsageKnown || snapshot.UsageUnknown || snapshot.Terminal != TerminalActive {
		t.Fatalf("unbudgeted snapshot = %+v", snapshot)
	}
}

func TestLedgerInvalidLimitsFailBeforeAnyRequest(t *testing.T) {
	for _, limit := range []int{-5, -1, 0, 101, 1000} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			ctx := context.Background()
			got, err := NewBudgetContext(ctx, limit)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("limit %d must be rejected: %v", limit, err)
			}
			if got != ctx || BudgetAttached(got) {
				t.Fatal("invalid limit must not attach a ledger")
			}
			if err := ReserveCompletion(got); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLedgerAcceptsBoundaryLimits(t *testing.T) {
	for _, limit := range []int{1, 100} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			ctx, err := NewBudgetContext(context.Background(), limit)
			if err != nil {
				t.Fatal(err)
			}
			snapshot := BudgetStatus(ctx)
			if snapshot.Maximum != limit || snapshot.Remaining != limit || snapshot.Unbounded {
				t.Fatalf("snapshot = %+v", snapshot)
			}
		})
	}
}

func TestLedgerTracksPerCallAndCumulativeUsage(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, usage := range []Usage{{Prompt: 3, Output: 2, Total: 5}, {Prompt: 10, Output: 7, Total: 17}} {
		if err := ReserveCompletion(ctx); err != nil {
			t.Fatal(err)
		}
		RecordCompletionUsage(ctx, usage)
	}
	snapshot := BudgetStatus(ctx)
	if snapshot.Started != 2 || snapshot.Remaining != 1 || !snapshot.UsageKnown {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if len(snapshot.Calls) != 2 || snapshot.Calls[0] != (Usage{3, 2, 5}) || snapshot.Calls[1] != (Usage{10, 7, 17}) {
		t.Fatalf("per-call = %+v", snapshot.Calls)
	}
	if snapshot.Cumulative != (Usage{Prompt: 13, Output: 9, Total: 22}) {
		t.Fatalf("cumulative = %+v", snapshot.Cumulative)
	}
}

func TestLedgerInvalidUsageBecomesUnknownNeverKnownZero(t *testing.T) {
	cases := []struct {
		name  string
		usage Usage
	}{
		{"negative prompt", Usage{Prompt: -1, Output: 1, Total: 0}},
		{"negative output", Usage{Prompt: 1, Output: -1, Total: 0}},
		{"negative total", Usage{Prompt: 1, Output: 1, Total: -2}},
		{"total mismatch", Usage{Prompt: 1, Output: 1, Total: 3}},
		{"total below sum", Usage{Prompt: 5, Output: 5, Total: 9}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, err := NewBudgetContext(context.Background(), 5)
			if err != nil {
				t.Fatal(err)
			}
			if err := ReserveCompletion(ctx); err != nil {
				t.Fatal(err)
			}
			RecordCompletionUsage(ctx, tc.usage)
			snapshot := BudgetStatus(ctx)
			if snapshot.UsageKnown || !snapshot.UsageUnknown || snapshot.Terminal != TerminalUsageUnknown {
				t.Fatalf("invalid usage must be unknown: %+v", snapshot)
			}
			if len(snapshot.Calls) != 0 || snapshot.Cumulative != (Usage{}) {
				t.Fatalf("invalid usage must not record counts: %+v", snapshot)
			}
			if err := ReserveCompletion(ctx); !errors.Is(err, ErrUsageUnknown) || errors.Is(err, ErrBudgetExhausted) {
				t.Fatalf("usage_unknown must block further starts as usage_unknown: %v", err)
			}
		})
	}
}

func TestLedgerUnknownBlocksSameInvocationButFreshInvocationWorks(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	MarkUsageUnknown(ctx)
	if err := ReserveCompletion(ctx); !errors.Is(err, ErrUsageUnknown) {
		t.Fatalf("blocked invocation must report usage_unknown: %v", err)
	}
	fresh, err := NewBudgetContext(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(fresh); err != nil {
		t.Fatalf("fresh invocation blocked: %v", err)
	}
	if BudgetStatus(fresh).Terminal == TerminalUsageUnknown {
		t.Fatal("fresh invocation inherited unknown state")
	}
}

func TestLedgerExhaustionAndTerminalStates(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("limit 1 allowed a second start: %v", err)
	}
	if BudgetStatus(ctx).Terminal != TerminalBudgetExhausted {
		t.Fatalf("terminal = %v", BudgetStatus(ctx).Terminal)
	}
	// The ledger terminal reflects remaining capacity. Task completion is an
	// explicit service outcome set separately and never inferred from started.
	RecordCompletionUsage(ctx, Usage{Prompt: 2, Output: 3, Total: 5})
	if BudgetStatus(ctx).Terminal != TerminalBudgetExhausted {
		t.Fatalf("terminal after valid usage at limit = %v", BudgetStatus(ctx).Terminal)
	}
	SetOutcome(ctx, OutcomeCompleted)
	if BudgetStatus(ctx).Outcome != OutcomeCompleted {
		t.Fatalf("outcome = %v", BudgetStatus(ctx).Outcome)
	}
}

func TestLedgerConcurrentReservationOfLastSlot(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	const goroutines = 64
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		granted int
		denied  int
	)
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := ReserveCompletion(ctx)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				granted++
			case errors.Is(err, ErrBudgetExhausted):
				denied++
			default:
				t.Errorf("unexpected reservation error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if granted != 1 || denied != goroutines-1 {
		t.Fatalf("granted=%d denied=%d", granted, denied)
	}
	if snapshot := BudgetStatus(ctx); snapshot.Started != 1 {
		t.Fatalf("started = %d", snapshot.Started)
	}
}

func TestLedgerIsolationBetweenConcurrentInvocations(t *testing.T) {
	first, _ := NewBudgetContext(context.Background(), 1)
	second, _ := NewBudgetContext(context.Background(), 1)
	if err := ReserveCompletion(first); err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(second); err != nil {
		t.Fatalf("second invocation shares first budget: %v", err)
	}
	if err := ReserveCompletion(first); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatal("first invocation not exhausted")
	}
	if err := ReserveCompletion(second); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatal("second invocation not exhausted")
	}
}

func TestLedgerSnapshotIsACopy(t *testing.T) {
	ctx, _ := NewBudgetContext(context.Background(), 3)
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	RecordCompletionUsage(ctx, Usage{Prompt: 1, Output: 1, Total: 2})
	snapshot := BudgetStatus(ctx)
	snapshot.Calls[0] = Usage{Prompt: 99, Output: 99, Total: 198}
	if BudgetStatus(ctx).Calls[0] != (Usage{1, 1, 2}) {
		t.Fatal("snapshot aliases ledger storage")
	}
}

// A late valid call must not overwrite an earlier usage_unknown terminal, and a
// completed outcome set after unknown stays usage_unknown.
func TestLedgerUnknownNotOverwrittenByLateCompleted(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	MarkUsageUnknown(ctx)
	// A subsequent in-flight call that returns valid usage cannot clear unknown.
	RecordCompletionUsage(ctx, Usage{Prompt: 1, Output: 1, Total: 2})
	SetOutcome(ctx, OutcomeCompleted)
	SetOutcome(ctx, OutcomeBudgetExhausted)
	snapshot := BudgetStatus(ctx)
	if !snapshot.UsageUnknown || snapshot.UsageKnown || snapshot.Outcome != OutcomeUsageUnknown {
		t.Fatalf("unknown terminal was overwritten: %+v", snapshot)
	}
	if len(snapshot.Calls) != 0 {
		t.Fatalf("late call recorded a known count after unknown: %+v", snapshot.Calls)
	}
	if err := ReserveCompletion(ctx); !errors.Is(err, ErrUsageUnknown) {
		t.Fatalf("late call allowed a further POST: %v", err)
	}
}

// Gap: after a terminal provider failure no new POST may start, and a later
// valid completion cannot overwrite the failed terminal outcome.
func TestLedgerProviderFailureBlocksStartsAndIsSticky(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	SetOutcome(ctx, OutcomeProviderError)
	MarkInvocationFailed(ctx, OutcomeProviderError)
	if err := ReserveCompletion(ctx); !errors.Is(err, ErrInvocationFailed) {
		t.Fatalf("start after provider failure must be blocked: %v", err)
	}
	RecordCompletionUsage(ctx, Usage{1, 1, 2})
	SetOutcome(ctx, OutcomeCompleted)
	if snapshot := BudgetStatus(ctx); snapshot.Outcome != OutcomeProviderError || snapshot.Terminal != TerminalFailed {
		t.Fatalf("provider failure was overwritten: %+v", snapshot)
	}
}

// Gap: after cancellation no new POST may start, and the cancelled terminal is
// sticky against a later completed outcome.
func TestLedgerCancellationBlocksStartsAndIsSticky(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	SetOutcome(ctx, OutcomeCancelled)
	MarkInvocationFailed(ctx, OutcomeCancelled)
	if err := ReserveCompletion(ctx); !errors.Is(err, ErrInvocationFailed) {
		t.Fatalf("start after cancellation must be blocked: %v", err)
	}
	SetOutcome(ctx, OutcomeCompleted)
	if snapshot := BudgetStatus(ctx); snapshot.Outcome != OutcomeCancelled {
		t.Fatalf("cancelled terminal overwritten: %+v", snapshot)
	}
}

// Gap: a compatible retry (fresh reservation) before a failure is declared is
// still allowed; the failure terminal only sticks once declared.
func TestLedgerCompatibleRetryAllowedBeforeFailureDeclaration(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	// First attempt fails with a compatibility rejection but does not declare a
	// terminal failure, so the compatible retry may take the second slot.
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatalf("compatible retry must be allowed: %v", err)
	}
	RecordCompletionUsage(ctx, Usage{2, 3, 5})
	if snapshot := BudgetStatus(ctx); snapshot.Started != 2 || snapshot.Outcome == OutcomeProviderError {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// Gap: reusing a failed shared parent ledger must not start another request.
func TestLedgerFailedParentBlocksNestedInvocation(t *testing.T) {
	parent, err := NewBudgetContext(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(parent); err != nil {
		t.Fatal(err)
	}
	MarkInvocationFailed(parent, OutcomeProviderError)
	// A nested service call with no explicit limit reuses the parent ledger.
	nested := parent
	if err := ReserveCompletion(nested); !errors.Is(err, ErrInvocationFailed) {
		t.Fatalf("nested invocation started on a failed parent: %v", err)
	}
}

// Gap: cumulative overflow marks the ledger unknown and ReconcileCompletionUsage
// returns ErrUsageUnknown so HTTP never emits success for a ledger-only failure.
func TestLedgerOverflowReconcileReturnsUnknown(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileCompletionUsage(ctx, Usage{Prompt: math.MaxInt - 1, Output: 1, Total: math.MaxInt}); err != nil {
		t.Fatalf("first valid usage: %v", err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ReconcileCompletionUsage(ctx, Usage{Prompt: 2, Output: 0, Total: 2}); !errors.Is(err, ErrUsageUnknown) {
		t.Fatalf("overflow must return ErrUsageUnknown: %v", err)
	}
	if snapshot := BudgetStatus(ctx); !snapshot.UsageUnknown || snapshot.UsageKnown {
		t.Fatalf("overflow must mark unknown: %+v", snapshot)
	}
}
