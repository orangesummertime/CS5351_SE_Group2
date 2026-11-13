package ai

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

// retryTranslationProvider fails a specific call index with a budget error so
// the service stops after committing earlier batches.
type stagedTranslator struct {
	usage   Usage
	posts   int
	failAt  int
	failErr error
}

func (p *stagedTranslator) Translate(ctx context.Context, _ Config, segments []Segment) ([]Segment, error) {
	if err := ReserveCompletion(ctx); err != nil {
		return nil, err
	}
	p.posts++
	if p.failAt != 0 && p.posts == p.failAt {
		return nil, p.failErr
	}
	RecordCompletionUsage(ctx, p.usage)
	out := make([]Segment, len(segments))
	for i, s := range segments {
		out[i] = Segment{Index: s.Index, Text: "译" + s.Text}
	}
	return out, nil
}

// Gap 1: a limited translation error must return the committed partial result
// and budget report rather than an empty result, so HTTP can render progress.
func TestGap1TranslationErrorReturnsPartialAndBudget(t *testing.T) {
	provider := &stagedTranslator{usage: Usage{1, 1, 2}}
	root := t.TempDir() + "/ai"
	s, err := New(root, NewMemoryResults(), provider)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Configure(ConfigureInput{BaseURL: "https://ai-fixture.example/v1", Model: "fixture", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	rev := s.Status().Revision
	in := translationInput(rev, 24, limit(2))
	in.Source = "article"
	in.Track.Language = "text-article"
	in.ContentID = "rss:" + strings.Repeat("a", 64)
	result, err := s.Translate(context.Background(), in)
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("want budget exhausted, got %v", err)
	}
	if len(result.Texts) != 16 {
		t.Fatalf("partial texts not returned on error: %d", len(result.Texts))
	}
	if result.Budget == nil || result.Budget.Started != 2 || result.Budget.Status != "budget_exhausted" {
		t.Fatalf("budget report missing on error: %+v", result.Budget)
	}
	if !result.Budget.UsageKnown {
		t.Fatal("whole-task usage should be known (both calls reconciled)")
	}
	if result.Budget.Cumulative.Total == nil || *result.Budget.Cumulative.Total != 4 {
		t.Fatalf("cumulative = %+v", result.Budget.Cumulative)
	}
}

// Gap 3: a started but failed call leaves whole-task usage unknown while known
// per-call counts are still surfaced; cumulative stays null.
func TestGap3FailedCallUsageUnknownButPerCallKnown(t *testing.T) {
	provider := &stagedTranslator{usage: Usage{3, 4, 7}, failAt: 2, failErr: ErrProvider}
	root := t.TempDir() + "/ai"
	s, err := New(root, NewMemoryResults(), provider)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Configure(ConfigureInput{BaseURL: "https://ai-fixture.example/v1", Model: "fixture", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	rev := s.Status().Revision
	in := translationInput(rev, 16, nil)
	in.Source = "article"
	in.Track.Language = "text-article"
	in.ContentID = "rss:" + strings.Repeat("b", 64)
	ctx, err := NewBudgetContext(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = s.Translate(ctx, in)
	snapshot := BudgetStatus(ctx)
	if snapshot.UsageKnown {
		t.Fatal("whole-task usage must be unknown when a started call failed")
	}
	report := newBudgetReport(snapshot)
	if report.UsageKnown || report.Cumulative.Total != nil {
		t.Fatalf("cumulative must be null when unknown: %+v", report.Cumulative)
	}
}

// Gap 3: cumulative integer overflow is malformed usage, never a known count.
func TestGap3CumulativeOverflowIsUnknown(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	RecordCompletionUsage(ctx, Usage{Prompt: math.MaxInt - 1, Output: 1, Total: math.MaxInt})
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	// This call is individually valid but overflows the cumulative sum.
	RecordCompletionUsage(ctx, Usage{Prompt: 2, Output: 0, Total: 2})
	snapshot := BudgetStatus(ctx)
	if !snapshot.UsageUnknown || snapshot.UsageKnown {
		t.Fatalf("overflow must be unknown, not known: %+v", snapshot)
	}
}

// Gap 4: a later valid completion cannot clear an existing unknown terminal.
func TestGap4LaterValidCannotClearUnknown(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	MarkUsageUnknown(ctx)
	if err := ReserveCompletion(ctx); !errors.Is(err, ErrUsageUnknown) {
		t.Fatalf("blocked start must be usage_unknown: %v", err)
	}
	// Attempting to record a later valid completion must not revive the ledger.
	RecordCompletionUsage(ctx, Usage{1, 1, 2})
	SetOutcome(ctx, OutcomeCompleted)
	if snapshot := BudgetStatus(ctx); !snapshot.UsageUnknown || snapshot.Outcome == OutcomeCompleted {
		t.Fatalf("unknown terminal revived: %+v", snapshot)
	}
}

// Gap 4: a nested service call with no explicit limit reuses the inherited
// ledger instead of resetting it.
func TestGap4NestedCallReusesInheritedLedger(t *testing.T) {
	provider := &stagedTranslator{usage: Usage{1, 1, 2}}
	root := t.TempDir() + "/ai"
	s, err := New(root, NewMemoryResults(), provider)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Configure(ConfigureInput{BaseURL: "https://ai-fixture.example/v1", Model: "fixture", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	rev := s.Status().Revision
	parent, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	// Nested translate with nil limit must consume the inherited slot.
	if _, err := s.Translate(parent, translationInput(rev, 1, nil)); err != nil {
		t.Fatalf("nested translate: %v", err)
	}
	if snapshot := BudgetStatus(parent); snapshot.Started != 1 {
		t.Fatalf("nested call did not use parent ledger: %+v", snapshot)
	}
	if err := ReserveCompletion(parent); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatal("nested call created a fresh ledger")
	}
}
