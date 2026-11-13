package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/longbridgeapp/opencc"
)

func summarySegmentsText(segments []SummarySegment) string {
	var builder strings.Builder
	for _, segment := range segments {
		fmt.Fprintf(&builder, "[%d] %s\n", segment.Index, segment.Text)
	}
	return builder.String()
}

func summaryCallStarted(cache *summaryCache, phase string, save func(summaryCache) error) error {
	cache.Result.Progress.CurrentCall = cache.Result.CompletedCalls + 1
	cache.Result.Progress.Phase = phase
	cache.Result.Progress.StartedAt = time.Now().Unix()
	cache.Result.Progress.Error = ""
	return save(*cache)
}

func summaryCallFinished(cache *summaryCache) {
	cache.Result.Progress.CurrentCall = 0
	cache.Result.Progress.Phase = "checkpoint"
	cache.Result.Progress.StartedAt = 0
	cache.Result.Progress.Error = ""
}

func summaryCallFailed(cache *summaryCache, save func(summaryCache) error, callErr error) error {
	cache.Result.Progress.CurrentCall = 0
	cache.Result.Progress.StartedAt = 0
	phase := cache.Result.Progress.Phase
	cache.Result.Progress.Phase = "error"
	if phase == "merge" && cache.Result.CompletedCalls+1 >= cache.Result.Progress.TotalCalls {
		cache.Result.Progress.Error = "最后合并请求超时，重试将使用已完成章节继续收尾；已完成内容已保留。"
	} else {
		cache.Result.Progress.Error = "模型请求超时或失败，请重试；已完成内容已保留。"
	}
	if err := save(*cache); err != nil {
		return ErrStorage
	}
	return callErr
}

// mergeSummaryDraftsLocally is the recovery path for a final reduction that
// timed out after all leaf/chapter drafts were already validated. It only
// combines model-produced text and source indices; it never invents evidence
// or asks the provider for another slow completion.
func mergeSummaryDraftsLocally(drafts []SummaryDraft) SummaryDraft {
	var overviews []string
	points := make([]SummaryPoint, 0, 5)
	outline := make([]SummaryOutline, 0, 16)
	for _, draft := range drafts {
		if text := cleanSummaryDraftPrefix(draft.Overview); text != "" {
			overviews = append(overviews, text)
		}
		for _, point := range draft.Points {
			if len(points) >= 5 {
				break
			}
			point.Text = cleanSummaryDraftPrefix(point.Text)
			points = append(points, point)
		}
		outline = append(outline, draft.Outline...)
	}
	overview := strings.TrimSpace(strings.Join(overviews, "；"))
	if len(overview) > 1200 {
		runes := []rune(overview)
		if len(runes) > 1200 {
			overview = string(runes[:1200])
		}
	}
	return SummaryDraft{Overview: overview, Points: points, Outline: outline}
}

func cleanSummaryDraftPrefix(value string) string {
	value = strings.TrimSpace(value)
	for _, prefix := range []string{"草稿汇总称，", "草稿称，", "草稿指出，", "草稿记录，"} {
		value = strings.TrimPrefix(value, prefix)
	}
	return strings.TrimSpace(value)
}

