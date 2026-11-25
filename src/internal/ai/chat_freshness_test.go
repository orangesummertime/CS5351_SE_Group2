package ai

import (
	"context"
	"sync"
	"testing"

	"tangentry/internal/citation"
	"tangentry/internal/freshness"
)

// memoryVersions is a concurrency-safe ContentVersionReader for tests.
type memoryVersions struct {
	mu      sync.Mutex
	current map[string]freshness.Current
	err     error
}

func newMemoryVersions() *memoryVersions {
	return &memoryVersions{current: map[string]freshness.Current{}}
}

func (m *memoryVersions) set(id string, current freshness.Current) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current[id] = current
}

func (m *memoryVersions) delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.current[id] = freshness.Current{Exists: false}
}

func (m *memoryVersions) CurrentContentVersion(_ context.Context, contentID string) (freshness.Current, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return freshness.Current{}, m.err
	}
	current, ok := m.current[contentID]
	if !ok {
		return freshness.Current{Exists: false}, nil
	}
	return current, nil
}

func chatFreshnessService(t *testing.T, versions *memoryVersions) *Service {
	t.Helper()
	provider := &retrievalChatProvider{answer: "根据原文，答案是苹果。"}
	service, _, _ := configured(t, provider)
	if versions != nil {
		service.SetContentVersionReader(versions)
	}
	return service
}

