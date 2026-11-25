package ai

import (
	"context"
	"strings"

	"tangentry/internal/citation"
	"tangentry/internal/freshness"
	"tangentry/internal/retrieval"
)

// ContentVersionReader looks up the live saved_content revision and hash.
// A missing item must return a Current with Exists=false and a nil error.
// Any error fails closed: callers never display evidence as currently valid.
type ContentVersionReader interface {
	CurrentContentVersion(ctx context.Context, contentID string) (freshness.Current, error)
}

// SetContentVersionReader connects the optional content-version lookup at the
// composition root. Without it, stored answers cannot be proven current and
// are assessed as if the lookup failed (fail closed).
func (s *Service) SetContentVersionReader(reader ContentVersionReader) {
	s.mu.Lock()
	s.contentVersions = reader
	s.mu.Unlock()
}

func (s *Service) contentVersionReader() ContentVersionReader {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.contentVersions
}

func lookupCurrent(ctx context.Context, reader ContentVersionReader, contentID string) (freshness.Current, error) {
	if reader == nil {
		return freshness.Current{}, errVersionLookupUnavailable
	}
	return reader.CurrentContentVersion(ctx, contentID)
}

// errVersionLookupUnavailable is a sentinel used when the service was not
// wired with a content version reader. Assess treats any lookup error as
// unavailable (fail closed), so this must be a non-nil error.
var errVersionLookupUnavailable = errSentinel("content version lookup is not configured")

// SourceTextReader is optional. A ContentVersionReader that also returns the
// saved source text from the same read lets chat check client-supplied
// context against that snapshot. Without it, client context cannot be proven
// to match the saved source and is not stamped current.
type SourceTextReader interface {
	SavedSourceText(ctx context.Context, contentID string) (text string, current freshness.Current, err error)
}

// sourceBinding is the saved-content snapshot the generation context was
// read from. Established is false when that identity cannot be proven.
// Stamp must not copy a later live row onto the answer in that case.
type sourceBinding struct {
	Established bool
	Revision    int64
	Hash        string
}

// bindGenerationSource captures the source identity at context-assembly time,
// before the provider runs. A later lookup is compared with this snapshot.
func (s *Service) bindGenerationSource(ctx context.Context, in ChatRequest, passages []retrieval.Passage) sourceBinding {
	reader := s.contentVersionReader()
	if in.Retrieval != nil && in.Retrieval.Enabled {
		return bindRetrievedSource(ctx, reader, in.ContentID, passages)
	}
	if strings.TrimSpace(in.Context) != "" {
		return bindClientContext(ctx, reader, in.ContentID, in.Context)
	}
	current, err := lookupCurrent(ctx, reader, in.ContentID)
	if err != nil || !current.Exists {
		return sourceBinding{}
	}
	return sourceBinding{Established: true, Revision: current.Revision, Hash: current.Hash}
}

func bindRetrievedSource(ctx context.Context, reader ContentVersionReader, contentID string, passages []retrieval.Passage) sourceBinding {
	current, err := lookupCurrent(ctx, reader, contentID)
	if err != nil || !current.Exists {
		return sourceBinding{}
	}
	revision, ok := uniformContentRevision(passages)
	if !ok {
		return sourceBinding{}
	}
	// The passages name an earlier revision than the row we just read.
	// Keep that earlier revision and do not attach the newer hash.
	if revision > 0 && revision != current.Revision {
		return sourceBinding{Established: true, Revision: revision}
	}
	return sourceBinding{Established: true, Revision: current.Revision, Hash: current.Hash}
}

func bindClientContext(ctx context.Context, reader ContentVersionReader, contentID, contextText string) sourceBinding {
	texter, ok := reader.(SourceTextReader)
	if !ok {
		return sourceBinding{}
	}
	text, current, err := texter.SavedSourceText(ctx, contentID)
	if err != nil || !current.Exists {
		return sourceBinding{}
	}
	if !clientContextMatchesSaved(contextText, text) {
		return sourceBinding{}
	}
	return sourceBinding{Established: true, Revision: current.Revision, Hash: current.Hash}
}

// clientContextMatchesSaved reports whether the client context contains the
// exact saved source text. A shorter or different excerpt is not treated as
// that snapshot.
func clientContextMatchesSaved(contextText, saved string) bool {
	saved = strings.TrimSpace(saved)
	if saved == "" {
		return false
	}
	return strings.Contains(contextText, saved)
}

// uniformContentRevision returns the shared content-revision of passages.
// ok is false when a passage version is present but is not a single
// content-revision value. No versioned passages returns 0, true.
func uniformContentRevision(passages []retrieval.Passage) (int64, bool) {
	var revision int64
	seen := false
	for _, passage := range passages {
		if strings.TrimSpace(passage.SourceVersion) == "" {
			continue
		}
		parsed := freshness.ParseRevision(passage.SourceVersion)
		if parsed <= 0 {
			return 0, false
		}
		if seen && parsed != revision {
			return 0, false
		}
		revision = parsed
		seen = true
	}
	return revision, true
}

