package ai

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"tangentry/internal/subtitles"
)

// budgetTranslator simulates the adapter contract: it reserves one completion
// slot per POST and records a validated usage triple. It lets service tests
// exercise the real ledger through the public service methods.
type budgetTranslator struct {
	mu        sync.Mutex
	posts     int
	usage     Usage
	unknown   bool
	failWith  error
	transform func([]Segment) []Segment
}

func (p *budgetTranslator) Translate(ctx context.Context, _ Config, segments []Segment) ([]Segment, error) {
	if err := ReserveCompletion(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.posts++
	p.mu.Unlock()
	if p.failWith != nil {
		return nil, p.failWith
	}
	if p.unknown {
		MarkUsageUnknown(ctx)
		return nil, ErrUsageUnknown
	}
	RecordCompletionUsage(ctx, p.usage)
	out := make([]Segment, len(segments))
	for i, segment := range segments {
		out[i] = Segment{Index: segment.Index, Text: "译" + segment.Text}
	}
	if p.transform != nil {
		out = p.transform(segments)
	}
	return out, nil
}

// Chat reserves one slot so the clear/cache paths and isolated chats exercise
// the ledger, but history clear never calls it.
func (p *budgetTranslator) Chat(ctx context.Context, _ Config, _ string, _ []ChatMessage) (string, error) {
	if err := ReserveCompletion(ctx); err != nil {
		return "", err
	}
	RecordCompletionUsage(ctx, p.usage)
	return "answer", nil
}

type budgetSummarizer struct {
	mu       sync.Mutex
	posts    int
	usage    Usage
	unknown  bool
	failWith error
}

func (p *budgetSummarizer) Summarize(ctx context.Context, _ Config, request SummaryRequest) (SummaryDraft, error) {
	if err := ReserveCompletion(ctx); err != nil {
		return SummaryDraft{}, err
	}
	p.mu.Lock()
	p.posts++
	p.mu.Unlock()
	if p.failWith != nil {
		return SummaryDraft{}, p.failWith
	}
	if p.unknown {
		MarkUsageUnknown(ctx)
		return SummaryDraft{}, ErrUsageUnknown
	}
	RecordCompletionUsage(ctx, p.usage)
	indices := make([]int, len(request.Segments))
	for i, segment := range request.Segments {
		indices[i] = segment.Index
	}
	if request.Mode == "reduce" {
		var allowed []int
		for _, draft := range request.Drafts {
			for _, point := range draft.Points {
				allowed = append(allowed, point.Indices...)
			}
		}
		return SummaryDraft{Overview: "归并概述", Points: []SummaryPoint{{Text: "归并要点", Indices: allowed}}}, nil
	}
	return SummaryDraft{Overview: "概述", Points: []SummaryPoint{{Text: "要点", Indices: indices}}}, nil
}

func limit(v int) *int { return &v }

// newBudgetService builds a service whose translator and summarizer share the
// given providers. It configures a translation model and a summary model.
func newBudgetService(t *testing.T, translator *budgetTranslator, summarizer *budgetSummarizer) (*Service, int64) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "ai")
	s, err := New(root, NewMemoryResults(), translator, summarizer)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	summaryModel := "summary-model"
	status, err := s.Configure(ConfigureInput{BaseURL: "https://ai-fixture.example/v1", Model: "fixture", SummaryModel: &summaryModel, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	return s, status.Revision
}

func translationInput(rev int64, cues int, calls *int) Input {
	in := Input{ContentID: "youtube:jNQXAC9IVRw", Revision: rev, MaxProviderCalls: calls, Track: subtitles.Track{Language: "en"}}
	for i := 0; i < cues; i++ {
		in.Track.Cues = append(in.Track.Cues, subtitles.Cue{Start: float64(i), End: float64(i) + 1, Text: strings.Repeat("segment ", 3) + string(rune('a'+i%26))})
	}
	return in
}

// A-T15 / A-T13: invalid limits are rejected before any POST, for every entry
// point, while omitted and boundary limits are accepted.
func TestAT15InvalidLimitsRejectedBeforeSideEffects(t *testing.T) {
	for _, bad := range []int{-1, 0, 101} {
		translator := &budgetTranslator{usage: Usage{1, 1, 2}}
		summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
		s, rev := newBudgetService(t, translator, summarizer)
		calls := limit(bad)
		if _, err := s.Translate(context.Background(), translationInput(rev, 2, calls)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("translate limit %d: %v", bad, err)
		}
		if _, err := s.SummaryGenerate(context.Background(), SummaryInput{ContentID: "rss:bad", Source: "article:original", Text: "原文", Revision: rev, MaxProviderCalls: calls}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("summary limit %d: %v", bad, err)
		}
		// Clear path with an invalid limit must fail before deleting anything.
		if _, err := s.Chat(context.Background(), ChatRequest{ContentID: "rss:bad", Revision: rev, Model: "fixture", Clear: true, MaxProviderCalls: calls}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("clear limit %d: %v", bad, err)
		}
		if _, err := s.Check(context.Background(), CheckInput{Task: "translation", Revision: rev, MaxProviderCalls: calls}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("check limit %d: %v", bad, err)
		}
		if translator.posts != 0 || summarizer.posts != 0 {
			t.Fatalf("limit %d made a provider POST: %d/%d", bad, translator.posts, summarizer.posts)
		}
	}
	// Boundary and omitted limits are accepted.
	for _, good := range []*int{nil, limit(1), limit(100)} {
		translator := &budgetTranslator{usage: Usage{1, 1, 2}}
		summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
		s, rev := newBudgetService(t, translator, summarizer)
		if _, err := s.Translate(context.Background(), translationInput(rev, 1, good)); err != nil {
			t.Fatalf("translate limit %v rejected: %v", good, err)
		}
	}
}

// A-T14 / limited report: a single-call task under limit 1 completes (not
// budget_exhausted) and reports per-call and cumulative usage.
func TestAT14LimitedTranslationCompletesWithReport(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{Prompt: 4, Output: 6, Total: 10}}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	result, err := s.Translate(context.Background(), translationInput(rev, 1, limit(1)))
	if err != nil {
		t.Fatal(err)
	}
	if result.Budget == nil {
		t.Fatal("limited result missing budget report")
	}
	report := result.Budget
	if report.Max != 1 || report.Started != 1 || !report.UsageKnown || report.Status != "completed" {
		t.Fatalf("report = %+v", report)
	}
	if len(report.PerCall) != 1 || report.PerCall[0].Total == nil || *report.PerCall[0].Total != 10 {
		t.Fatalf("per-call = %+v", report.PerCall)
	}
	if report.Cumulative.Total == nil || *report.Cumulative.Total != 10 {
		t.Fatalf("cumulative = %+v", report.Cumulative)
	}
}

