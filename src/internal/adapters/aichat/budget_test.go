package aichat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"tangentry/internal/ai"
	"testing"
)

// usageCompletion builds a chat completion with an explicit usage block. A nil
// usage omits the field entirely.
func usageCompletion(text, finish string, usage map[string]any) string {
	message := map[string]any{"finish_reason": finish, "message": map[string]any{"content": text}}
	envelope := map[string]any{"choices": []any{message}}
	if usage != nil {
		envelope["usage"] = usage
	}
	data, _ := json.Marshal(envelope)
	return string(data)
}

func validUsage(prompt, output int) map[string]any {
	return map[string]any{"prompt_tokens": prompt, "completion_tokens": output, "total_tokens": prompt + output}
}

func budgetContext(t *testing.T, limit int) context.Context {
	t.Helper()
	ctx, err := ai.NewBudgetContext(context.Background(), limit)
	if err != nil {
		t.Fatalf("budget context: %v", err)
	}
	return ctx
}

// A-T01: omitting the limit keeps the wire shape and result unchanged.
func TestAT01ChatOmittedLimitPreservesWireShape(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("request body")
		}
		if _, ok := body["usage"]; ok {
			t.Error("request must not add usage field")
		}
		io.WriteString(w, completion("answer", "stop"))
	}))
	defer server.Close()
	text, err := New().Chat(context.Background(), ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil || text != "answer" || calls.Load() != 1 {
		t.Fatalf("text=%q err=%v calls=%d", text, err, calls.Load())
	}
}

// A-T14: two valid positive usages are tracked exactly, per call and cumulative.
func TestAT14ChatRecordsExactPerCallAndCumulativeUsage(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		io.WriteString(w, usageCompletion("answer", "stop", validUsage(calls, calls*2)))
	}))
	defer server.Close()
	ctx := budgetContext(t, 2)
	client := New()
	config := ai.Config{BaseURL: server.URL, Model: "fixture"}
	if _, err := client.Chat(ctx, config, "ctx", []ai.ChatMessage{{Role: "user", Content: "one"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Chat(ctx, config, "ctx", []ai.ChatMessage{{Role: "user", Content: "two"}}); err != nil {
		t.Fatal(err)
	}
	snapshot := ai.BudgetStatus(ctx)
	if snapshot.Started != 2 || !snapshot.UsageKnown || snapshot.UsageUnknown {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	wantCalls := []ai.Usage{{Prompt: 1, Output: 2, Total: 3}, {Prompt: 2, Output: 4, Total: 6}}
	if fmt.Sprint(snapshot.Calls) != fmt.Sprint(wantCalls) {
		t.Fatalf("calls = %+v", snapshot.Calls)
	}
	if snapshot.Cumulative != (ai.Usage{Prompt: 3, Output: 6, Total: 9}) {
		t.Fatalf("cumulative = %+v", snapshot.Cumulative)
	}
}

// A-T13: invalid limits fail before any provider request.
func TestAT13InvalidLimitFailsBeforeAnyPost(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	for _, limit := range []int{0, -1, 101} {
		if _, err := ai.NewBudgetContext(context.Background(), limit); !errors.Is(err, ai.ErrInvalid) {
			t.Fatalf("limit %d accepted", limit)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid limit made %d requests", calls.Load())
	}
}

// A-T04: a 404 chat fallback needs a second reservation; limit 1 blocks /responses.
func TestAT04NotFoundFallbackNeedsFreshReservation(t *testing.T) {
	var chat, responses atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			chat.Add(1)
			w.WriteHeader(http.StatusNotFound)
		case "/responses":
			responses.Add(1)
			io.WriteString(w, `{"output_text":"兼容","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
		}
	}))
	defer server.Close()
	config := ai.Config{BaseURL: server.URL, Model: "fixture"}
	ctx := budgetContext(t, 1)
	text, err := New().Chat(ctx, config, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
	if !errors.Is(err, ai.ErrBudgetExhausted) || text != "" {
		t.Fatalf("limit 1 allowed fallback: text=%q err=%v", text, err)
	}
	if chat.Load() != 1 || responses.Load() != 0 {
		t.Fatalf("chat=%d responses=%d", chat.Load(), responses.Load())
	}
	if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 1 || snapshot.Terminal != ai.TerminalBudgetExhausted {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// A-T04 (positive): with limit 2 the fallback succeeds using a fresh reservation.
func TestAT04NotFoundFallbackSucceedsWithinBudget(t *testing.T) {
	var chat, responses atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			chat.Add(1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		responses.Add(1)
		io.WriteString(w, `{"output_text":"兼容","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`)
	}))
	defer server.Close()
	ctx := budgetContext(t, 2)
	text, err := New().Chat(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
	if err != nil || text != "兼容" || chat.Load() != 1 || responses.Load() != 1 {
		t.Fatalf("text=%q err=%v chat=%d responses=%d", text, err, chat.Load(), responses.Load())
	}
	if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 2 || snapshot.Cumulative != (ai.Usage{Prompt: 2, Output: 3, Total: 5}) {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// A-T05: an explicit unsupported JSON-mode rejection retries with a fresh
// reservation; limit 1 blocks the retry.
func TestAT05JSONModeRetryNeedsFreshReservation(t *testing.T) {
	for _, limit := range []int{1, 2} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if n == 1 {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"param": "response_format", "code": "unsupported_parameter"}})
					return
				}
				io.WriteString(w, usageCompletion(`{"translations":[{"index":0,"text":"译文"}]}`, "stop", validUsage(3, 4)))
			}))
			defer server.Close()
			ctx := budgetContext(t, limit)
			_, err := New().Translate(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, []ai.Segment{{Index: 0, Text: "source"}})
			if limit == 1 {
				if !errors.Is(err, ai.ErrBudgetExhausted) || calls.Load() != 1 {
					t.Fatalf("limit 1 retried: err=%v calls=%d", err, calls.Load())
				}
				return
			}
			if err != nil || calls.Load() != 2 {
				t.Fatalf("limit 2 retry failed: err=%v calls=%d", err, calls.Load())
			}
			if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 2 || snapshot.Cumulative != (ai.Usage{Prompt: 3, Output: 4, Total: 7}) {
				t.Fatalf("snapshot = %+v", snapshot)
			}
		})
	}
}