// SummaryGenerate coordinates one resumable summary operation. Cache encoding
// and migration stay behind summaryStore; this file only owns provider calls,
// checkpoints and reduction order.
func (s *Service) SummaryGenerate(parent context.Context, in SummaryInput) (result SummaryResult, retErr error) {
	if !validBudgetLimit(in.MaxProviderCalls) {
		return SummaryResult{}, ErrInvalid
	}
	segments, err := normalizeSummaryInput(in)
	if err != nil {
		return SummaryResult{}, err
	}
	if err = parent.Err(); err != nil {
		return SummaryResult{}, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return SummaryResult{}, ErrStorage
	}
	c, provider := s.config, s.summarizer
	if in.Revision != c.Revision {
		s.mu.Unlock()
		return SummaryResult{}, ErrConflict
	}
	if !s.publicStatus(c).SummaryReady || provider == nil {
		s.mu.Unlock()
		return SummaryResult{}, ErrNotReady
	}
	if s.active != "" {
		s.mu.Unlock()
		return SummaryResult{}, ErrBusy
	}
	c = summaryConfig(c)
	key := summaryKey(in, segments, SummaryCacheSystemPrompt(c), summaryBudgetKey(c.SummaryContextBudget), SummaryCacheUserPrompt(c))
	groups, totalCalls := summaryPlan(segments, summaryContextBudget(c.SummaryContextBudget))
	parent, err = withBudget(parent, in.MaxProviderCalls)
	if err != nil {
		s.mu.Unlock()
		return SummaryResult{}, err
	}
	ctx, cancel := context.WithTimeout(parent, 45*time.Minute)
	s.active = "summary:" + key
	s.cancel = cancel
	s.wait.Add(1)
	s.mu.Unlock()
	sectionCount := summarySectionCount(segments, in.Source, groups)
	fresh := summaryCache{Result: SummaryResult{Key: key, ContentID: in.ContentID, Source: in.Source, BaseURL: c.BaseURL, Model: c.Model, TotalCalls: totalCalls, FormatVersion: summaryFormatVersion, Progress: SummaryProgress{TotalCalls: totalCalls, TotalSections: sectionCount}}, Current: []SummaryDraft{}}
	cache := fresh
	defer func() {
		cancel()
		s.mu.Lock()
		s.active = ""
		s.cancel = nil
		s.mu.Unlock()
		s.wait.Done()
		if retErr != nil {
			// Classify the terminal outcome BEFORE snapshotting the returned
			// budget in the same LIFO frame. A provider failure under any limit
			// must report provider_error, not active or budget_exhausted, and
			// never a fake known-zero token count. The partial checkpoint stays.
			SetOutcome(ctx, terminalOutcome(retErr))
			if BudgetAttached(ctx) {
				result = cache.Result
				result.Budget = budgetReportFor(ctx)
			}
		}
	}()
	if in.Regenerate {
		if err := s.results.DeleteAIResult(ctx, kindSummary, key, targetChinese); err != nil {
			return SummaryResult{}, ErrStorage
		}
	} else if cache, err = s.summaryLoad(ctx, key, in, segments, c); errors.Is(err, ErrNotFound) {
		cache = fresh
	} else if err != nil {
		return SummaryResult{}, err
	}
	if cache.Result.TotalCalls != totalCalls || !validCachedSummary(cache.Result, segments) {
		return SummaryResult{}, ErrStorage
	}
	if cache.Result.Complete && (cache.Result.FormatVersion != summaryFormatVersion || cache.Result.Document == nil) {
		cache.Result.Complete = false
		cache.Result.CompletedCalls = 0
		cache.Result.Overview = ""
		cache.Result.Points = nil
		cache.Result.Citations = nil
		cache.Result.Outline = nil
		cache.Result.Document = nil
		cache.Result.FormatVersion = summaryFormatVersion
		cache.Current = nil
		cache.Next = nil
	}
	if cache.Result.Complete {
		normalizeSummaryDocumentLevels(cache.Result.Document)
		if err := s.applySummaryTitleOverrides(ctx, &cache.Result); err != nil {
			return SummaryResult{}, err
		}
		SetOutcome(ctx, OutcomeCompleted)
		cache.Result.Budget = budgetReportFor(ctx)
		return cache.Result, nil
	}
	cache.Result.BaseURL, cache.Result.Model = c.BaseURL, c.Model
	cache.Result.FormatVersion = summaryFormatVersion
	cache.Result.Progress.TotalCalls = totalCalls
	cache.Result.Progress.TotalSections = sectionCount
	save := func(value summaryCache) error { return s.summaryWrite(ctx, value) }
	converter, err := opencc.New("t2s")
	if err != nil {
		return SummaryResult{}, ErrFormat
	}
	allowedOriginal := make(map[int]bool, len(segments))
	for _, segment := range segments {
		allowedOriginal[segment.Index] = true
	}
	if err := validateSummaryCheckpoint(cache, groups, allowedOriginal); err != nil {
		return SummaryResult{}, err
	}
	documentCapable := false
	if capability, ok := provider.(DocumentSummarizer); ok {
		documentCapable = capability.SupportsSummaryDocument(c)
	}
	if documentCapable && len(groups) == 1 && cache.Result.CompletedCalls == 0 && len(cache.Current) == 0 && summaryDocumentEligible(segments) {
		if err := s.generateSummaryDocument(ctx, provider, c, converter, &cache, segments, allowedOriginal, save); err != nil {
			return SummaryResult{}, err
		}
		if cache.Result.Complete {
			if err := s.applySummaryTitleOverrides(ctx, &cache.Result); err != nil {
				return SummaryResult{}, err
			}
			s.mu.Lock()
			writer := s.saveSummaryNote
			s.mu.Unlock()
			if writer != nil {
				_ = writer(ctx, cache.Result)
			}
			SetOutcome(ctx, OutcomeCompleted)
			cache.Result.Budget = budgetReportFor(ctx)
			return cache.Result, nil
		}
	}
	if err := s.generateSummaryLeaves(ctx, provider, c, converter, &cache, groups, save); err != nil {
		return SummaryResult{}, err
	}
	if err := s.reduceSummaryDrafts(ctx, provider, c, converter, &cache, segments, save); err != nil {
		return SummaryResult{}, err
	}
	if len(cache.Current) != 1 {
		return SummaryResult{}, ErrFormat
	}
	final := cache.Current[0]
	cache.Result.Complete = true
	cache.Result.Progress.CompletedCalls = cache.Result.CompletedCalls
	cache.Result.Progress.Complete = true
	cache.Result.Overview = final.Overview
	cache.Result.Points = final.Points
	cache.Result.Citations = buildCitations(final, segments)
	cache.Result.Outline = normalizeOutline(final.Outline, segments)
	if len(cache.Result.Outline) == 0 {
		// Some otherwise valid models omit the optional field. Turn the concrete
		// points into a useful outline rather than rendering an empty panel.
		cache.Result.Outline = make([]SummaryOutline, 0, len(final.Points))
		for _, point := range final.Points {
			cache.Result.Outline = append(cache.Result.Outline, SummaryOutline{Title: point.Text, Level: 0, Indices: append([]int(nil), point.Indices...)})
		}
		cache.Result.Outline = normalizeOutline(cache.Result.Outline, segments)
	}
	cache.Result.Document = buildSummaryDocument(cache.Result, final, segments)
	normalizeSummaryDocumentLevels(cache.Result.Document)
	if cache.Result.Document != nil {
		cache.Result.Progress.CompletedSections = len(cache.Result.Document.Sections)
		cache.Result.Progress.TotalSections = len(cache.Result.Document.Sections)
	}
	if err := save(cache); err != nil {
		return SummaryResult{}, ErrStorage
	}
	if err := s.applySummaryTitleOverrides(ctx, &cache.Result); err != nil {
		return SummaryResult{}, err
	}
	s.mu.Lock()
	writer := s.saveSummaryNote
	s.mu.Unlock()
	if writer != nil {
		_ = writer(ctx, cache.Result)
	}
	SetOutcome(ctx, OutcomeCompleted)
	cache.Result.Budget = budgetReportFor(ctx)
	return cache.Result, nil
}