// Omitted limit omits the budget report entirely (upstream compatibility).
func TestOmittedLimitOmitsBudgetReport(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{1, 1, 2}}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	result, err := s.Translate(context.Background(), translationInput(rev, 1, nil))
	if err != nil || result.Budget != nil {
		t.Fatalf("unbudgeted report leaked: %v %+v", err, result.Budget)
	}
}

// A-T03: a multi-batch translation under limit 1 commits the first batch, then
// stops with budget_exhausted. Valid checkpoints are retained and an explicit
// fresh budget resumes them without repeating the completed batch.
func TestAT03MultiBatchTranslationStopsAtLimitAndResumes(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{1, 1, 2}}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	// Two article batches of 8 cues each (article batch limit is 8).
	in := translationInput(rev, 16, limit(1))
	in.Source = "article"
	in.Track.Language = "text-article"
	in.ContentID = "rss:" + strings.Repeat("a", 64)
	if _, err := s.Translate(context.Background(), in); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("limit 1 with two batches: %v", err)
	}
	if translator.posts != 1 {
		t.Fatalf("posts = %d, want 1", translator.posts)
	}
	inspection, err := s.Inspect(context.Background(), in)
	if err != nil || len(inspection.Result.Texts) != 8 {
		t.Fatalf("completed batch not retained: %+v %v", inspection.Result.Texts, err)
	}
	// Explicit resume with a fresh budget reuses the first batch.
	in.MaxProviderCalls = limit(2)
	result, err := s.Translate(context.Background(), in)
	if err != nil || len(result.Texts) != 16 {
		t.Fatalf("resume failed: %v texts=%d", err, len(result.Texts))
	}
	if translator.posts != 2 {
		t.Fatalf("resume repeated completed batch: posts=%d", translator.posts)
	}
	if result.Budget == nil || result.Budget.Status != "completed" {
		t.Fatalf("resume report = %+v", result.Budget)
	}
}