// A-T06: an empty stream must consume its slot; the chat.go fallback reserves a
// new slot, so with limit 1 the fallback completion is blocked and never
// reports success.
func TestAT06EmptyStreamFallbackNeedsFreshReservation(t *testing.T) {
	var streams, plain atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["stream"] == true {
				streams.Add(1)
				io.WriteString(w, "data: [DONE]\n\n")
				return
			}
			plain.Add(1)
			io.WriteString(w, usageCompletion("answer", "stop", validUsage(1, 1)))
		case "/responses":
			io.WriteString(w, "data: [DONE]\n\n")
		}
	}))
	defer server.Close()
	ctx := budgetContext(t, 1)
	config := ai.Config{BaseURL: server.URL, Model: "fixture"}
	// The stream attempt itself is a real POST that fails as an empty stream.
	if err := New().ChatStream(ctx, config, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(string) error { return nil }); !errors.Is(err, ai.ErrFormat) {
		t.Fatalf("empty stream classification = %v", err)
	}
	if streams.Load() != 1 {
		t.Fatalf("stream POST count = %d", streams.Load())
	}
	if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 1 {
		t.Fatalf("stream slot not consumed: %+v", snapshot)
	}
	// The service fallback would now need another reservation, which is blocked.
	answer, err := New().Chat(ctx, config, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
	if !errors.Is(err, ai.ErrBudgetExhausted) || answer != "" || plain.Load() != 0 {
		t.Fatalf("fallback not blocked: answer=%q err=%v plain=%d", answer, err, plain.Load())
	}
}

