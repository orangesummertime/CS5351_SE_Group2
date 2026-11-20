package httptransport

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"tangentry/internal/ai"
	"tangentry/internal/freshness"
)

type httpChatProvider struct {
	answer string
}

func (p httpChatProvider) Translate(_ context.Context, _ ai.Config, _ []ai.Segment) ([]ai.Segment, error) {
	return nil, nil
}

func (p httpChatProvider) Chat(_ context.Context, _ ai.Config, _ string, _ []ai.ChatMessage) (string, error) {
	return p.answer, nil
}

func TestETChatHistoryHTTPReflectsUpdateAndDelete(t *testing.T) {
	provider := httpChatProvider{answer: "这是保存下来的回答。"}
	service, err := ai.New(filepath.Join(t.TempDir(), "ai"), ai.NewMemoryResults(), provider)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	key := "synthetic-only-key"
	status, err := service.Configure(ai.ConfigureInput{BaseURL: "https://ai-fixture.example/v1", Model: "fixture", Key: &key, Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	versions := &httpVersions{current: freshness.Current{Exists: true, Revision: 1, Hash: "v1"}}
	service.SetContentVersionReader(versions)

	const contentID = "rss:http-freshness"
	_, err = service.Chat(context.Background(), ai.ChatRequest{
		ContentID: contentID,
		Revision:  status.Revision,
		Model:     "fixture",
		Messages:  []ai.ChatMessage{{Role: "user", Content: "请概述"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	h := &aiHandler{ai: AIDependencies{AI: service}}
	getHistory := func() ai.ChatResult {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/api/ai/chat?contentId="+contentID, nil)
		w := httptest.NewRecorder()
		h.aiChatHistory(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("history status = %d body=%s", w.Code, w.Body.String())
		}
		var result ai.ChatResult
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}

	first := getHistory()
	if first.Messages[len(first.Messages)-1].VersionStatus != freshness.StatusCurrent {
		t.Fatalf("initial history status = %+v", first.Messages)
	}

	versions.set(freshness.Current{Exists: true, Revision: 2, Hash: "v2"})
	updated := getHistory()
	if updated.Messages[len(updated.Messages)-1].VersionStatus != freshness.StatusExpired {
		t.Fatalf("updated history status = %+v", updated.Messages)
	}
	if freshness.DisplayedAsCurrentlyValid(updated.Messages[len(updated.Messages)-1].VersionStatus) {
		t.Fatal("HTTP reopen after update still displayed as currently valid")
	}

	versions.set(freshness.Current{Exists: false})
	deleted := getHistory()
	if deleted.Messages[len(deleted.Messages)-1].VersionStatus != freshness.StatusUnavailable {
		t.Fatalf("deleted history status = %+v", deleted.Messages)
	}
}

type httpVersions struct {
	current freshness.Current
}

func (h *httpVersions) set(current freshness.Current) { h.current = current }

func (h *httpVersions) CurrentContentVersion(context.Context, string) (freshness.Current, error) {
	return h.current, nil
}
