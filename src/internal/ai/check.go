package ai

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"
)

type CheckInput struct {
	Task     string `json:"task"`
	Revision int64  `json:"revision"`
	// MaxProviderCalls optionally caps started completion POSTs for this Check.
	MaxProviderCalls *int `json:"maxProviderCalls,omitempty"`
}

type CheckResult struct {
	Task      string `json:"task"`
	Model     string `json:"model"`
	ElapsedMS int64  `json:"elapsedMs"`
	Revision  int64  `json:"revision"`
	// Budget is present only for limited invocations.
	Budget *BudgetReport `json:"budget,omitempty"`
}

// Check uses fixed synthetic text and the same provider contract as reading
// tasks, without reading or writing any article, note or generation cache.
func (s *Service) Check(parent context.Context, in CheckInput) (CheckResult, error) {
	if !validBudgetLimit(in.MaxProviderCalls) {
		return CheckResult{}, ErrInvalid
	}
	if in.Task != "translation" && in.Task != "summary" {
		return CheckResult{}, ErrInvalid
	}
	if err := parent.Err(); err != nil {
		return CheckResult{}, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return CheckResult{}, ErrStorage
	}
	c := s.config
	if in.Revision != c.Revision {
		s.mu.Unlock()
		return CheckResult{}, ErrConflict
	}
	status := s.publicStatus(c)
	if in.Task == "translation" && (!status.Ready || s.provider == nil) || in.Task == "summary" && !status.SummaryReady {
		s.mu.Unlock()
		return CheckResult{}, ErrNotReady
	}
	if s.active != "" {
		s.mu.Unlock()
		return CheckResult{}, ErrBusy
	}
	parent, err := withBudget(parent, in.MaxProviderCalls)
	if err != nil {
		s.mu.Unlock()
		return CheckResult{}, err
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	s.active = "check:" + in.Task
	s.cancel = cancel
	s.wait.Add(1)
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		s.active = ""
		s.cancel = nil
		s.mu.Unlock()
		s.wait.Done()
	}()

	start := time.Now()
	const text = "The reader keeps your notes with the original article."
	if in.Task == "summary" {
		c = summaryConfig(c)
		draft, err := s.summarizer.Summarize(ctx, c, SummaryRequest{Mode: "leaf", Segments: []SummarySegment{{Index: 0, Text: text}}})
		if err != nil {
			SetOutcome(ctx, terminalOutcome(err))
			return CheckResult{Budget: budgetReportFor(ctx)}, err
		}
		if !validateDraft(draft, map[int]bool{0: true}) {
			SetOutcome(ctx, OutcomeProviderError)
			return CheckResult{Budget: budgetReportFor(ctx)}, ErrFormat
		}
	} else {
		output, err := s.provider.Translate(ctx, c, []Segment{{Index: 0, Text: text}})
		if err != nil {
			SetOutcome(ctx, terminalOutcome(err))
			return CheckResult{Budget: budgetReportFor(ctx)}, err
		}
		if len(output) != 1 || output[0].Index != 0 || !utf8.ValidString(output[0].Text) || strings.TrimSpace(output[0].Text) == "" || len(output[0].Text) > 24000 {
			SetOutcome(ctx, OutcomeProviderError)
			return CheckResult{Budget: budgetReportFor(ctx)}, ErrFormat
		}
	}
	if err := ctx.Err(); err != nil {
		SetOutcome(ctx, terminalOutcome(err))
		return CheckResult{Budget: budgetReportFor(ctx)}, err
	}
	SetOutcome(ctx, OutcomeCompleted)
	return CheckResult{Task: in.Task, Model: c.Model, ElapsedMS: time.Since(start).Milliseconds(), Revision: c.Revision, Budget: budgetReportFor(ctx)}, nil
}