// A-T08: every non-stream response path with invalid usage becomes
// usage_unknown with no known zero and no further POST.
func TestAT08InvalidUsageIsUnknownAcrossResponsePaths(t *testing.T) {
	type path struct {
		name string
		call func(context.Context, *Client, ai.Config) error
		body func(map[string]any) string
	}
	paths := []path{
		{
			name: "chat",
			call: func(ctx context.Context, c *Client, cfg ai.Config) error {
				_, err := c.Chat(ctx, cfg, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
				return err
			},
			body: func(u map[string]any) string { return usageCompletion("answer", "stop", u) },
		},
		{
			name: "responses",
			call: func(ctx context.Context, c *Client, cfg ai.Config) error {
				_, err := c.Chat(ctx, cfg, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
				return err
			},
			body: func(u map[string]any) string {
				envelope := map[string]any{"output_text": "answer"}
				if u != nil {
					envelope["usage"] = u
				}
				data, _ := json.Marshal(envelope)
				return string(data)
			},
		},
		{
			name: "translation",
			call: func(ctx context.Context, c *Client, cfg ai.Config) error {
				_, err := c.Translate(ctx, cfg, []ai.Segment{{Index: 0, Text: "source"}})
				return err
			},
			body: func(u map[string]any) string {
				return usageCompletion(`{"translations":[{"index":0,"text":"译文"}]}`, "stop", u)
			},
		},
	}
	invalid := []struct {
		name  string
		usage map[string]any
	}{
		{"missing", nil},
		{"partial", map[string]any{"prompt_tokens": 1, "completion_tokens": 2}},
		{"negative", map[string]any{"prompt_tokens": -1, "completion_tokens": 2, "total_tokens": 1}},
		{"non-integer", map[string]any{"prompt_tokens": "1", "completion_tokens": 2, "total_tokens": 3}},
		{"inconsistent", map[string]any{"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 9}},
	}
	for _, p := range paths {
		for _, bad := range invalid {
			t.Run(p.name+"/"+bad.name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if p.name == "responses" && r.URL.Path == "/chat/completions" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					io.WriteString(w, p.body(bad.usage))
				}))
				defer server.Close()
				cfg := ai.Config{BaseURL: server.URL, Model: "fixture"}
				client := New()
				ctx := budgetContext(t, 3)
				err := p.call(ctx, client, cfg)
				if !errors.Is(err, ai.ErrUsageUnknown) {
					t.Fatalf("error = %v", err)
				}
				snapshot := ai.BudgetStatus(ctx)
				if snapshot.UsageKnown || !snapshot.UsageUnknown || snapshot.Terminal != ai.TerminalUsageUnknown {
					t.Fatalf("snapshot = %+v", snapshot)
				}
				if len(snapshot.Calls) != 0 || snapshot.Cumulative != (ai.Usage{}) {
					t.Fatalf("unknown became known: %+v", snapshot)
				}
				before := calls.Load()
				if err := p.call(ctx, client, cfg); !errors.Is(err, ai.ErrUsageUnknown) || errors.Is(err, ai.ErrBudgetExhausted) {
					t.Fatalf("usage_unknown must block next POST as usage_unknown, not budget_exhausted: %v", err)
				}
				if calls.Load() != before {
					t.Fatalf("blocked invocation started another POST: %d -> %d", before, calls.Load())
				}
			})
		}
	}
}

// A-T08 (summary): a completed summary with missing usage ends usage_unknown.
func TestAT08SummaryMissingUsageIsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, completion(`{"overview":"摘要","points":[{"text":"要点","indices":[0]}]}`, "stop"))
	}))
	defer server.Close()
	ctx := budgetContext(t, 3)
	_, err := New().Summarize(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, ai.SummaryRequest{Mode: "leaf", Segments: []ai.SummarySegment{{Index: 0, Text: "source"}}})
	if !errors.Is(err, ai.ErrUsageUnknown) {
		t.Fatalf("summary missing usage = %v", err)
	}
	if snapshot := ai.BudgetStatus(ctx); !snapshot.UsageUnknown || snapshot.UsageKnown {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// A-T09: cancellation after start does not refund or retry, and reports a
// distinct cancellation terminal.
func TestAT09CancellationNoRefundOrRetry(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(budgetContext(t, 2))
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := New().Chat(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
		done <- err
	}()
	<-started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation classification = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("cancellation retried: %d", calls.Load())
	}
	if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 1 {
		t.Fatalf("cancellation refunded: %+v", snapshot)
	}
}