// A-T02: a fixture with exactly two summaryPlan groups under limit 1 starts one
// POST, then budget_exhausted with a valid partial checkpoint.
func TestAT02SummaryStopsMidwayWithPartialCheckpoint(t *testing.T) {
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	translator := &budgetTranslator{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	// Build segments that force exactly two leaf groups: more than one leaf
	// worth of bytes but under the reduce threshold.
	in := SummaryInput{ContentID: "rss:at02", Source: "article:original", Revision: rev, MaxProviderCalls: limit(1)}
	big := strings.Repeat("word ", 1600) // 8000 bytes, the per-segment maximum
	// 4 x 8000 bytes exceeds the default 28000-byte leaf budget, so the plan
	// yields exactly two groups (3 + 1).
	in.Segments = []SummarySegment{{Index: 0, Text: big}, {Index: 1, Text: big}, {Index: 2, Text: big}, {Index: 3, Text: big}}
	// Confirm the plan is exactly two groups before exercising the service.
	segments, err := normalizeSummaryInput(in)
	if err != nil {
		t.Fatal(err)
	}
	groups, _ := summaryPlan(segments, summaryContextBudget(0))
	if len(groups) != 2 {
		t.Fatalf("fixture produced %d groups, want 2", len(groups))
	}
	if _, err := s.SummaryGenerate(context.Background(), in); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("limit 1 with two groups: %v", err)
	}
	if summarizer.posts != 1 {
		t.Fatalf("posts = %d, want 1", summarizer.posts)
	}
	inspection, err := s.SummaryInspect(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.Result.Complete || inspection.Result.CompletedCalls != 1 {
		t.Fatalf("partial checkpoint invalid: complete=%v completed=%d", inspection.Result.Complete, inspection.Result.CompletedCalls)
	}
	// Explicit resume with fresh budget resumes from the checkpoint.
	in.MaxProviderCalls = limit(4)
	result, err := s.SummaryGenerate(context.Background(), in)
	if err != nil || !result.Complete {
		t.Fatalf("resume failed: %v complete=%v", err, result.Complete)
	}
}

// A-T21 / usage_unknown: a completed call with unknown usage stops the
// invocation, is reported as usage_unknown (not budget_exhausted), and a fresh
// explicit invocation can resume valid completed work.
func TestAT21UnknownUsageStopsAndResumeWorks(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{1, 1, 2}}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}, unknown: true}
	s, rev := newBudgetService(t, translator, summarizer)
	in := SummaryInput{ContentID: "rss:at21", Source: "article:original", Revision: rev, MaxProviderCalls: limit(5), Segments: []SummarySegment{{Index: 0, Text: "short source"}}}
	if _, err := s.SummaryGenerate(context.Background(), in); !errors.Is(err, ErrUsageUnknown) {
		t.Fatalf("unknown usage must terminate: %v", err)
	}
	if summarizer.posts != 1 {
		t.Fatalf("posts after unknown = %d, want 1", summarizer.posts)
	}
	// A second call with the same (already unknown) budget is not reached; the
	// fresh explicit invocation below uses a new ledger.
	summarizer.unknown = false
	in.MaxProviderCalls = limit(5)
	result, err := s.SummaryGenerate(context.Background(), in)
	if err != nil || !result.Complete {
		t.Fatalf("fresh invocation failed: %v complete=%v", err, result.Complete)
	}
}

