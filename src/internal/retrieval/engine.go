package retrieval

import (
	"context"
	"strings"
	"sync"
)

// Engine orchestrates the retrieval pipeline for the AI service: validate
// budgets → preflight → select → assemble → wire response. It returns the
// assembled six-layer context (for the provider system message) and the
// namespaced response. A preflight- or assembly-detectable failure returns a
// StatusError and starts zero chat requests (the caller never chats on error).
type Engine struct {
	Source    PassageSource
	Embedder  Embedder // nil → lexical-only
	Assembler ContextAssembler
	mu        sync.Mutex // guards Embedder swaps (settings save)
}

// SetEmbedder replaces the embedding adapter at runtime (saved settings take
// effect without a restart). A nil embedder falls back to lexical-only. Each
// Run snapshots the current embedder, so an in-flight run keeps its adapter.
func (e *Engine) SetEmbedder(emb Embedder) {
	e.mu.Lock()
	e.Embedder = emb
	e.mu.Unlock()
}

// RunInput feeds one retrieval operation.
type RunInput struct {
	ContentID       string
	Question        string
	Request         Request
	History         []HistoryTurn
	SummaryMaterial string
	ResumableState  string
}

// RunResult carries the wire response and the assembled context.
type RunResult struct {
	Response Response
	Context  string
}

// budgetedEmbedder counts requests and enforces B's budget around an inner
// embedder (each Embed call is one request under mode-aware batching).
type budgetedEmbedder struct {
	inner   Embedder
	tracker *BudgetTracker
}

func (b *budgetedEmbedder) ModelID() string            { return b.inner.ModelID() }
func (b *budgetedEmbedder) Dimensions() int            { return b.inner.Dimensions() }
func (b *budgetedEmbedder) BatchingMode() BatchingMode { return b.inner.BatchingMode() }
func (b *budgetedEmbedder) Embed(ctx context.Context, texts []string, role EmbedRole) ([][]float32, error) {
	if !b.tracker.Reserve() {
		return nil, StatusError{Status: string(SelectionEmbeddingBudgetExhaust)}
	}
	chars := 0
	for _, t := range texts {
		chars += RuneLen(t)
	}
	b.tracker.AddChars(chars)
	// Adapters that report usage validity mark this invocation's tracker
	// directly (missing usage stops later batches); the slot above is the
	// only reservation, so no double counting occurs.
	if aware, ok := b.inner.(UsageReportingEmbedder); ok {
		return aware.EmbedWithUsage(ctx, texts, role, b.tracker)
	}
	return b.inner.Embed(ctx, texts, role)
}

