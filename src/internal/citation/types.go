package citation

import "tangentry/internal/freshness"

type Status string

const (
	StatusVerified   Status = "verified"
	StatusInvalid    Status = "invalid"
	StatusUnverified Status = "unverified"
	StatusStale      Status = "stale"
	StatusAmbiguous  Status = "ambiguous"
)

type Citation struct {
	ID            string `json:"id"`
	PassageID     string `json:"passageId"`
	ContentID     string `json:"contentId"`
	Source        string `json:"source"`
	SourceVersion string `json:"sourceVersion"`
	Locator       string `json:"locator"`

	Quote     string `json:"quote"`
	StartRune int    `json:"startRune"`
	EndRune   int    `json:"endRune"`

	Status Status `json:"status"`
	Reason string `json:"reason,omitempty"`

	// ContentRevision and ContentHash capture the saved content snapshot at
	// generation time (E / FR-05). Freshness is recomputed on read and is
	// not a substitute for Status, which is C's generation-time result.
	ContentRevision int64            `json:"contentRevision,omitempty"`
	ContentHash     string           `json:"contentHash,omitempty"`
	Freshness       freshness.Status `json:"freshness,omitempty"`
}

type RawCitation struct {
	PassageID string
	Quote     string
}