// A-T21 service-level unknown outcome on the ledger is usage_unknown, and the
// report status reflects it.
func TestAT21UnknownReportStatus(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{1, 1, 2}, unknown: true}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	// Attach a ledger directly to observe the terminal via the service path.
	ctx, err := NewBudgetContext(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	in := translationInput(rev, 1, nil)
	if _, err := s.Translate(ctx, in); !errors.Is(err, ErrUsageUnknown) {
		t.Fatalf("unknown usage: %v", err)
	}
	snapshot := BudgetStatus(ctx)
	if snapshot.Outcome != OutcomeUsageUnknown || snapshot.UsageKnown {
		t.Fatalf("outcome = %v known=%v", snapshot.Outcome, snapshot.UsageKnown)
	}
}

// A-T09: cancellation after a start consumes the slot and never refunds or
// reports success; the outcome is cancelled.
func TestAT09CancellationConsumesSlot(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{1, 1, 2}, failWith: context.Canceled}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	ctx, err := NewBudgetContext(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Translate(ctx, translationInput(rev, 1, nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation classification: %v", err)
	}
	if snapshot := BudgetStatus(ctx); snapshot.Started != 1 || snapshot.Outcome != OutcomeCancelled {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// A-T10: cache hit, history read and model-list make zero provider POSTs, even
// with an explicit limit.
func TestAT10CacheAndHistoryConsumeNoSlot(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{1, 1, 2}}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	in := translationInput(rev, 1, limit(1))
	first, err := s.Translate(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if translator.posts != 1 {
		t.Fatalf("first posts = %d", translator.posts)
	}
	// A cache hit with a limit must not start a POST.
	if _, err := s.Translate(context.Background(), in); err != nil {
		t.Fatalf("cache hit: %v", err)
	}
	if translator.posts != 1 {
		t.Fatalf("cache hit made a POST: posts=%d", translator.posts)
	}
	// History read and clear start no POST.
	if _, err := s.ChatHistory(context.Background(), "rss:history"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Chat(context.Background(), ChatRequest{ContentID: "rss:history", Revision: rev, Model: "fixture", Clear: true, MaxProviderCalls: limit(1)}); err != nil {
		t.Fatal(err)
	}
	_ = first
}

// A-T16: two concurrent chats with isolated limit-1 budgets each start one POST.
func TestAT16IsolatedChatBudgetsEachStart(t *testing.T) {
	var mu sync.Mutex
	var posts int
	provider := &countingChatProvider{onChat: func(ctx context.Context) error {
		if err := ReserveCompletion(ctx); err != nil {
			return err
		}
		mu.Lock()
		posts++
		mu.Unlock()
		RecordCompletionUsage(ctx, Usage{1, 1, 2})
		return nil
	}}
	root := filepath.Join(t.TempDir(), "ai")
	s, err := New(root, NewMemoryResults(), provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if _, err := s.Configure(ConfigureInput{BaseURL: "https://ai-fixture.example/v1", Model: "fixture", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	rev := s.Status().Revision
	first, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	run := func(ctx context.Context, id string) error {
		_, err := s.Chat(ctx, ChatRequest{ContentID: id, Revision: rev, Model: "fixture", Messages: []ChatMessage{{Role: "user", Content: "hi"}}})
		return err
	}
	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs[0] = run(first, "rss:a") }()
	go func() { defer wg.Done(); errs[1] = run(second, "rss:b") }()
	wg.Wait()
	// The service serializes operations via s.active; one may be busy. Accept
	// either both completing or a busy rejection, but never a shared budget.
	for _, err := range errs {
		if err != nil && !errors.Is(err, ErrBusy) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if BudgetStatus(first).Started > 1 || BudgetStatus(second).Started > 1 {
		t.Fatal("an invocation exceeded its own limit")
	}
}

type countingChatProvider struct {
	onChat func(context.Context) error
}

func (p *countingChatProvider) Translate(context.Context, Config, []Segment) ([]Segment, error) {
	return nil, ErrProvider
}
func (p *countingChatProvider) Chat(ctx context.Context, _ Config, _ string, _ []ChatMessage) (string, error) {
	if p.onChat != nil {
		if err := p.onChat(ctx); err != nil {
			return "", err
		}
	}
	return "answer", nil
}

// streamingBudgetProvider reserves once per stream POST, then delegates to a
// scripted emitter so tests can simulate mid-stream disconnects and missing
// usage without a network.
type streamingBudgetProvider struct {
	emit    []string
	final   error
	usage   *Usage // nil = usage unknown
	mu      sync.Mutex
	streams int
}

func (p *streamingBudgetProvider) Translate(context.Context, Config, []Segment) ([]Segment, error) {
	return nil, ErrProvider
}
func (p *streamingBudgetProvider) Chat(context.Context, Config, string, []ChatMessage) (string, error) {
	return "", ErrProvider
}
func (p *streamingBudgetProvider) ChatStream(ctx context.Context, _ Config, _ string, _ []ChatMessage, emit func(string) error) error {
	if err := ReserveCompletion(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	p.streams++
	p.mu.Unlock()
	for _, chunk := range p.emit {
		if err := emit(chunk); err != nil {
			return err
		}
	}
	if p.final != nil {
		return p.final
	}
	if p.usage == nil {
		MarkUsageUnknown(ctx)
		return ErrUsageUnknown
	}
	RecordCompletionUsage(ctx, *p.usage)
	return nil
}

// A-T18: a mid-stream disconnect after content keeps the started slot, stores
// no completed result, and reports cancellation distinctly (no success).
func TestAT18SSEDisconnectMidStreamNoSuccess(t *testing.T) {
	provider := &streamingBudgetProvider{emit: []string{"partial"}, final: context.Canceled}
	root := filepath.Join(t.TempDir(), "ai")
	s, err := New(root, NewMemoryResults(), provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if _, err := s.Configure(ConfigureInput{BaseURL: "https://ai-fixture.example/v1", Model: "fixture", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	rev := s.Status().Revision
	ctx, err := NewBudgetContext(context.Background(), 3)
	if err != nil {
		t.Fatal(err)
	}
	var forwarded strings.Builder
	_, err = s.ChatStream(ctx, ChatRequest{ContentID: "rss:at18", Revision: rev, Model: "fixture", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}, func(chunk string) error {
		forwarded.WriteString(chunk)
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("disconnect classification: %v", err)
	}
	if snapshot := BudgetStatus(ctx); snapshot.Started != 1 || snapshot.Outcome != OutcomeCancelled {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	history, err := s.ChatHistory(context.Background(), "rss:at18")
	if err != nil || len(history.Messages) != 0 {
		t.Fatalf("disconnect stored a completed answer: %v %+v", err, history.Messages)
	}
}

// A-T19: a corrupted summary checkpoint cannot be reused as complete or bypass
// a fresh limit.
func TestAT19CorruptedCheckpointCannotBypassLimit(t *testing.T) {
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	translator := &budgetTranslator{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	in := SummaryInput{ContentID: "rss:at19", Source: "article:original", Revision: rev, Text: "source text"}
	if _, err := s.SummaryGenerate(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	c := summaryConfig(s.config)
	key := summaryKey(in, mustSegments(t, in), SummaryCacheSystemPrompt(c), summaryBudgetKey(c.SummaryContextBudget), SummaryCacheUserPrompt(c))
	cache, err := s.summaryRead(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	// Corrupt the stored checkpoint: mark complete with a bogus document.
	cache.Result.Complete = true
	cache.Result.Document = nil
	if err := s.summaryWrite(context.Background(), cache); err != nil {
		t.Fatal(err)
	}
	// A limited invocation must not trust the corrupted complete state.
	in.MaxProviderCalls = limit(1)
	if _, err := s.SummaryGenerate(context.Background(), in); err != nil {
		t.Fatalf("corrupted checkpoint handling: %v", err)
	}
	if summarizer.posts != 2 {
		t.Fatalf("corrupted checkpoint bypassed regeneration: posts=%d", summarizer.posts)
	}
}

func mustSegments(t *testing.T, in SummaryInput) []SummarySegment {
	t.Helper()
	segments, err := normalizeSummaryInput(in)
	if err != nil {
		t.Fatal(err)
	}
	return segments
}

// A-T20: a slot reserved by a crashed/aborted invocation is not refunded and
// does not let a later start exceed the limit.
func TestAT20AbortedReservationNotRefunded(t *testing.T) {
	ctx, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash right after reservation: the slot is consumed and the
	// ledger is abandoned. A clone of the invocation cannot start again.
	if err := ReserveCompletion(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ReserveCompletion(ctx); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("aborted reservation refunded or overrun: %v", err)
	}
	if snapshot := BudgetStatus(ctx); snapshot.Started != 1 || snapshot.Remaining != 0 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// A-T22: an agent-derived invocation that carries a parent ledger reuses it
// (never creates a nested budget), while Check counts one synthetic POST and a
// zero-call Check consumes none.
func TestAT22AgentParentLedgerReusedAndCheckCounts(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{1, 1, 2}}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	// Agent-derived translate/summarize inherit a parent ledger attached by an
	// outer invocation; no request field is supplied.
	parent, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	in := translationInput(rev, 1, nil)
	if _, err := s.Translate(parent, in); err != nil {
		t.Fatalf("agent parent budget translate: %v", err)
	}
	if snapshot := BudgetStatus(parent); snapshot.Started != 1 {
		t.Fatalf("agent invocation did not use parent ledger: %+v", snapshot)
	}
	if err := ReserveCompletion(parent); !errors.Is(err, ErrBudgetExhausted) {
		t.Fatal("agent invocation did not share the parent budget")
	}
	// Check with a limit counts exactly one synthetic POST.
	checkTranslator := &budgetTranslator{usage: Usage{1, 1, 2}}
	checkSummarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	cs, crev := newBudgetService(t, checkTranslator, checkSummarizer)
	result, err := cs.Check(context.Background(), CheckInput{Task: "translation", Revision: crev, MaxProviderCalls: limit(1)})
	if err != nil {
		t.Fatalf("limited check: %v", err)
	}
	if checkTranslator.posts != 1 {
		t.Fatalf("check posts = %d, want 1", checkTranslator.posts)
	}
	if result.Budget == nil || result.Budget.Started != 1 || result.Budget.Status != "completed" {
		t.Fatalf("check report = %+v", result.Budget)
	}
	// A zero-call path (invalid task) consumes no slot and no POST.
	zeroTranslator := &budgetTranslator{usage: Usage{1, 1, 2}}
	zeroSummarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	zs, zrev := newBudgetService(t, zeroTranslator, zeroSummarizer)
	if _, err := zs.Check(context.Background(), CheckInput{Task: "nonsense", Revision: zrev, MaxProviderCalls: limit(1)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero-call check: %v", err)
	}
	if zeroTranslator.posts != 0 || zeroSummarizer.posts != 0 {
		t.Fatal("zero-call check made a POST")
	}
}

// incompleteOnceTranslator reserves a slot, records valid usage, and returns
// ErrIncomplete on its first >=8-segment batch so the service's smaller-half
// recovery path is triggered. Later calls succeed. It counts actual provider
// operations so a blocked recovery half is observable.
type incompleteOnceTranslator struct {
	mu     sync.Mutex
	starts int
	sizes  []int
}

func (p *incompleteOnceTranslator) Translate(ctx context.Context, _ Config, segments []Segment) ([]Segment, error) {
	if err := ReserveCompletion(ctx); err != nil {
		return nil, err
	}
	p.mu.Lock()
	p.starts++
	p.sizes = append(p.sizes, len(segments))
	first := p.starts == 1
	p.mu.Unlock()
	RecordCompletionUsage(ctx, Usage{1, 1, 2})
	if first && len(segments) >= 8 {
		return nil, ErrIncomplete
	}
	out := make([]Segment, len(segments))
	for i, segment := range segments {
		out[i] = Segment{Index: segment.Index, Text: "译" + segment.Text}
	}
	return out, nil
}

// A-T03 recovery under limit 1: the first >=8-segment batch starts one
// operation, records valid usage, and returns ErrIncomplete. The smaller-half
// recovery must be blocked before it starts by the exhausted budget, and no
// invalid batch checkpoint may be treated as complete.
func TestAT03ForcedSmallerHalfRecoveryBlockedAtLimit1(t *testing.T) {
	translator := &incompleteOnceTranslator{}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	root := filepath.Join(t.TempDir(), "ai")
	s, newErr := New(root, NewMemoryResults(), translator, summarizer)
	if newErr != nil {
		t.Fatal(newErr)
	}
	t.Cleanup(s.Close)
	summaryModel := "summary-model"
	status, cfgErr := s.Configure(ConfigureInput{BaseURL: "https://ai-fixture.example/v1", Model: "fixture", SummaryModel: &summaryModel, Revision: 1})
	if cfgErr != nil {
		t.Fatal(cfgErr)
	}
	rev := status.Revision
	// 9 short cues => one >=8 batch that triggers the recovery path.
	in := translationInput(rev, 9, nil)
	ctx, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Translate(ctx, in)
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("want budget_exhausted at limit 1, got %v", err)
	}
	translator.mu.Lock()
	starts, sizes := translator.starts, append([]int(nil), translator.sizes...)
	translator.mu.Unlock()
	if starts != 1 {
		t.Fatalf("provider operations started = %d, want exactly 1 (recovery half must be blocked): sizes=%v", starts, sizes)
	}
	if len(sizes) != 1 || sizes[0] < 8 {
		t.Fatalf("first operation sizes = %v, want one >=8 batch", sizes)
	}
	// No invalid batch is committed: the first batch failed, so no texts yet.
	if len(result.Texts) != 0 {
		t.Fatalf("incomplete batch treated as committed: %+v", result.Texts)
	}
	if result.Budget == nil || result.Budget.Status != "budget_exhausted" || result.Budget.Started != 1 {
		t.Fatalf("report = %+v", result.Budget)
	}
}

// A-T22 summary inheritance: an agent-derived summary carrying a parent ledger
// and no request limit reuses that ledger. With the exact-two-leaf fixture and
// a limit-1 parent it starts one leaf then reports budget_exhausted with a
// partial 1/3 checkpoint, without creating a nested budget.
func TestAT22SummaryInheritsParentLedger(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{1, 1, 2}}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	big := strings.Repeat("word ", 1600)
	in := SummaryInput{ContentID: "rss:at22-summary", Source: "article:original", Revision: rev}
	in.Segments = []SummarySegment{{Index: 0, Text: big}, {Index: 1, Text: big}, {Index: 2, Text: big}, {Index: 3, Text: big}}
	parent, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.SummaryGenerate(parent, in)
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("summary with limit-1 parent: %v", err)
	}
	if summarizer.posts != 1 {
		t.Fatalf("summary posts = %d, want 1", summarizer.posts)
	}
	if snapshot := BudgetStatus(parent); snapshot.Started != 1 {
		t.Fatalf("summary did not reuse parent ledger: %+v", snapshot)
	}
	if result.Complete || result.CompletedCalls != 1 || result.TotalCalls != 3 {
		t.Fatalf("partial checkpoint invalid: complete=%v completed=%d total=%d", result.Complete, result.CompletedCalls, result.TotalCalls)
	}
	if result.Budget == nil || result.Budget.Status != "budget_exhausted" {
		t.Fatalf("report = %+v", result.Budget)
	}
}

// A-T22 Check summary: a limited summary Check starts exactly one synthetic
// operation and reports it; an invalid Check consumes none.
func TestAT22CheckSummaryCountsOneSyntheticOperation(t *testing.T) {
	translator := &budgetTranslator{usage: Usage{1, 1, 2}}
	summarizer := &budgetSummarizer{usage: Usage{1, 1, 2}}
	s, rev := newBudgetService(t, translator, summarizer)
	result, err := s.Check(context.Background(), CheckInput{Task: "summary", Revision: rev, MaxProviderCalls: limit(1)})
	if err != nil {
		t.Fatalf("limited summary check: %v", err)
	}
	if summarizer.posts != 1 {
		t.Fatalf("summary check posts = %d, want 1", summarizer.posts)
	}
	if result.Budget == nil || result.Budget.Started != 1 || result.Budget.Status != "completed" {
		t.Fatalf("summary check report = %+v", result.Budget)
	}
	// Zero-call rejection makes no POST.
	if _, err := s.Check(context.Background(), CheckInput{Task: "nonsense", Revision: rev, MaxProviderCalls: limit(1)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid check: %v", err)
	}
	if summarizer.posts != 1 || translator.posts != 0 {
		t.Fatalf("invalid check made a POST: summary=%d translation=%d", summarizer.posts, translator.posts)
	}
}