func summaryDocumentEligible(segments []SummarySegment) bool {
	if len(segments) == 0 || len(segments) > summaryLeafCount {
		return false
	}
	bytes := 0
	for _, segment := range segments {
		bytes += len(segment.Text)
	}
	return bytes <= summaryLeafBytes
}

func (s *Service) generateSummaryDocument(ctx context.Context, provider Summarizer, config Config, converter *opencc.OpenCC, cache *summaryCache, segments []SummarySegment, allowed map[int]bool, save func(summaryCache) error) error {
	if err := summaryCallStarted(cache, "document", save); err != nil {
		return ErrStorage
	}
	batchCtx, batchCancel := context.WithTimeout(ctx, 90*time.Second)
	templates := promptTemplates(config)
	userPrompt, promptErr := RenderPrompt(templates.Summary.User, map[string]string{"title": "", "source_type": "summary document", "content": summarySegmentsText(segments), "target_language": "简体中文"})
	if promptErr != nil {
		batchCancel()
		_ = summaryCallFailed(cache, save, promptErr)
		return ErrFormat
	}
	draft, callErr := provider.Summarize(batchCtx, config, SummaryRequest{Mode: "document", Prompt: templates.Summary.System, UserPrompt: userPrompt, Segments: segments})
	batchCancel()
	if ctx.Err() != nil {
		return summaryCallFailed(cache, save, ctx.Err())
	}
	if callErr != nil {
		return summaryCallFailed(cache, save, callErr)
	}
	if !validateDraft(draft, allowed) {
		_ = summaryCallFailed(cache, save, ErrFormat)
		return ErrFormat
	}
	if err := convertSummaryDraft(converter, &draft); err != nil {
		_ = summaryCallFailed(cache, save, err)
		return err
	}
	result := cache.Result
	result.Overview = draft.Overview
	result.Points = draft.Points
	result.Citations = buildCitations(draft, segments)
	result.Outline = normalizeOutline(draft.Outline, segments)
	if len(result.Outline) == 0 {
		for _, point := range draft.Points {
			result.Outline = append(result.Outline, SummaryOutline{Title: point.Text, Level: 0, Indices: append([]int(nil), point.Indices...)})
		}
		result.Outline = normalizeOutline(result.Outline, segments)
	}
	result.Document = buildSummaryDocument(result, draft, segments)
	if result.Document == nil || !validSummaryDocument(result.Document, segments) {
		_ = summaryCallFailed(cache, save, ErrFormat)
		return ErrFormat
	}
	result.FormatVersion = summaryFormatVersion
	result.Complete = true
	result.CompletedCalls = 1
	result.Progress.CompletedCalls = 1
	result.Progress.TotalCalls = 1
	result.Progress.Complete = true
	result.Progress.CompletedSections = len(result.Document.Sections)
	result.Progress.TotalSections = len(result.Document.Sections)
	cache.Result = result
	summaryCallFinished(cache)
	if err := save(*cache); err != nil {
		return ErrStorage
	}
	return nil
}

