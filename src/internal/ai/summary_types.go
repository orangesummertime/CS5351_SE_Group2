package ai

import (
	"context"
	"encoding/json"
	"io"
	"strings"
)

type SummarySegment struct {
	Index          int      `json:"index"`
	Start          float64  `json:"start"`
	End            float64  `json:"end"`
	Page           int      `json:"page,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Heading        string   `json:"heading,omitempty"`
	SegmentIndexes []int    `json:"segmentIndexes,omitempty"`
	CueIDs         []string `json:"cueIds,omitempty"`
	Text           string   `json:"text"`
}

// SummaryBlock is an optional structured source block. It is intentionally
// small so article and PDF callers can preserve their source shape without
// changing the resumable segment pipeline.
type SummaryBlock struct {
	Kind           string `json:"kind"`
	Heading        string `json:"heading,omitempty"`
	Text           string `json:"text"`
	Page           int    `json:"page,omitempty"`
	SegmentIndexes []int  `json:"segmentIndexes,omitempty"`
}

type SummaryInput struct {
	ContentID string           `json:"contentId"`
	Source    string           `json:"source"`
	Text      string           `json:"text,omitempty"`
	Segments  []SummarySegment `json:"segments,omitempty"`
	Blocks    []SummaryBlock   `json:"blocks,omitempty"`
	Revision  int64            `json:"revision"`
	// Regenerate discards the stored summary of this text first.
	Regenerate bool `json:"regenerate,omitempty"`
	// MaxProviderCalls optionally caps started completion POSTs for this
	// invocation. Omitted means unlimited; explicit values must be 1..100.
	MaxProviderCalls *int `json:"maxProviderCalls,omitempty"`
}

type SummaryPoint struct {
	Text    string `json:"text"`
	Indices []int  `json:"indices"`
}

type SummaryDraft struct {
	Overview string           `json:"overview"`
	Points   []SummaryPoint   `json:"points"`
	Outline  []SummaryOutline `json:"outline,omitempty"`
	Document *SummaryDocument `json:"document,omitempty"`
}

type SummaryOutline struct {
	Title    string           `json:"title"`
	Level    int              `json:"level"`
	Indices  []int            `json:"indices,omitempty"`
	Start    float64          `json:"start,omitempty"`
	End      float64          `json:"end,omitempty"`
	Page     int              `json:"page,omitempty"`
	Anchor   string           `json:"anchor,omitempty"`
	Children []SummaryOutline `json:"children,omitempty"`
}

type SummaryCitation struct {
	Index int     `json:"index"`
	Start float64 `json:"start,omitempty"`
	End   float64 `json:"end,omitempty"`
	Quote string  `json:"quote"`
	Page  int     `json:"page,omitempty"`
}

const summaryFormatVersion = 2

// SummaryEvidence keeps an AI claim attached to the exact source material
// that supports it. Indices remain compatible with legacy citations while the
// richer fields support sentence, cue, timestamp, and PDF-page navigation.
type SummaryEvidence struct {
	ID             string   `json:"id,omitempty"`
	Index          int      `json:"index,omitempty"`
	SentenceID     string   `json:"sentenceId,omitempty"`
	Quote          string   `json:"quote"`
	Start          float64  `json:"start,omitempty"`
	End            float64  `json:"end,omitempty"`
	Page           int      `json:"page,omitempty"`
	CueIDs         []string `json:"cueIds,omitempty"`
	SegmentIndexes []int    `json:"segmentIndexes,omitempty"`
}

// Preserve a real video cue that starts at zero while keeping untimed article
// evidence compact on the wire.
func (evidence SummaryEvidence) MarshalJSON() ([]byte, error) {
	type wire struct {
		ID             string   `json:"id,omitempty"`
		Index          int      `json:"index,omitempty"`
		SentenceID     string   `json:"sentenceId,omitempty"`
		Quote          string   `json:"quote"`
		Start          *float64 `json:"start,omitempty"`
		End            *float64 `json:"end,omitempty"`
		Page           int      `json:"page,omitempty"`
		CueIDs         []string `json:"cueIds,omitempty"`
		SegmentIndexes []int    `json:"segmentIndexes,omitempty"`
	}
	value := wire{ID: evidence.ID, Index: evidence.Index, SentenceID: evidence.SentenceID, Quote: evidence.Quote, Page: evidence.Page, CueIDs: evidence.CueIDs, SegmentIndexes: evidence.SegmentIndexes}
	if evidence.End > evidence.Start {
		value.Start, value.End = &evidence.Start, &evidence.End
	}
	return json.Marshal(value)
}

type SummarySentence struct {
	ID       string            `json:"id"`
	Text     string            `json:"text"`
	Evidence []SummaryEvidence `json:"evidence,omitempty"`
	Start    float64           `json:"start,omitempty"`
	End      float64           `json:"end,omitempty"`
	Page     int               `json:"page,omitempty"`
}

type SummaryUnit struct {
	UnitID           string            `json:"unitId"`
	Type             string            `json:"type,omitempty"`
	Role             string            `json:"role,omitempty"`
	Topic            string            `json:"topic,omitempty"`
	Takeaway         string            `json:"takeaway,omitempty"`
	Key              string            `json:"key,omitempty"`
	Reason           string            `json:"reason,omitempty"`
	Full             string            `json:"full,omitempty"`
	Sentences        []SummarySentence `json:"sentences,omitempty"`
	BriefSentenceIDs []string          `json:"briefSentenceIds,omitempty"`
	Evidence         []SummaryEvidence `json:"evidence,omitempty"`
	Source           []SummaryEvidence `json:"source,omitempty"`
}

type SummarySection struct {
	SectionID string            `json:"sectionId"`
	Title     string            `json:"title"`
	Level     int               `json:"level,omitempty"`
	Topic     string            `json:"topic,omitempty"`
	Takeaway  string            `json:"takeaway,omitempty"`
	Start     float64           `json:"start,omitempty"`
	End       float64           `json:"end,omitempty"`
	Page      int               `json:"page,omitempty"`
	Source    []SummaryEvidence `json:"source,omitempty"`
	Evidence  []SummaryEvidence `json:"evidence,omitempty"`
	Units     []SummaryUnit     `json:"units,omitempty"`
	Children  []SummarySection  `json:"children,omitempty"`
}

type SummaryDocument struct {
	FormatVersion int               `json:"formatVersion"`
	ContentID     string            `json:"contentId"`
	Source        string            `json:"source"`
	Model         string            `json:"model,omitempty"`
	Provenance    map[string]string `json:"provenance,omitempty"`
	Levels        []string          `json:"levels"`
	Topic         string            `json:"topic,omitempty"`
	Sections      []SummarySection  `json:"sections"`
}

type SummaryProgress struct {
	CompletedCalls    int `json:"completedCalls"`
	TotalCalls        int `json:"totalCalls"`
	CompletedSections int `json:"completedSections,omitempty"`
	TotalSections     int `json:"totalSections,omitempty"`
	// CurrentCall is one-based and only set while a provider request is in
	// flight. Keeping it in the checkpoint lets the UI distinguish a slow
	// model response from a stalled generation.
	CurrentCall int    `json:"currentCall,omitempty"`
	Phase       string `json:"phase,omitempty"`
	StartedAt   int64  `json:"startedAt,omitempty"`
	Error       string `json:"error,omitempty"`
	Complete    bool   `json:"complete"`
}

type SummaryTitleOverride struct {
	NodeID string `json:"nodeId"`
	Title  string `json:"title"`
}

type SummaryTitleOverrides struct {
	Key       string                 `json:"key"`
	Overrides []SummaryTitleOverride `json:"overrides"`
}

// A timed citation may start at zero. Keep article citations untimed without
// dropping the first video's exact start from the wire or persisted cache.
func (citation SummaryCitation) MarshalJSON() ([]byte, error) {
	type wireCitation struct {
		Index int      `json:"index"`
		Start *float64 `json:"start,omitempty"`
		End   *float64 `json:"end,omitempty"`
		Quote string   `json:"quote"`
		Page  int      `json:"page,omitempty"`
	}
	value := wireCitation{Index: citation.Index, Quote: citation.Quote, Page: citation.Page}
	if citation.End > citation.Start {
		value.Start, value.End = &citation.Start, &citation.End
	}
	return json.Marshal(value)
}

type SummaryResult struct {
	Key            string            `json:"key"`
	ContentID      string            `json:"contentId"`
	Source         string            `json:"source"`
	BaseURL        string            `json:"baseUrl"`
	Model          string            `json:"model"`
	TotalCalls     int               `json:"totalCalls"`
	CompletedCalls int               `json:"completedCalls"`
	Complete       bool              `json:"complete"`
	FormatVersion  int               `json:"formatVersion"`
	Document       *SummaryDocument  `json:"document,omitempty"`
	Progress       SummaryProgress   `json:"progress"`
	Overview       string            `json:"overview,omitempty"`
	Points         []SummaryPoint    `json:"points,omitempty"`
	Citations      []SummaryCitation `json:"citations,omitempty"`
	Outline        []SummaryOutline  `json:"outline,omitempty"`
	UpdatedAt      int64             `json:"updatedAt"`
	// Budget is present only for limited invocations.
	Budget *BudgetReport `json:"budget,omitempty"`
}

type SummaryInspection struct {
	Settings Status        `json:"settings"`
	Result   SummaryResult `json:"result"`
	Active   bool          `json:"active"`
}

type SummaryRequest struct {
	Mode       string
	Prompt     string
	UserPrompt string
	Segments   []SummarySegment
	Drafts     []SummaryDraft
}

type Summarizer interface {
	Summarize(context.Context, Config, SummaryRequest) (SummaryDraft, error)
}

// DocumentSummarizer is an optional provider capability. Providers that can
// produce and validate a complete content tree in one response opt in; other
// providers continue using the resumable leaf/reduce protocol.
type DocumentSummarizer interface {
	SupportsSummaryDocument(Config) bool
}

type summaryCache struct {
	Result  SummaryResult  `json:"result"`
	Current []SummaryDraft `json:"current"`
	Next    []SummaryDraft `json:"next,omitempty"`
}

func (in *SummaryInput) UnmarshalJSON(data []byte) error {
	var raw struct {
		ContentID        *string           `json:"contentId"`
		Source           *string           `json:"source"`
		Text             *string           `json:"text"`
		Segments         *[]SummarySegment `json:"segments"`
		Blocks           *[]SummaryBlock   `json:"blocks"`
		Revision         *int64            `json:"revision"`
		Regenerate       bool              `json:"regenerate"`
		MaxProviderCalls *int              `json:"maxProviderCalls"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		if err == nil {
			return ErrInvalid
		}
		return err
	}
	if raw.ContentID == nil || raw.Source == nil || raw.Revision == nil || raw.Text == nil && raw.Segments == nil && raw.Blocks == nil {
		return ErrInvalid
	}
	*in = SummaryInput{ContentID: *raw.ContentID, Source: *raw.Source, Revision: *raw.Revision, Regenerate: raw.Regenerate, MaxProviderCalls: raw.MaxProviderCalls}
	if raw.Text != nil {
		in.Text = *raw.Text
	}
	if raw.Segments != nil {
		in.Segments = *raw.Segments
	}
	if raw.Blocks != nil {
		in.Blocks = *raw.Blocks
	}
	return nil
}
