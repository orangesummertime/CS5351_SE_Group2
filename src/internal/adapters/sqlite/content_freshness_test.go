package sqlite

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"tangentry/internal/ai"
	"tangentry/internal/freshness"
)

func TestETStoreCurrentContentVersionUpdateDeleteAndConcurrent(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "freshness.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	id := "bilibili:BV1freshness"

	if current, err := store.CurrentContentVersion(ctx, id); err != nil || current.Exists {
		t.Fatalf("missing content: %+v %v", current, err)
	}

	first, err := store.Save(ctx, versionTestItem(id, "第一版原文。"))
	if err != nil {
		t.Fatal(err)
	}
	current, err := store.CurrentContentVersion(ctx, id)
	if err != nil || !current.Exists || current.Revision != first.ContentRevision || current.Hash != first.ContentHash {
		t.Fatalf("after save: %+v %v (item=%+v)", current, err, first)
	}

	second, err := store.Save(ctx, versionTestItem(id, "第二版原文，已经改写。"))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.CurrentContentVersion(ctx, id)
	if err != nil || updated.Revision != second.ContentRevision || updated.Hash != second.ContentHash {
		t.Fatalf("after update: %+v %v", updated, err)
	}
	if updated.Revision <= current.Revision || updated.Hash == current.Hash {
		t.Fatalf("update did not change revision/hash: before=%+v after=%+v", current, updated)
	}
	status := freshness.Assess(freshness.Snapshot{ContentID: id, ContentRevision: first.ContentRevision, ContentHash: first.ContentHash}, updated, nil)
	if status != freshness.StatusExpired || freshness.DisplayedAsCurrentlyValid(status) {
		t.Fatalf("updated row still current: %q", status)
	}

	if err := store.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	missing, err := store.CurrentContentVersion(ctx, id)
	if err != nil || missing.Exists {
		t.Fatalf("after delete: %+v %v", missing, err)
	}
	status = freshness.Assess(freshness.Snapshot{ContentID: id, ContentRevision: second.ContentRevision, ContentHash: second.ContentHash}, missing, nil)
	if status != freshness.StatusUnavailable {
		t.Fatalf("deleted row status = %q", status)
	}
}

func TestETServiceWithSQLiteReopenAndConcurrentSave(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "freshness-chat.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	id := "bilibili:BV1chatFresh"

	item, err := store.Save(ctx, versionTestItem(id, "可供引用的原文句子。"))
	if err != nil {
		t.Fatal(err)
	}

	provider := &sqliteChatProvider{answer: "这是根据原文的回答。"}
	service, err := ai.New(filepath.Join(t.TempDir(), "ai"), store, provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	key := "synthetic-only-key"
	status, err := service.Configure(ai.ConfigureInput{BaseURL: "https://ai-fixture.example/v1", Model: "fixture", Key: &key, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	service.SetContentVersionReader(store)

	created, err := service.Chat(ctx, ai.ChatRequest{
		ContentID: id,
		Revision:  status.Revision,
		Model:     "fixture",
		Messages:  []ai.ChatMessage{{Role: "user", Content: "这句话说了什么？"}},
	})
	if err != nil || len(created.Messages) < 2 {
		t.Fatalf("chat: %+v %v", created, err)
	}
	if created.Messages[1].VersionStatus != freshness.StatusCurrent || created.Messages[1].SourceRevision != item.ContentRevision {
		t.Fatalf("stamped message: %+v (item rev %d)", created.Messages[1], item.ContentRevision)
	}

	// Concurrent save versus history reads; the post-join reopen must be expired.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = store.Save(ctx, versionTestItem(id, "并发更新后的原文。"))
	}()
	go func() {
		defer wg.Done()
		_, _ = service.ChatHistory(ctx, id)
	}()
	wg.Wait()

	history, err := service.ChatHistory(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	got := history.Messages[len(history.Messages)-1].VersionStatus
	if got != freshness.StatusExpired {
		t.Fatalf("reopen after concurrent save = %q, want expired", got)
	}
	if freshness.DisplayedAsCurrentlyValid(got) {
		t.Fatal("old chat answer still displayed as currently valid")
	}
}

// sqliteChatProvider is a minimal chatter for store-backed tests.
type sqliteChatProvider struct {
	answer string
}

func (p *sqliteChatProvider) Translate(context.Context, ai.Config, []ai.Segment) ([]ai.Segment, error) {
	return nil, nil
}

func (p *sqliteChatProvider) Chat(context.Context, ai.Config, string, []ai.ChatMessage) (string, error) {
	return p.answer, nil
}
