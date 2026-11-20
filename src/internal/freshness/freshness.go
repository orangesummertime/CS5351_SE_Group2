package freshness

import (
	"strconv"
	"strings"
)

// Status is the user-visible freshness of a stored chat answer or citation
// after the underlying saved content may have changed. It is computed from
// captured snapshots and the current content row; it is not C's generation-time
// verification status (verified/invalid/unverified/stale/ambiguous).
type Status string

const (
	// StatusCurrent means the captured snapshot still matches the live saved
	// content (same content_revision and content_hash).
	StatusCurrent Status = "current"
	// StatusExpired means the content still exists but no longer matches the
	// snapshot captured when the answer or citation was stored.
	StatusExpired Status = "expired"
	// StatusUnavailable means the content has been deleted or cannot be found.
	StatusUnavailable Status = "unavailable"
)

// Snapshot is the server-owned content version captured when an answer or
// citation was stored. Legacy pre-E rows may have only a SourceVersion string.
type Snapshot struct {
	ContentID       string
	ContentRevision int64
	ContentHash     string
	// SourceVersion is C's "content-revision:N" string; used only when
	// ContentRevision is not already set.
	SourceVersion string
}

// Current is the live saved_content row, or a missing row.
type Current struct {
	Exists   bool
	Revision int64
	Hash     string
}

// ParseRevision extracts N from "content-revision:N". It returns 0 if the
// value is empty or not in that form.
func ParseRevision(sourceVersion string) int64 {
	const prefix = "content-revision:"
	if !strings.HasPrefix(sourceVersion, prefix) {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(sourceVersion[len(prefix):]), 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// Revision returns the captured revision, preferring the explicit field and
// falling back to SourceVersion.
func (s Snapshot) Revision() int64 {
	if s.ContentRevision > 0 {
		return s.ContentRevision
	}
	return ParseRevision(s.SourceVersion)
}

// Assess compares a captured snapshot with the current saved content.
// Lookup errors must be reported by the caller via lookupErr; any lookup
// failure fails closed and is never displayed as current.
func Assess(captured Snapshot, current Current, lookupErr error) Status {
	if lookupErr != nil {
		// A missing row is not an error; the reader should pass Exists=false
		// with a nil error. Any other error cannot be trusted as a match.
		return StatusUnavailable
	}
	if !current.Exists {
		return StatusUnavailable
	}
	revision := captured.Revision()
	hash := strings.TrimSpace(captured.ContentHash)
	// Legacy or incomplete snapshots cannot be proven current.
	if revision <= 0 && hash == "" {
		return StatusExpired
	}
	if revision > 0 && current.Revision != revision {
		return StatusExpired
	}
	if hash != "" && current.Hash != hash {
		return StatusExpired
	}
	return StatusCurrent
}

// DisplayedAsCurrentlyValid reports whether a status may be shown as
// "currently valid". NFR-05 requires this to be false for every stale case.
func DisplayedAsCurrentlyValid(status Status) bool {
	return status == StatusCurrent
}

// Worst returns the more conservative of two statuses. Unavailable outranks
// expired, which outranks current or an empty value.
func Worst(a, b Status) Status {
	rank := func(s Status) int {
		switch s {
		case StatusUnavailable:
			return 3
		case StatusExpired:
			return 2
		case StatusCurrent:
			return 1
		default:
			return 0
		}
	}
	if rank(b) > rank(a) {
		return b
	}
	if rank(a) > 0 {
		return a
	}
	return b
}