// A-T10: model discovery GET is not a completion POST and consumes no slot.
func TestAT10ModelDiscoveryIsNotCounted(t *testing.T) {
	var models atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			models.Add(1)
			io.WriteString(w, `{"data":[{"id":"alpha"}]}`)
			return
		}
	}))
	defer server.Close()
	ctx := budgetContext(t, 1)
	if _, err := New().ListModels(ctx, ai.Config{BaseURL: server.URL, Model: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 0 || snapshot.Remaining != 1 {
		t.Fatalf("model discovery consumed a slot: %+v", snapshot)
	}
	// The single slot is still available for a completion.
	if err := ai.ReserveCompletion(ctx); err != nil {
		t.Fatalf("slot consumed by GET /models: %v", err)
	}
}

// A-T07: two simultaneous reservations for the final slot grant at most one.
func TestAT07ConcurrentLastSlotAtMostOneStarts(t *testing.T) {
	var calls atomic.Int32
	gate := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-gate
		io.WriteString(w, usageCompletion("answer", "stop", validUsage(1, 1)))
	}))
	defer server.Close()
	ctx := budgetContext(t, 1)
	config := ai.Config{BaseURL: server.URL, Model: "fixture"}
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, results[i] = New().Chat(ctx, config, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
		}(i)
	}
	close(gate)
	wg.Wait()
	exhausted := 0
	for _, err := range results {
		if errors.Is(err, ai.ErrBudgetExhausted) {
			exhausted++
		}
	}
	if exhausted != 1 || calls.Load() > 1 {
		t.Fatalf("exhausted=%d calls=%d results=%v", exhausted, calls.Load(), results)
	}
}

