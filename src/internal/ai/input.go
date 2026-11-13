package ai

import (
	"bytes"
	"encoding/json"
	"io"
	"tangentry/internal/subtitles"
)

// The HTTP contract requires actual cue bounds and indices, rather than
// silently treating an omitted/null start as a real zero-second position.
func (in *Input) UnmarshalJSON(data []byte) error {
	var raw struct {
		Source           string  `json:"source"`
		ContentID        *string `json:"contentId"`
		TargetLanguage   string  `json:"targetLanguage"`
		Revision         *int64  `json:"revision"`
		Regenerate       bool    `json:"regenerate"`
		MaxProviderCalls *int    `json:"maxProviderCalls"`
		Track            *struct {
			Language  *string `json:"language"`
			Label     string  `json:"label"`
			Automatic bool    `json:"automatic"`
			Cues      *[]struct {
				Start *float64 `json:"start"`
				End   *float64 `json:"end"`
				Text  *string  `json:"text"`
			} `json:"cues"`
		} `json:"track"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
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
	if raw.ContentID == nil || raw.Revision == nil || raw.Track == nil || raw.Track.Language == nil || raw.Track.Cues == nil {
		return ErrInvalid
	}
	track := subtitles.Track{Language: *raw.Track.Language, Label: raw.Track.Label, Automatic: raw.Track.Automatic, Cues: make([]subtitles.Cue, len(*raw.Track.Cues))}
	for i, cue := range *raw.Track.Cues {
		if cue.Start == nil || cue.End == nil || cue.Text == nil {
			return ErrInvalid
		}
		track.Cues[i] = subtitles.Cue{Start: *cue.Start, End: *cue.End, Text: *cue.Text}
	}
	*in = Input{
		ContentID:        *raw.ContentID,
		Revision:         *raw.Revision,
		Track:            track,
		Source:           raw.Source,
		TargetLanguage:   raw.TargetLanguage,
		Regenerate:       raw.Regenerate,
		MaxProviderCalls: raw.MaxProviderCalls,
	}
	return nil
}
