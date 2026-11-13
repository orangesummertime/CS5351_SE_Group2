package aichat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"tangentry/internal/ai"
	"testing"
)

func TestListModelsAndResponsesFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"id":"alpha"},{"id":"alpha"},{"id":"beta"}]}`)
		case "/v1/chat/completions":
			w.WriteHeader(http.StatusNotFound)
		case "/v1/responses":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"output_text": "兼容回复"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	client := New()
	models, err := client.ListModels(context.Background(), ai.Config{BaseURL: server.URL + "/v1", Model: "alpha"})
	if err != nil || strings.Join(models, ",") != "alpha,beta" {
		t.Fatalf("models=%v err=%v", models, err)
	}
	text, err := client.Chat(context.Background(), ai.Config{BaseURL: server.URL + "/v1", Model: "alpha"}, "背景", []ai.ChatMessage{{Role: "user", Content: "问题"}})
	if err != nil || text != "兼容回复" {
		t.Fatalf("responses fallback=%q err=%v", text, err)
	}
}

func TestParseStream(t *testing.T) {
	var got strings.Builder
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"，世界\"}}]}\n\n" + "data: [DONE]\n\n"
	if _, err := parseStream(strings.NewReader(input), func(value string) error { _, err := got.WriteString(value); return err }); err != nil {
		t.Fatal(err)
	}
	if got.String() != "你好，世界" {
		t.Fatalf("stream chunks = %q", got.String())
	}
}

func TestParseStreamRejectsEmpty(t *testing.T) {
	if _, err := parseStream(strings.NewReader("data: [DONE]\n\n"), func(string) error { return nil }); err == nil {
		t.Fatal("empty stream accepted")
	}
}