// A-T16: two isolated invocations with limit 1 each may both start one POST.
func TestAT16IsolatedInvocationsEachMayStart(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.WriteString(w, usageCompletion("answer", "stop", validUsage(1, 1)))
	}))
	defer server.Close()
	config := ai.Config{BaseURL: server.URL, Model: "fixture"}
	first, second := budgetContext(t, 1), budgetContext(t, 1)
	if _, err := New().Chat(first, config, "ctx", []ai.ChatMessage{{Role: "user", Content: "a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := New().Chat(second, config, "ctx", []ai.ChatMessage{{Role: "user", Content: "b"}}); err != nil {
		t.Fatalf("isolated invocation blocked: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
}

// A-T17: transport failures consume a slot and are never silently retried.
func TestAT17TransportFailureConsumesSlotNoRetry(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	ctx := budgetContext(t, 3)
	_, err := New().Chat(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
	if !errors.Is(err, ai.ErrProvider) || calls.Load() != 1 {
		t.Fatalf("5xx retried or misclassified: err=%v calls=%d", err, calls.Load())
	}
	if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 1 {
		t.Fatalf("slot not consumed on failure: %+v", snapshot)
	}
}

// Streaming usage parsing: chat final envelope with usage reconciles.
func TestChatStreamUsageFromFinalEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}}\n\n"+
			"data: [DONE]\n\n")
	}))
	defer server.Close()
	ctx := budgetContext(t, 2)
	var got strings.Builder
	err := New().ChatStream(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(s string) error { got.WriteString(s); return nil })
	if err != nil || got.String() != "你好" {
		t.Fatalf("stream = %q err=%v", got.String(), err)
	}
	if snapshot := ai.BudgetStatus(ctx); snapshot.Cumulative != (ai.Usage{Prompt: 4, Output: 6, Total: 10}) || !snapshot.UsageKnown {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// Streaming chat without any usage must end usage_unknown, not a false success.
func TestChatStreamCompletedTextWithoutUsageIsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"+
			"data: [DONE]\n\n")
	}))
	defer server.Close()
	ctx := budgetContext(t, 2)
	var got strings.Builder
	err := New().ChatStream(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(s string) error { got.WriteString(s); return nil })
	if !errors.Is(err, ai.ErrUsageUnknown) {
		t.Fatalf("missing stream usage = %v", err)
	}
	if snapshot := ai.BudgetStatus(ctx); !snapshot.UsageUnknown || snapshot.UsageKnown {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// Streaming Responses usage from response.completed reconciles.
func TestResponsesStreamUsageFromCompletedEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			w.WriteHeader(http.StatusNotFound)
		case "/responses":
			io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n"+
				"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n")
		}
	}))
	defer server.Close()
	ctx := budgetContext(t, 3)
	var got strings.Builder
	err := New().ChatStream(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(s string) error { got.WriteString(s); return nil })
	if err != nil || got.String() != "hi" {
		t.Fatalf("responses stream = %q err=%v", got.String(), err)
	}
	if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 2 || snapshot.Cumulative != (ai.Usage{Prompt: 1, Output: 2, Total: 3}) {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// The stream request format must keep stream:true and no usage request fields.
func TestChatStreamPreservesRequestFormat(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	if err := New().ChatStream(context.Background(), ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if body["stream"] != true {
		t.Fatalf("stream request changed: %+v", body)
	}
	for _, forbidden := range []string{"stream_options", "include_usage"} {
		if _, ok := body[forbidden]; ok {
			t.Fatalf("stream request added %s", forbidden)
		}
	}
}

// A-T08 streaming: every invalid streaming usage shape must end usage_unknown
// for both the chat and Responses SSE protocols, never a known zero, and must
// block further starts in the same invocation.
func TestAT08StreamingInvalidUsageIsUnknown(t *testing.T) {
	usage := func(block string) map[string]string {
		return map[string]string{
			"chat":      "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]" + block + "}\n\ndata: [DONE]\n\n",
			"responses": "data: {\"type\":\"response.output_text.delta\",\"delta\":\"x\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{" + strings.TrimPrefix(block, ",") + "}}\n\n",
		}
	}
	cases := []struct {
		name  string
		block string
	}{
		{"missing", ""},
		{"partial", `,"usage":{"prompt_tokens":1,"completion_tokens":2}`},
		{"negative", `,"usage":{"prompt_tokens":-1,"completion_tokens":2,"total_tokens":1}`},
		{"non-integer", `,"usage":{"prompt_tokens":"1","completion_tokens":2,"total_tokens":3}`},
		{"inconsistent", `,"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":9}`},
	}
	for _, tc := range cases {
		for protocol, streamBody := range usage(tc.block) {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if protocol == "responses" && r.URL.Path == "/chat/completions" {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					io.WriteString(w, streamBody)
				}))
				defer server.Close()
				cfg := ai.Config{BaseURL: server.URL, Model: "fixture"}
				ctx := budgetContext(t, 3)
				call := func() error {
					return New().ChatStream(ctx, cfg, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(string) error { return nil })
				}
				if err := call(); !errors.Is(err, ai.ErrUsageUnknown) {
					t.Fatalf("stream invalid usage must be usage_unknown: %v", err)
				}
				snapshot := ai.BudgetStatus(ctx)
				if !snapshot.UsageUnknown || snapshot.UsageKnown || len(snapshot.Calls) != 0 {
					t.Fatalf("snapshot = %+v", snapshot)
				}
				before := calls.Load()
				if err := call(); !errors.Is(err, ai.ErrUsageUnknown) || errors.Is(err, ai.ErrBudgetExhausted) {
					t.Fatalf("blocked start after unknown must report usage_unknown: %v", err)
				}
				if calls.Load() != before {
					t.Fatalf("started a POST after unknown: %d -> %d", before, calls.Load())
				}
			})
		}
	}
}