func validateSummaryCheckpoint(cache summaryCache, groups [][]SummarySegment, allowedOriginal map[int]bool) error {
	for _, draft := range append(append([]SummaryDraft{}, cache.Current...), cache.Next...) {
		if !validateDraft(draft, allowedOriginal) {
			return ErrStorage
		}
	}
	if cache.Result.CompletedCalls <= len(groups) {
		if len(cache.Current) != cache.Result.CompletedCalls || len(cache.Next) != 0 {
			return ErrStorage
		}
		for i, draft := range cache.Current {
			allowed := make(map[int]bool)
			for _, segment := range groups[i] {
				allowed[segment.Index] = true
			}
			if !validateDraft(draft, allowed) {
				return ErrStorage
			}
		}
	}
	for i, draft := range cache.Next {
		start := i * summaryReduceCount
		end := start + summaryReduceCount
		if start >= len(cache.Current) {
			return ErrStorage
		}
		if end > len(cache.Current) {
			end = len(cache.Current)
		}
		allowed := make(map[int]bool)
		for _, previous := range cache.Current[start:end] {
			for _, point := range previous.Points {
				for _, index := range point.Indices {
					allowed[index] = true
				}
			}
			collectOutlineIndices(previous.Outline, allowed)
		}
		if !validateDraft(draft, allowed) {
			return ErrStorage
		}
	}
	if cache.Result.CompletedCalls >= len(groups) {
		count, completed := len(groups), len(groups)
		for count > len(cache.Current) && count > 1 {
			count = (count + summaryReduceCount - 1) / summaryReduceCount
			completed += count
		}
		if count != len(cache.Current) || cache.Result.CompletedCalls != completed+len(cache.Next) {
			return ErrStorage
		}
	}
	return nil
}