// Run executes the pipeline. On rejection the Response is still populated
// (status + any spent embedding counts) and a StatusError is returned so the
// caller starts zero chat requests.
func (e *Engine) Run(ctx context.Context, in RunInput) (RunResult, error) {
	e.mu.Lock()
	embedder := e.Embedder
	e.mu.Unlock()
	asm := e.Assembler
	if asm == nil {
		asm = NewContextAssembler()
	}
	maxChars, err := validateRequest(in.Request)
	if err != nil {
		return RunResult{Response: Response{SchemaVersion: WireSchemaVersion, Status: string(SelectionInvalidBudget)}}, err
	}
	// Preflight before any selector/embedding work (INV-04).
	if perr := Preflight(maxChars, DefaultMaxBytes); perr != nil {
		status := string(SelectionContextBudgetExceeded)
		if perr == ErrInvalidBudget {
			status = string(SelectionInvalidBudget)
		}
		return RunResult{Response: Response{SchemaVersion: WireSchemaVersion, Status: status}}, StatusError{Status: status}
	}

	// Build the embedding budget + mode from the request (defaults applied).
	mode := in.Request.Mode
	if mode == "" {
		mode = ModeHybrid
	}
	batchMode := BatchingShared
	if in.Request.EmbeddingBudget.BatchingMode == string(BatchingSplit) {
		batchMode = BatchingSplit
	} else if embedder != nil {
		batchMode = embedder.BatchingMode()
	}
	// An omitted maxRequests uses the mode default; an explicit value is
	// preserved as-is, including 0, which turns embedding off (budget
	// contract: zero = off, never silently replaced by the default).
	maxRequests := 24
	if batchMode == BatchingSplit {
		maxRequests = 25
	}
	if in.Request.EmbeddingBudget.MaxRequests != nil {
		maxRequests = *in.Request.EmbeddingBudget.MaxRequests
	}
	embChars := in.Request.EmbeddingBudget.MaxChars
	if embChars == 0 {
		embChars = 100000
	}
	sweepCandidates := in.Request.EmbeddingBudget.SweepCandidates
	if sweepCandidates == 0 && in.Request.EmbeddingBudget.MaxRequests != nil {
		sweepCandidates = 90
	}
	topK := 100

	var emb Embedder
	var tracker *BudgetTracker
	if embedder != nil && mode != ModeLexical {
		tracker = NewBudgetTracker(maxRequests, embChars)
		emb = &budgetedEmbedder{inner: embedder, tracker: tracker}
	}

	cfg := SelectorConfig{
		TopK:                topK,
		Sweep:               SweepConfig{Enabled: embedder != nil, SweepCandidates: sweepCandidates},
		Budget:              BudgetConfig{Mode: batchMode, MaxRequests: maxRequests, BatchSize: 8, TopK: topK, SweepCandidates: sweepCandidates, MaxEmbeddingChars: embChars},
		Weights:             DefaultWeights(),
		AssemblyBudgetRunes: maxChars,
		MaxBytes:            DefaultMaxBytes,
		Mode:                mode,
	}
	sel := NewSelector(e.Source, emb, cfg)
	selection, serr := sel.Select(ctx, in.ContentID, in.Question, 0)
	if serr != nil {
		return RunResult{}, serr
	}

	resp := Response{SchemaVersion: WireSchemaVersion, Status: string(selection.Status), Mode: selection.Mode}
	resp.Embedding = embeddingSummary(embedder, tracker)
	resp.Rerank = RerankSummary{Enabled: false, BudgetStatus: BudgetDisabled}

	// Terminal selection statuses start zero chat requests.
	switch selection.Status {
	case SelectionInvalidBudget, SelectionContextBudgetExceeded, SelectionNoVerifiedPassages,
		SelectionStaleSource, SelectionInvalidInput, SelectionEmpty, SelectionNoMatch,
		SelectionPDFNoText, SelectionPDFExtracting, SelectionPDFExtractFail, SelectionPDFOCRUnavailable:
		return RunResult{Response: resp}, StatusError{Status: string(selection.Status)}
	}

	// Assemble the six-layer context (current task = the question).
	assembly, aerr := asm.Assemble(ctx, AssemblyInput{
		Selection:       selection,
		SummaryMaterial: in.SummaryMaterial,
		History:         in.History,
		ResumableState:  in.ResumableState,
		CurrentTask:     in.Question,
		BudgetRunes:     maxChars,
		MaxBytes:        DefaultMaxBytes,
	})
	if aerr != nil {
		return RunResult{}, aerr
	}
	resp.Passages = make([]PassageRef, 0, len(assembly.Passages))
	for _, p := range assembly.Passages {
		resp.Passages = append(resp.Passages, RefFromPassage(p))
	}
	if assembly.Status == AssemblyContextBudgetExceeded || assembly.Status == AssemblyInvalidBudget {
		resp.Status = assembly.Status
		return RunResult{Response: resp}, StatusError{Status: assembly.Status}
	}
	if assembly.Status == AssemblyDegradedBudget && resp.Status == string(SelectionOK) {
		resp.Status = AssemblyDegradedBudget
	}
	return RunResult{Response: resp, Context: assembly.Context}, nil
}

func embeddingSummary(emb Embedder, tracker *BudgetTracker) EmbeddingSummary {
	if emb == nil {
		return EmbeddingSummary{BudgetStatus: BudgetDisabled}
	}
	s := EmbeddingSummary{Model: emb.ModelID(), Dimensions: emb.Dimensions()}
	if tracker != nil {
		s.BudgetStatus = tracker.Status()
		s.Requests = tracker.Requests()
	} else {
		s.BudgetStatus = BudgetCompleted
	}
	return s
}

// LastUserQuestion extracts the current question (last user message).
func LastUserQuestion(messages []HistoryTurn) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return strings.TrimSpace(messages[i].Content)
		}
	}
	return ""
}