// A-T17 transport/status classification matrix from the ADR: every non-200
// completion POST consumes one slot and is never silently retried within the
// invocation. 404 is a compatibility fallback (covered separately); here the
// /responses target also fails, so a single logical attempt ends in failure.
func TestAT17StatusClassificationMatrix(t *testing.T) {
	cases := []struct {
		status   int
		expected error
	}{
		{http.StatusBadRequest, ai.ErrRequestRejected},
		{http.StatusUnauthorized, ai.ErrAuth},
		{http.StatusForbidden, ai.ErrAuth},
		{http.StatusTooManyRequests, ai.ErrRate},
		{http.StatusInternalServerError, ai.ErrProvider},
		{http.StatusBadGateway, ai.ErrProvider},
		{http.StatusGatewayTimeout, ai.ErrGatewayTimeout},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				io.WriteString(w, `{"error":{"message":"synthetic-secret-in-upstream-error"}}`)
			}))
			defer server.Close()
			ctx := budgetContext(t, 5)
			_, err := New().Chat(ctx, ai.Config{BaseURL: server.URL, Model: "fixture", Key: "synthetic-test-key"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
			if !errors.Is(err, tc.expected) {
				t.Fatalf("status %d: %v", tc.status, err)
			}
			if strings.Contains(err.Error(), "synthetic-secret") {
				t.Fatal("upstream body leaked")
			}
			if calls.Load() != 1 {
				t.Fatalf("status %d retried: calls=%d", tc.status, calls.Load())
			}
			if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 1 {
				t.Fatalf("status %d did not consume exactly one slot: %+v", tc.status, snapshot)
			}
		})
	}
}

// A-T17 timeout: a transport timeout consumes one slot, is classified as
// deadline, and is never retried.
func TestAT17TimeoutConsumesSlotNoRetry(t *testing.T) {
	var calls atomic.Int32
	client := &Client{HTTP: &http.Client{Transport: chatRoundTripper(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, fmt.Errorf("synthetic-private-timeout: %w", context.DeadlineExceeded)
	})}}
	ctx := budgetContext(t, 5)
	_, err := client.Chat(ctx, ai.Config{BaseURL: "https://ai-fixture.example/v1", Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}})
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "synthetic-private") {
		t.Fatalf("timeout classification: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("timeout retried: calls=%d", calls.Load())
	}
	if snapshot := ai.BudgetStatus(ctx); snapshot.Started != 1 {
		t.Fatalf("timeout did not consume a slot: %+v", snapshot)
	}
}

// Finding A (A-T18): a limited chat stream that emits content plus a valid
// usage block but ends with a premature EOF (no [DONE]) is truncated, not
// completed. It must report a non-empty-stream error, never be seen as success,
// preserve the forwarded content and leave the one started slot consumed.
func TestAT18ChatStreamContentUsageEOFIsTruncated(t *testing.T) {
	var streams atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		streams.Add(1)
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"部分\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"回答\"}}]}\n\n"+
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}}\n\n")
	}))
	defer server.Close()
	ctx := budgetContext(t, 3)
	var got strings.Builder
	err := New().ChatStream(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(s string) error { got.WriteString(s); return nil })
	if !errors.Is(err, ai.ErrTruncated) || !errors.Is(err, ai.ErrFormat) {
		t.Fatalf("content+usage+EOF must be truncated, got %v", err)
	}
	if errors.Is(err, ai.ErrEmptyCompletion) {
		t.Fatalf("non-empty truncated stream reported as empty: %v", err)
	}
	if got.String() != "部分回答" {
		t.Fatalf("partial content lost: %q", got.String())
	}
	snapshot := ai.BudgetStatus(ctx)
	if snapshot.Started != 1 {
		t.Fatalf("started=%d, want 1", snapshot.Started)
	}
	if streams.Load() != 1 {
		t.Fatalf("stream POSTs = %d, want exactly 1", streams.Load())
	}
}