func (s *Service) generateSummaryLeaves(ctx context.Context, provider Summarizer, config Config, converter *opencc.OpenCC, cache *summaryCache, groups [][]SummarySegment, save func(summaryCache) error) error {
	for cache.Result.CompletedCalls < len(groups) {
		if len(cache.Current) != cache.Result.CompletedCalls || len(cache.Next) != 0 {
			return ErrStorage
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		group := groups[len(cache.Current)]
		allowed := map[int]bool{}
		for _, segment := range group {
			allowed[segment.Index] = true
		}
		batchCtx, batchCancel := context.WithTimeout(ctx, 90*time.Second)
		if err := summaryCallStarted(cache, "chapter", save); err != nil {
			batchCancel()
			return ErrStorage
		}
		templates := promptTemplates(config)
		userPrompt, promptErr := RenderPrompt(templates.Summary.User, map[string]string{"title": "", "source_type": "summary leaf", "content": summarySegmentsText(group), "target_language": "简体中文"})
		if promptErr != nil {
			batchCancel()
			_ = summaryCallFailed(cache, save, promptErr)
			return ErrFormat
		}
		draft, callErr := provider.Summarize(batchCtx, config, SummaryRequest{Mode: "leaf", Prompt: templates.Summary.System, UserPrompt: userPrompt, Segments: group})
		batchCancel()
		if ctx.Err() != nil {
			return summaryCallFailed(cache, save, ctx.Err())
		}
		if callErr != nil {
			return summaryCallFailed(cache, save, callErr)
		}
		if !validateDraft(draft, allowed) {
			_ = summaryCallFailed(cache, save, ErrFormat)
			return ErrFormat
		}
		if err := convertSummaryDraft(converter, &draft); err != nil {
			_ = summaryCallFailed(cache, save, err)
			return err
		}
		cache.Current = append(cache.Current, draft)
		cache.Result.CompletedCalls++
		cache.Result.Progress.CompletedCalls = cache.Result.CompletedCalls
		cache.Result.Progress.CompletedSections = minInt(len(cache.Current), cache.Result.Progress.TotalSections)
		summaryCallFinished(cache)
		if err := save(*cache); err != nil {
			return ErrStorage
		}
	}
	return nil
}

func (s *Service) reduceSummaryDrafts(ctx context.Context, provider Summarizer, config Config, converter *opencc.OpenCC, cache *summaryCache, segments []SummarySegment, save func(summaryCache) error) error {
	for len(cache.Current) > 1 {
		next := cache.Next
		if len(next) > (len(cache.Current)+summaryReduceCount-1)/summaryReduceCount {
			return ErrStorage
		}
		for start := len(next) * summaryReduceCount; start < len(cache.Current); start += summaryReduceCount {
			end := start + summaryReduceCount
			if end > len(cache.Current) {
				end = len(cache.Current)
			}
			drafts := cache.Current[start:end]
			allowed := map[int]bool{}
			for _, d := range drafts {
				for _, p := range d.Points {
					for _, index := range p.Indices {
						allowed[index] = true
					}
				}
				collectOutlineIndices(d.Outline, allowed)
			}
			// If the final provider reduction already failed, resume locally on
			// the next attempt instead of issuing the same request again.
			if cache.Result.Progress.Phase == "error" && len(cache.Current) <= summaryReduceCount && len(next) == 0 {
				draft := mergeSummaryDraftsLocally(drafts)
				if !validateDraft(draft, allowed) {
					return ErrFormat
				}
				next = append(next, draft)
				cache.Result.CompletedCalls++
				cache.Result.Progress.CompletedCalls = cache.Result.CompletedCalls
				cache.Next = next
				summaryCallFinished(cache)
				if err := save(*cache); err != nil {
					return ErrStorage
				}
				continue
			}
			batchCtx, batchCancel := context.WithTimeout(ctx, 90*time.Second)
			if err := summaryCallStarted(cache, "merge", save); err != nil {
				batchCancel()
				return ErrStorage
			}
			templates := promptTemplates(config)
			userPrompt, promptErr := RenderPrompt(templates.Summary.User, map[string]string{"title": "", "source_type": "summary reduction", "content": "", "target_language": "简体中文"})
			if promptErr != nil {
				batchCancel()
				_ = summaryCallFailed(cache, save, promptErr)
				return ErrFormat
			}
			draft, callErr := provider.Summarize(batchCtx, config, SummaryRequest{Mode: "reduce", Prompt: templates.Summary.System, UserPrompt: userPrompt, Drafts: drafts})
			batchCancel()
			if ctx.Err() != nil {
				return summaryCallFailed(cache, save, ctx.Err())
			}
			if callErr != nil {
				return summaryCallFailed(cache, save, callErr)
			}
			if !validateDraft(draft, allowed) {
				_ = summaryCallFailed(cache, save, ErrFormat)
				return ErrFormat
			}
			if err := convertSummaryDraft(converter, &draft); err != nil {
				_ = summaryCallFailed(cache, save, err)
				return err
			}
			next = append(next, draft)
			cache.Result.CompletedCalls++
			cache.Result.Progress.CompletedCalls = cache.Result.CompletedCalls
			cache.Next = next
			summaryCallFinished(cache)
			if err := save(*cache); err != nil {
				return ErrStorage
			}
		}
		cache.Current = next
		cache.Next = nil
		if err := save(*cache); err != nil {
			return ErrStorage
		}
	}
	return nil
}

func collectOutlineIndices(nodes []SummaryOutline, allowed map[int]bool) {
	for _, node := range nodes {
		for _, index := range node.Indices {
			allowed[index] = true
		}
		collectOutlineIndices(node.Children, allowed)
	}
}

func convertSummaryDraft(converter *opencc.OpenCC, draft *SummaryDraft) error {
	var err error
	draft.Overview, err = converter.Convert(strings.TrimSpace(draft.Overview))
	if err != nil {
		return ErrFormat
	}
	for i := range draft.Points {
		draft.Points[i].Text, _ = converter.Convert(strings.TrimSpace(draft.Points[i].Text))
	}
	if draft.Document != nil {
		if err := convertSummaryDocumentAI(converter, draft.Document); err != nil {
			return err
		}
	}
	return nil
}

func convertSummaryDocumentAI(converter *opencc.OpenCC, document *SummaryDocument) error {
	if document == nil {
		return nil
	}
	var convert = func(value *string) error {
		converted, err := converter.Convert(strings.TrimSpace(*value))
		if err != nil {
			return ErrFormat
		}
		*value = converted
		return nil
	}
	if err := convert(&document.Topic); err != nil {
		return err
	}
	var walk func([]SummarySection) error
	walk = func(sections []SummarySection) error {
		for index := range sections {
			if err := convert(&sections[index].Title); err != nil {
				return err
			}
			if err := convert(&sections[index].Topic); err != nil {
				return err
			}
			if err := convert(&sections[index].Takeaway); err != nil {
				return err
			}
			for unitIndex := range sections[index].Units {
				unit := &sections[index].Units[unitIndex]
				if err := convert(&unit.Topic); err != nil {
					return err
				}
				if err := convert(&unit.Takeaway); err != nil {
					return err
				}
				if err := convert(&unit.Key); err != nil {
					return err
				}
				if err := convert(&unit.Reason); err != nil {
					return err
				}
			}
			if err := walk(sections[index].Children); err != nil {
				return err
			}
		}
		return nil
	}
	return walk(document.Sections)
}

func normalizeOutline(outline []SummaryOutline, segments []SummarySegment) []SummaryOutline {
	if len(outline) == 0 {
		return nil
	}
	byIndex := make(map[int]SummarySegment, len(segments))
	for _, segment := range segments {
		byIndex[segment.Index] = segment
	}
	var walk func([]SummaryOutline, int) []SummaryOutline
	walk = func(nodes []SummaryOutline, parent int) []SummaryOutline {
		result := make([]SummaryOutline, 0, len(nodes))
		for _, node := range nodes {
			node.Title = strings.TrimSpace(node.Title)
			if node.Title == "" || len(node.Title) > 400 || node.Level < 0 || node.Level > 8 || len(node.Indices) > summaryPointIndexLimit {
				continue
			}
			valid := true
			for _, index := range node.Indices {
				if _, ok := byIndex[index]; !ok {
					valid = false
					break
				}
			}
			if !valid {
				continue
			}
			if len(node.Indices) > 0 {
				first, last := byIndex[node.Indices[0]], byIndex[node.Indices[len(node.Indices)-1]]
				node.Start, node.End = first.Start, last.End
				node.Page = first.Page
			}
			node.Children = walk(node.Children, node.Level)
			result = append(result, node)
		}
		return result
	}
	return walk(outline, -1)
}