func sendChat(t *testing.T, service *Service, contentID, question string) ChatResult {
	t.Helper()
	result, err := service.Chat(context.Background(), ChatRequest{
		ContentID: contentID,
		Revision:  service.Status().Revision,
		Model:     "fixture",
		Messages:  []ChatMessage{{Role: "user", Content: question}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	return result
}

func assistantOf(t *testing.T, result ChatResult) ChatMessage {
	t.Helper()
	if len(result.Messages) < 2 || result.Messages[len(result.Messages)-1].Role != "assistant" {
		t.Fatalf("missing assistant message: %+v", result.Messages)
	}
	return result.Messages[len(result.Messages)-1]
}

func TestETStampAndReopenAfterUpdateAndDelete(t *testing.T) {
	const id = "rss:freshness-1"
	versions := newMemoryVersions()
	versions.set(id, freshness.Current{Exists: true, Revision: 1, Hash: "hash-v1"})
	service := chatFreshnessService(t, versions)

	created := sendChat(t, service, id, "果实是什么？")
	first := assistantOf(t, created)
	if first.SourceRevision != 1 || first.SourceHash != "hash-v1" {
		t.Fatalf("snapshot not stamped: rev=%d hash=%q", first.SourceRevision, first.SourceHash)
	}
	if first.VersionStatus != freshness.StatusCurrent {
		t.Fatalf("new answer status = %q, want current", first.VersionStatus)
	}

	// Reopen while unchanged.
	reopen, err := service.ChatHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got := assistantOf(t, reopen).VersionStatus; got != freshness.StatusCurrent {
		t.Fatalf("reopen unchanged = %q", got)
	}

	// Update the source and reopen: the stored answer must be expired, not current.
	versions.set(id, freshness.Current{Exists: true, Revision: 2, Hash: "hash-v2"})
	afterUpdate, err := service.ChatHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	updated := assistantOf(t, afterUpdate)
	if updated.VersionStatus != freshness.StatusExpired {
		t.Fatalf("after update = %q, want expired", updated.VersionStatus)
	}
	if freshness.DisplayedAsCurrentlyValid(updated.VersionStatus) {
		t.Fatal("updated source still displayed as currently valid")
	}
	// C-style Status, if present, must not override freshness.
	for _, c := range updated.Citations {
		if freshness.DisplayedAsCurrentlyValid(c.Freshness) {
			t.Fatalf("citation still current after update: %+v", c)
		}
	}

	// Second reopen is stable.
	again, err := service.ChatHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if assistantOf(t, again).VersionStatus != freshness.StatusExpired {
		t.Fatal("reopen after update was not stable")
	}

	// Delete the source: reopen must report unavailable.
	versions.delete(id)
	afterDelete, err := service.ChatHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got := assistantOf(t, afterDelete).VersionStatus; got != freshness.StatusUnavailable {
		t.Fatalf("after delete = %q, want unavailable", got)
	}
	if freshness.DisplayedAsCurrentlyValid(assistantOf(t, afterDelete).VersionStatus) {
		t.Fatal("deleted source still displayed as currently valid")
	}
}

func TestETNewAnswerAfterUpdateIsCurrentWhileOldIsExpired(t *testing.T) {
	const id = "rss:freshness-2"
	versions := newMemoryVersions()
	versions.set(id, freshness.Current{Exists: true, Revision: 1, Hash: "h1"})
	service := chatFreshnessService(t, versions)
	first := sendChat(t, service, id, "第一问")

	versions.set(id, freshness.Current{Exists: true, Revision: 2, Hash: "h2"})
	history := append(append([]ChatMessage(nil), first.Messages...), ChatMessage{Role: "user", Content: "第二问"})
	second, err := service.Chat(context.Background(), ChatRequest{
		ContentID: id,
		Revision:  service.Status().Revision,
		Model:     "fixture",
		Messages:  history,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Messages) < 4 {
		t.Fatalf("want two turns, got %d messages", len(second.Messages))
	}
	oldAnswer, newAnswer := second.Messages[1], second.Messages[3]
	if oldAnswer.VersionStatus != freshness.StatusExpired {
		t.Fatalf("old answer = %q, want expired", oldAnswer.VersionStatus)
	}
	// Expiry must be the revision mismatch. Stripping the captured snapshot
	// would also look expired and would hide this regression.
	if oldAnswer.SourceRevision != 1 || oldAnswer.SourceHash != "h1" {
		t.Fatalf("old answer lost its captured snapshot: rev=%d hash=%q", oldAnswer.SourceRevision, oldAnswer.SourceHash)
	}
	if newAnswer.VersionStatus != freshness.StatusCurrent || newAnswer.SourceRevision != 2 || newAnswer.SourceHash != "h2" {
		t.Fatalf("new answer = %+v", newAnswer)
	}
}

func TestETUnchangedSourceSecondTurnKeepsEarlierAnswerCurrent(t *testing.T) {
	const id = "rss:freshness-two-turn"
	versions := newMemoryVersions()
	versions.set(id, freshness.Current{Exists: true, Revision: 1, Hash: "hash-v1"})
	provider := &retrievalChatProvider{answer: "根据原文，答案是苹果。[[cite:p1|苹果]]"}
	service, _, _ := configured(t, provider)
	service.SetContentVersionReader(versions)

	first, err := service.Chat(context.Background(), ChatRequest{
		ContentID: id,
		Revision:  service.Status().Revision,
		Model:     "fixture",
		Messages:  []ChatMessage{{Role: "user", Content: "果实是什么？"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	original := assistantOf(t, first)
	if original.VersionStatus != freshness.StatusCurrent || original.SourceRevision != 1 || original.SourceHash != "hash-v1" {
		t.Fatalf("first answer = rev=%d hash=%q status=%q", original.SourceRevision, original.SourceHash, original.VersionStatus)
	}
	if len(original.Citations) != 1 || original.Citations[0].Quote != "苹果" || original.Citations[0].ContentRevision != 1 || original.Citations[0].ContentHash != "hash-v1" {
		t.Fatalf("first citation = %+v", original.Citations)
	}

	// The UI resends the transcript. Forged evidence on that resend must be
	// ignored; the unchanged source must leave the first answer current.
	resent := append([]ChatMessage(nil), first.Messages...)
	resent[1].VersionStatus = freshness.StatusExpired
	resent[1].SourceRevision = 99
	resent[1].SourceHash = "forged-hash"
	resent[1].Citations = []citation.Citation{{
		ID: "forged", Quote: "伪造引文", Status: citation.StatusVerified,
		ContentRevision: 99, ContentHash: "forged-hash", Freshness: freshness.StatusCurrent,
	}}
	resent = append(resent, ChatMessage{
		Role: "user", Content: "第二问",
		VersionStatus: freshness.StatusCurrent, SourceRevision: 99, SourceHash: "forged-hash",
		Citations: []citation.Citation{{ID: "user-forged", Quote: "伪造", Status: citation.StatusVerified, Freshness: freshness.StatusCurrent}},
	})
	second, err := service.Chat(context.Background(), ChatRequest{
		ContentID: id,
		Revision:  service.Status().Revision,
		Model:     "fixture",
		Messages:  resent,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Messages) != 4 {
		t.Fatalf("want two turns, got %d messages", len(second.Messages))
	}
	assertEarlierAnswerKept(t, second.Messages[1], original)
	if second.Messages[2].VersionStatus != "" || second.Messages[2].SourceRevision != 0 || second.Messages[2].SourceHash != "" || len(second.Messages[2].Citations) != 0 {
		t.Fatalf("forged user evidence was retained: %+v", second.Messages[2])
	}
	latest := second.Messages[3]
	if latest.VersionStatus != freshness.StatusCurrent || latest.SourceRevision != 1 || latest.SourceHash != "hash-v1" {
		t.Fatalf("second answer = rev=%d hash=%q status=%q", latest.SourceRevision, latest.SourceHash, latest.VersionStatus)
	}
	if len(provider.lastMessages) < 2 || len(provider.lastMessages[1].Citations) != 1 || provider.lastMessages[1].Citations[0].Quote != "苹果" {
		t.Fatalf("provider saw forged or missing evidence: %+v", provider.lastMessages)
	}

	reopen, err := service.ChatHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopen.Messages) != 4 {
		t.Fatalf("reopen messages = %d, want 4", len(reopen.Messages))
	}
	assertEarlierAnswerKept(t, reopen.Messages[1], original)
	if reopen.Messages[1].Content != original.Content || reopen.Messages[3].Content != latest.Content {
		t.Fatal("reopen changed answer text")
	}
}

func assertEarlierAnswerKept(t *testing.T, got, original ChatMessage) {
	t.Helper()
	if got.Content != original.Content {
		t.Fatalf("content = %q, want %q", got.Content, original.Content)
	}
	if got.VersionStatus != freshness.StatusCurrent {
		t.Fatalf("unchanged source status = %q, want current", got.VersionStatus)
	}
	if !freshness.DisplayedAsCurrentlyValid(got.VersionStatus) {
		t.Fatal("unchanged source was not displayed as currently valid")
	}
	if got.SourceRevision != original.SourceRevision || got.SourceHash != original.SourceHash {
		t.Fatalf("snapshot = rev=%d hash=%q, want rev=%d hash=%q", got.SourceRevision, got.SourceHash, original.SourceRevision, original.SourceHash)
	}
	if len(got.Citations) != 1 || got.Citations[0].Quote != original.Citations[0].Quote || got.Citations[0].Status != original.Citations[0].Status || got.Citations[0].ContentRevision != original.Citations[0].ContentRevision || got.Citations[0].ContentHash != original.Citations[0].ContentHash {
		t.Fatalf("citation = %+v, want %+v", got.Citations, original.Citations)
	}
	if got.Citations[0].Quote == "伪造引文" || got.Citations[0].ContentHash == "forged-hash" {
		t.Fatalf("forged citation was stored: %+v", got.Citations[0])
	}
}

func TestETClientForgedFreshnessIsStripped(t *testing.T) {
	const id = "rss:freshness-forged"
	versions := newMemoryVersions()
	versions.set(id, freshness.Current{Exists: true, Revision: 1, Hash: "real"})
	service := chatFreshnessService(t, versions)
	result, err := service.Chat(context.Background(), ChatRequest{
		ContentID: id,
		Revision:  service.Status().Revision,
		Model:     "fixture",
		Messages: []ChatMessage{{
			Role:           "user",
			Content:        "伪造状态",
			VersionStatus:  freshness.StatusCurrent,
			SourceRevision: 99,
			SourceHash:     "forged-hash",
			Citations: []citation.Citation{{
				ID: "c1", Status: citation.StatusVerified, Freshness: freshness.StatusCurrent,
				ContentRevision: 99, ContentHash: "forged-hash",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Messages[0].VersionStatus != "" || result.Messages[0].SourceHash != "" || result.Messages[0].SourceRevision != 0 || len(result.Messages[0].Citations) != 0 {
		t.Fatalf("forged user evidence was retained: %+v", result.Messages[0])
	}
}

func TestETLookupErrorFailsClosed(t *testing.T) {
	const id = "rss:freshness-err"
	versions := newMemoryVersions()
	versions.set(id, freshness.Current{Exists: true, Revision: 1, Hash: "h"})
	service := chatFreshnessService(t, versions)
	_ = sendChat(t, service, id, "问题")
	versions.err = errSentinel("disk failed")
	history, err := service.ChatHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got := assistantOf(t, history).VersionStatus; got != freshness.StatusUnavailable {
		t.Fatalf("lookup error status = %q, want unavailable", got)
	}
	if freshness.DisplayedAsCurrentlyValid(assistantOf(t, history).VersionStatus) {
		t.Fatal("lookup error displayed as currently valid")
	}
}

func TestETUnconfiguredReaderFailsClosed(t *testing.T) {
	service := chatFreshnessService(t, nil)
	result := sendChat(t, service, "rss:unwired", "问题")
	if freshness.DisplayedAsCurrentlyValid(assistantOf(t, result).VersionStatus) {
		t.Fatal("unwired reader displayed answer as currently valid")
	}
}

func TestETConcurrentUpdateDuringReopen(t *testing.T) {
	const id = "rss:freshness-race"
	versions := newMemoryVersions()
	versions.set(id, freshness.Current{Exists: true, Revision: 1, Hash: "before"})
	service := chatFreshnessService(t, versions)
	_ = sendChat(t, service, id, "问题")

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_, _ = service.ChatHistory(context.Background(), id)
		}
	}()
	go func() {
		defer wg.Done()
		versions.set(id, freshness.Current{Exists: true, Revision: 2, Hash: "after"})
	}()
	wg.Wait()

	final, err := service.ChatHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if assistantOf(t, final).VersionStatus != freshness.StatusExpired {
		t.Fatalf("after concurrent update, status = %q, want expired", assistantOf(t, final).VersionStatus)
	}
	if freshness.DisplayedAsCurrentlyValid(assistantOf(t, final).VersionStatus) {
		t.Fatal("concurrent update left evidence displayed as currently valid")
	}
}

func TestETApplyFreshnessOnStoredCitations(t *testing.T) {
	messages := []ChatMessage{{
		Role:           "assistant",
		Content:        "旧答案",
		SourceRevision: 1,
		SourceHash:     "old",
		VersionStatus:  freshness.StatusCurrent,
		Citations: []citation.Citation{{
			ID: "c1", ContentID: "rss:x", SourceVersion: "content-revision:1",
			ContentRevision: 1, ContentHash: "old", Status: citation.StatusVerified,
			Freshness: freshness.StatusCurrent, Quote: "原文",
		}},
	}}
	out := applyFreshness(messages, "rss:x", freshness.Current{Exists: true, Revision: 2, Hash: "new"}, nil)
	if out[0].VersionStatus != freshness.StatusExpired {
		t.Fatalf("message status = %q", out[0].VersionStatus)
	}
	if out[0].Citations[0].Freshness != freshness.StatusExpired {
		t.Fatalf("citation freshness = %q", out[0].Citations[0].Freshness)
	}
	if out[0].Citations[0].Status != citation.StatusVerified {
		t.Fatal("C generation-time status must be preserved")
	}
	if freshness.DisplayedAsCurrentlyValid(out[0].Citations[0].Freshness) {
		t.Fatal("expired citation displayed as currently valid")
	}
}

// textVersions can prove whether client context matches a saved source.
// memoryVersions cannot, so client context stays unverified with it.
type textVersions struct {
	*memoryVersions
	text string
}

func (t textVersions) SavedSourceText(ctx context.Context, contentID string) (string, freshness.Current, error) {
	current, err := t.CurrentContentVersion(ctx, contentID)
	if err != nil {
		return "", freshness.Current{}, err
	}
	return t.text, current, nil
}

func TestETSourceChangeDuringGenerationIsNotCurrent(t *testing.T) {
	const id = "rss:freshness-mid-generation"
	versions := newMemoryVersions()
	versions.set(id, freshness.Current{Exists: true, Revision: 1, Hash: "hash-v1"})
	provider := &retrievalChatProvider{answer: "根据生成开始时的原文回答。"}
	provider.onChat = func() {
		// Save lands after context is read and before the answer is stamped.
		versions.set(id, freshness.Current{Exists: true, Revision: 2, Hash: "hash-v2"})
	}
	service, _, _ := configured(t, provider)
	service.SetContentVersionReader(versions)

	result, err := service.Chat(context.Background(), ChatRequest{
		ContentID: id,
		Revision:  service.Status().Revision,
		Model:     "fixture",
		Messages:  []ChatMessage{{Role: "user", Content: "原文说了什么？"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := assistantOf(t, result)
	if stored.VersionStatus == freshness.StatusCurrent || freshness.DisplayedAsCurrentlyValid(stored.VersionStatus) {
		t.Fatalf("mid-generation save stored the answer as current: rev=%d hash=%q status=%q", stored.SourceRevision, stored.SourceHash, stored.VersionStatus)
	}
	if stored.SourceRevision == 2 || stored.SourceHash == "hash-v2" {
		t.Fatalf("stamped the post-generation snapshot: %+v", stored)
	}

	reopen, err := service.ChatHistory(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	again := assistantOf(t, reopen)
	if again.VersionStatus == freshness.StatusCurrent || freshness.DisplayedAsCurrentlyValid(again.VersionStatus) {
		t.Fatalf("reopen still current after mid-generation save: rev=%d hash=%q status=%q", again.SourceRevision, again.SourceHash, again.VersionStatus)
	}
}

func TestETClientContextMismatchIsNotCurrent(t *testing.T) {
	const id = "rss:freshness-client-mismatch"
	const saved = "保存的原文句子。"
	versions := newMemoryVersions()
	versions.set(id, freshness.Current{Exists: true, Revision: 4, Hash: "saved-hash"})
	provider := &retrievalChatProvider{answer: "这是客户端上下文上的回答。"}
	service, _, _ := configured(t, provider)
	service.SetContentVersionReader(textVersions{memoryVersions: versions, text: saved})

	result, err := service.Chat(context.Background(), ChatRequest{
		ContentID: id,
		Context:   "这是另一份完全不同的客户端上下文。",
		Revision:  service.Status().Revision,
		Model:     "fixture",
		Messages:  []ChatMessage{{Role: "user", Content: "请回答"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := assistantOf(t, result)
	if stored.VersionStatus == freshness.StatusCurrent || freshness.DisplayedAsCurrentlyValid(stored.VersionStatus) {
		t.Fatalf("mismatched client context stamped current: rev=%d hash=%q status=%q", stored.SourceRevision, stored.SourceHash, stored.VersionStatus)
	}
	if stored.SourceRevision == 4 || stored.SourceHash == "saved-hash" {
		t.Fatalf("mismatched client context was stamped with the saved snapshot: %+v", stored)
	}

	matched, err := service.Chat(context.Background(), ChatRequest{
		ContentID: id,
		Context:   "概览\n\n" + saved,
		Revision:  service.Status().Revision,
		Model:     "fixture",
		Messages:  []ChatMessage{{Role: "user", Content: "第一问"}, {Role: "assistant", Content: stored.Content}, {Role: "user", Content: "请根据原文回答"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	current := matched.Messages[len(matched.Messages)-1]
	if current.VersionStatus != freshness.StatusCurrent || current.SourceRevision != 4 || current.SourceHash != "saved-hash" {
		t.Fatalf("matching client context = rev=%d hash=%q status=%q", current.SourceRevision, current.SourceHash, current.VersionStatus)
	}
}

func TestETUnverifiedClientContextIsNotCurrent(t *testing.T) {
	const id = "rss:freshness-client-unverified"
	versions := newMemoryVersions()
	versions.set(id, freshness.Current{Exists: true, Revision: 3, Hash: "live-hash"})
	provider := &retrievalChatProvider{answer: "无法核对来源的回答。"}
	service, _, _ := configured(t, provider)
	service.SetContentVersionReader(versions)

	result, err := service.Chat(context.Background(), ChatRequest{
		ContentID: id,
		Context:   "客户端自行提供的上下文。",
		Revision:  service.Status().Revision,
		Model:     "fixture",
		Messages:  []ChatMessage{{Role: "user", Content: "请回答"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := assistantOf(t, result)
	if stored.VersionStatus == freshness.StatusCurrent || freshness.DisplayedAsCurrentlyValid(stored.VersionStatus) {
		t.Fatalf("unverifiable client context stamped current: rev=%d hash=%q status=%q", stored.SourceRevision, stored.SourceHash, stored.VersionStatus)
	}
	if stored.SourceRevision == 3 || stored.SourceHash == "live-hash" {
		t.Fatalf("unverifiable client context copied the live snapshot: %+v", stored)
	}
}

func TestRestoreServerEvidenceStopsAtDivergence(t *testing.T) {
	stored := []ChatMessage{
		{Role: "user", Content: "q1"},
		{Role: "assistant", Content: "a1", SourceRevision: 1, SourceHash: "h1", Citations: []citation.Citation{{ID: "c1", Quote: "原文", Status: citation.StatusVerified, ContentRevision: 1, ContentHash: "h1"}}},
		{Role: "user", Content: "q2"},
		{Role: "assistant", Content: "a2", SourceRevision: 2, SourceHash: "h2", Citations: []citation.Citation{{ID: "c2", Quote: "后文", ContentRevision: 2, ContentHash: "h2"}}},
	}
	client := []ChatMessage{
		{Role: "user", Content: "q1", SourceRevision: 99, SourceHash: "forged"},
		{Role: "assistant", Content: "a1", SourceRevision: 99, SourceHash: "forged", Citations: []citation.Citation{{ID: "forged", Quote: "伪造"}}},
		{Role: "user", Content: "edited"},
		{Role: "assistant", Content: "a2", SourceRevision: 99, SourceHash: "forged", Citations: []citation.Citation{{ID: "forged-2", Quote: "伪造2"}}},
	}
	stripUntrustedEvidence(client)
	restoreServerEvidence(client, stored)
	if client[0].SourceRevision != 0 || client[0].SourceHash != "" || len(client[0].Citations) != 0 {
		t.Fatalf("user evidence restored: %+v", client[0])
	}
	if client[1].SourceRevision != 1 || client[1].SourceHash != "h1" || len(client[1].Citations) != 1 || client[1].Citations[0].Quote != "原文" {
		t.Fatalf("matching assistant not restored: %+v", client[1])
	}
	if client[3].SourceRevision != 0 || client[3].SourceHash != "" || len(client[3].Citations) != 0 {
		t.Fatalf("evidence restored after divergence: %+v", client[3])
	}
}

func TestETLegacyMessageWithoutSnapshotIsNotCurrent(t *testing.T) {
	messages := []ChatMessage{{Role: "assistant", Content: "上游旧对话，没有版本快照"}}
	out := applyFreshness(messages, "rss:legacy", freshness.Current{Exists: true, Revision: 3, Hash: "now"}, nil)
	if out[0].VersionStatus != freshness.StatusExpired {
		t.Fatalf("legacy status = %q, want expired", out[0].VersionStatus)
	}
}
