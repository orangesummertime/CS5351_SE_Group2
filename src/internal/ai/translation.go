package ai

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/longbridgeapp/opencc"
)

func New(root string, results ResultStore, provider Translator, summarizers ...Summarizer) (*Service, error) {
	if results == nil {
		return nil, ErrStorage
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, ErrStorage
	}
	c := Config{Protocol: "openai", BaseURL: "https://api.openai.com/v1", Revision: 1}
	path := filepath.Join(root, "settings.json")
	data, err := os.ReadFile(path)
	if err == nil {
		if len(data) > 128<<10 || json.Unmarshal(data, &c) != nil || !validConfig(c) {
			return nil, ErrStorage
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, ErrStorage
	}
	if c.Protocol == "" {
		c.Protocol = "openai"
	}
	if likelyTranslationModel(c.ChatModel) {
		// Older settings used the first enabled model (often a translation-only
		// model) as the implicit chat model. Clear that implicit choice instead of
		// silently selecting another model.
		c.ChatModel = ""
	}
	if len(c.EnabledModels) == 0 && c.Model != "" {
		c.EnabledModels = []string{c.Model}
		c.AvailableModels = []string{c.Model}
	}
	if !validConfig(c) {
		return nil, ErrStorage
	}
	var summarizer Summarizer
	if len(summarizers) > 0 {
		summarizer = summarizers[0]
	} else if value, ok := provider.(Summarizer); ok {
		summarizer = value
	}
	importLegacyCache(root, results)
	return &Service{config: c, settingsPath: path, results: results, summaries: newSummaryStore(results), provider: provider, summarizer: summarizer}, nil
}
func (s *Service) Inspect(ctx context.Context, in Input) (Inspection, error) {
	if !validate(in) {
		return Inspection{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Inspection{}, err
	}
	s.mu.Lock()
	c, active := s.config, s.active
	s.mu.Unlock()
	if subtitleAlreadyTarget(in) {
		status := s.publicStatus(c)
		status.Translating = active != ""
		return Inspection{Settings: status, Result: originalSubtitleResult(in, c)}, nil
	}
	expected := identity(in, c)
	result, err := s.load(ctx, in)
	if errors.Is(err, ErrNotFound) {
		result = expected
	} else if err != nil {
		return Inspection{}, err
	}
	if result.Total != expected.Total {
		return Inspection{}, ErrStorage
	}
	status := s.publicStatus(c)
	status.Translating = active != ""
	return Inspection{status, result, active == expected.Key}, nil
}
func (s *Service) Cached(ctx context.Context, key string) (Result, error) {
	if !keyPattern.MatchString(key) {
		return Result{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	result, err := s.read(ctx, kindTranslation, key)
	if errors.Is(err, ErrNotFound) {
		result, err = s.read(ctx, kindTitle, key)
	}
	return result, err
}
func (s *Service) Translate(parent context.Context, in Input) (result Result, retErr error) {
	if !validBudgetLimit(in.MaxProviderCalls) {
		return Result{}, ErrInvalid
	}
	if !validate(in) {
		return Result{}, ErrInvalid
	}
	if err := parent.Err(); err != nil {
		return Result{}, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return Result{}, ErrStorage
	}
	c := s.config
	c.TranslationTarget = inputTargetLanguage(in)
	if in.Revision != c.Revision {
		s.mu.Unlock()
		return Result{}, ErrConflict
	}
	if subtitleAlreadyTarget(in) {
		s.mu.Unlock()
		return originalSubtitleResult(in, c), nil
	}
	if !public(c).Ready || s.provider == nil {
		s.mu.Unlock()
		return Result{}, ErrNotReady
	}
	if s.active != "" {
		s.mu.Unlock()
		return Result{}, ErrBusy
	}
	result = identity(in, c)
	parent, err := withBudget(parent, in.MaxProviderCalls)
	if err != nil {
		s.mu.Unlock()
		return Result{}, err
	}
	ctx, cancel := context.WithTimeout(parent, 45*time.Minute)
	s.active = result.Key
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
		if retErr != nil {
			SetOutcome(ctx, terminalOutcome(retErr))
		}
		if BudgetAttached(ctx) {
			// Attach the report to any returned partial result so the HTTP layer
			// can render limited progress and counters without losing the ledger
			// state, including on error.
			result.Budget = budgetReportFor(ctx)
		}
	}()
	kind := translationKind(in)
	if in.Regenerate {
		if err := s.results.DeleteAIResult(ctx, kind, result.Key, targetChinese); err != nil {
			return Result{}, ErrStorage
		}
	} else {
		cached, err := s.load(ctx, in)
		if err == nil {
			if cached.Total != result.Total {
				return Result{}, ErrStorage
			}
			result = cached
		} else if !errors.Is(err, ErrNotFound) {
			return Result{}, err
		}
		if len(result.Texts) == result.Total {
			SetOutcome(ctx, OutcomeCompleted)
			result.Budget = budgetReportFor(ctx)
			return result, nil
		}
	}
	// Remaining batches use the current model; the row records the latest one.
	result.BaseURL, result.Model = c.BaseURL, c.Model
	// Creating the stored row is part of the explicit operation, never a read.
	if err := s.write(ctx, kind, result); err != nil {
		return Result{}, err
	}
	var converter *opencc.OpenCC
	if result.TargetLanguage == targetChinese {
		var err error
		converter, err = opencc.New("t2s")
		if err != nil {
			return Result{}, ErrFormat
		}
	}
	// One smaller-batch recovery per explicit operation. Successful halves use
	// the same prefix cache, so cancellation never discards committed progress.
	recoveryLimit, recoveryEnd := 0, 0
	for len(result.Texts) < result.Total {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		start := len(result.Texts)
		end, size := start, 0
		countLimit, byteLimit := batchCount, batchBytes
		if in.Source == "article" {
			countLimit, byteLimit = 8, articleBatchBytes
		}
		if provider, ok := s.provider.(interface{ TranslationBatchLimit(Config) int }); ok {
			if limit := provider.TranslationBatchLimit(c); limit > 0 && limit < countLimit {
				countLimit = limit
			}
		}
		if start < recoveryEnd {
			countLimit = min(countLimit, recoveryLimit, recoveryEnd-start)
		}
		for end < result.Total && end-start < countLimit && (end == start || size+len(in.Track.Cues[end].Text) <= byteLimit) {
			size += len(in.Track.Cues[end].Text)
			end++
		}
		segments := make([]Segment, 0, end-start)
		for i := start; i < end; i++ {
			segments = append(segments, Segment{Index: i, Text: in.Track.Cues[i].Text})
		}
		outputs, err := s.translateBatch(ctx, c, in, segments)
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err != nil {
			if recoveryLimit == 0 && len(segments) >= 8 && (errors.Is(err, ErrIncomplete) || errors.Is(err, ErrTruncated)) {
				recoveryLimit = (len(segments) + 1) / 2
				recoveryEnd = end
				continue
			}
			return result, err
		}
		if len(outputs) != len(segments) {
			return result, ErrFormat
		}
		texts := make([]string, len(segments))
		seen := make(map[int]bool, len(segments))
		for _, out := range outputs {
			if out.Index < start || out.Index >= end || seen[out.Index] || !utf8.ValidString(out.Text) || len(out.Text) > 24000 || strings.TrimSpace(out.Text) == "" {
				return result, ErrFormat
			}
			converted := strings.TrimSpace(out.Text)
			if converter != nil {
				converted, err = converter.Convert(converted)
				if err != nil {
					return result, ErrFormat
				}
			}
			if len(converted) > 24000 {
				return result, ErrFormat
			}
			seen[out.Index] = true
			texts[out.Index-start] = converted
		}
		candidate := result
		candidate.Texts = append(append([]string{}, result.Texts...), texts...)
		candidate.UpdatedAt = time.Now().Unix()
		total := 0
		for _, text := range candidate.Texts {
			total += len(text)
		}
		if total > 8<<20 {
			return result, ErrFormat
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if err := s.write(ctx, kind, candidate); err != nil {
			return result, err
		}
		result = candidate
	}
	SetOutcome(ctx, OutcomeCompleted)
	result.Budget = budgetReportFor(ctx)
	return result, nil
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.wait.Wait()
}