type errSentinel string

func (e errSentinel) Error() string { return string(e) }

// stampFreshness records the context-time source snapshot on a newly stored
// assistant message and assesses it against the live row. A binding that was
// not established is not filled from the live row, so the answer cannot be
// current. Citations keep a passage revision that disagrees with the binding.
func stampFreshness(message *ChatMessage, contentID string, binding sourceBinding, current freshness.Current, lookupErr error) {
	if message == nil {
		return
	}
	if binding.Established {
		message.SourceRevision = binding.Revision
		message.SourceHash = binding.Hash
	}
	message.VersionStatus = freshness.Assess(messageSnapshot(*message, contentID), current, lookupErr)
	for i := range message.Citations {
		fillCitationSnapshot(&message.Citations[i], binding)
		message.Citations[i].Freshness = freshness.Assess(citationSnapshot(message.Citations[i], contentID), current, lookupErr)
	}
}

func fillCitationSnapshot(c *citation.Citation, binding sourceBinding) {
	if c == nil || !binding.Established {
		return
	}
	passageRevision := freshness.ParseRevision(c.SourceVersion)
	if passageRevision > 0 && passageRevision != binding.Revision {
		return
	}
	if c.ContentRevision == 0 {
		c.ContentRevision = binding.Revision
	}
	if c.ContentHash == "" {
		c.ContentHash = binding.Hash
	}
}

// applyFreshness recomputes version status for stored messages against the
// live content. It does not rewrite C's generation-time Status field.
func applyFreshness(messages []ChatMessage, contentID string, current freshness.Current, lookupErr error) []ChatMessage {
	out := append([]ChatMessage(nil), messages...)
	for i := range out {
		if out[i].Role != "assistant" {
			out[i].VersionStatus = ""
			continue
		}
		out[i].VersionStatus = freshness.Assess(messageSnapshot(out[i], contentID), current, lookupErr)
		if len(out[i].Citations) == 0 {
			continue
		}
		citations := append([]citation.Citation(nil), out[i].Citations...)
		var worst freshness.Status
		for j := range citations {
			citations[j].Freshness = freshness.Assess(citationSnapshot(citations[j], contentID), current, lookupErr)
			worst = freshness.Worst(worst, citations[j].Freshness)
		}
		out[i].Citations = citations
		out[i].VersionStatus = freshness.Worst(out[i].VersionStatus, worst)
	}
	return out
}

func messageSnapshot(message ChatMessage, contentID string) freshness.Snapshot {
	return freshness.Snapshot{
		ContentID:       contentID,
		ContentRevision: message.SourceRevision,
		ContentHash:     message.SourceHash,
	}
}

func citationSnapshot(c citation.Citation, fallbackContentID string) freshness.Snapshot {
	contentID := c.ContentID
	if contentID == "" {
		contentID = fallbackContentID
	}
	return freshness.Snapshot{
		ContentID:       contentID,
		ContentRevision: c.ContentRevision,
		ContentHash:     c.ContentHash,
		SourceVersion:   c.SourceVersion,
	}
}

// stripUntrustedEvidence clears client-supplied citations and freshness so
// only server-assessed values are stored or returned. It does not consult
// the stored transcript; restoreServerEvidence puts that evidence back
// onto turns the client still repeated verbatim.
func stripUntrustedEvidence(messages []ChatMessage) {
	for i := range messages {
		messages[i].Citations = nil
		messages[i].VersionStatus = ""
		messages[i].SourceRevision = 0
		messages[i].SourceHash = ""
	}
}

// restoreServerEvidence copies server-owned snapshots and citations onto
// client messages that still match the stored transcript. A message inherits
// that evidence only when every message up to and including it has the same
// position, role, and exact content. Client-supplied evidence was already
// stripped and is never read here. Version status is left unset so the
// following assessment recomputes it from the restored snapshot.
func restoreServerEvidence(client, stored []ChatMessage) {
	n := len(client)
	if len(stored) < n {
		n = len(stored)
	}
	for i := 0; i < n; i++ {
		if client[i].Role != stored[i].Role || client[i].Content != stored[i].Content {
			return
		}
		if stored[i].Role != "assistant" {
			continue
		}
		client[i].SourceRevision = stored[i].SourceRevision
		client[i].SourceHash = stored[i].SourceHash
		client[i].Citations = cloneCitations(stored[i].Citations)
	}
}

func cloneCitations(in []citation.Citation) []citation.Citation {
	if len(in) == 0 {
		return nil
	}
	out := make([]citation.Citation, len(in))
	copy(out, in)
	return out
}

func (s *Service) assessHistory(ctx context.Context, contentID string, messages []ChatMessage) []ChatMessage {
	current, err := lookupCurrent(ctx, s.contentVersionReader(), contentID)
	return applyFreshness(messages, contentID, current, err)
}