// Finding A (A-T18): the same premature-EOF truncation applies to the Responses
// SSE protocol when content is delivered without response.completed.
func TestAT18ResponsesStreamContentUsageEOFIsTruncated(t *testing.T) {
	var got strings.Builder
	input := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
		"data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":2,\"total_tokens\":3}}}\n\n"
	// Drop the completion event to simulate truncation after the delta.
	truncated := "data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n"
	_ = input
	if _, err := parseResponsesStream(strings.NewReader(truncated), func(v string) error { _, e := got.WriteString(v); return e }, true); !errors.Is(err, ai.ErrTruncated) {
		t.Fatalf("responses content+EOF must be truncated, got %v", err)
	}
	if got.String() != "hi" {
		t.Fatalf("partial content lost: %q", got.String())
	}
	if _, err := parseResponsesStream(strings.NewReader(input), func(string) error { return nil }, true); err != nil {
		t.Fatalf("completed responses stream rejected: %v", err)
	}
}

// Finding A: an unbudgeted invocation keeps upstream compatibility. A stream
// with content but no [DONE] is still accepted because strict completion is a
// limited-invocation rule.
func TestStreamEOFUnbudgetedCompatibility(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"
	if _, err := parseStream(strings.NewReader(input), func(string) error { return nil }, false); err != nil {
		t.Fatalf("unbudgeted EOF rejected: %v", err)
	}
	if _, err := parseStream(strings.NewReader(input), func(string) error { return nil }, true); !errors.Is(err, ai.ErrTruncated) {
		t.Fatalf("limited EOF accepted: %v", err)
	}
}

// Finding B: a genuine usage-only chunk with an empty choices array is valid
// and must reconcile a known total even though it carries no content.
func TestStreamChoicesEmptyUsageOnlyChunkIsAccepted(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}}\n\n" +
		"data: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, input)
	}))
	defer server.Close()
	ctx := budgetContext(t, 2)
	var got strings.Builder
	err := New().ChatStream(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(s string) error { got.WriteString(s); return nil })
	if err != nil || got.String() != "你好" {
		t.Fatalf("usage-only chunk stream = %q err=%v", got.String(), err)
	}
	snapshot := ai.BudgetStatus(ctx)
	if snapshot.Cumulative != (ai.Usage{Prompt: 4, Output: 6, Total: 10}) || !snapshot.UsageKnown {
		t.Fatalf("usage-only chunk not reconciled: %+v", snapshot)
	}
}

// Finding B: malformed usage inside a usage-only chunk is still usage_unknown
// and blocks further starts; multiple choices remain a format error.
func TestStreamChoicesEmptyMalformedUsageIsUnknown(t *testing.T) {
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6}}\n\n" +
		"data: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, input)
	}))
	defer server.Close()
	ctx := budgetContext(t, 2)
	if err := New().ChatStream(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(string) error { return nil }); !errors.Is(err, ai.ErrUsageUnknown) {
		t.Fatalf("partial usage-only usage must be unknown: %v", err)
	}
	if snapshot := ai.BudgetStatus(ctx); !snapshot.UsageUnknown || snapshot.UsageKnown {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if err := New().ChatStream(ctx, ai.Config{BaseURL: server.URL, Model: "fixture"}, "ctx", []ai.ChatMessage{{Role: "user", Content: "hi"}}, func(string) error { return nil }); !errors.Is(err, ai.ErrUsageUnknown) {
		t.Fatalf("start after unknown not blocked: %v", err)
	}
	multi := "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}},{\"delta\":{\"content\":\"b\"}}]}\n\ndata: [DONE]\n\n"
	if _, err := parseStream(strings.NewReader(multi), func(string) error { return nil }, true); !errors.Is(err, ai.ErrFormat) || errors.Is(err, ai.ErrTruncated) {
		t.Fatalf("multiple choices must stay a format error: %v", err)
	}
}
