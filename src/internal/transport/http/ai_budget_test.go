package httptransport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"tangentry/internal/adapters/aichat"
	"tangentry/internal/ai"
	"tangentry/internal/subtitles"
	"testing"
	"time"
)

// providerUpstream is a deterministic OpenAI-compatible server that counts
// completion POSTs and returns a fixed body. It never contacts a real provider.
type providerUpstream struct {
	server     *httptest.Server
	postCount  atomic.Int64
	chatBody   func(int) string // n = 1-based completion POST number
	streamBody func(int) string
	mu         sync.Mutex
}

func newProviderUpstream(t *testing.T, chatBody func(int) string, streamBody func(int) string) *providerUpstream {
	t.Helper()
	up := &providerUpstream{chatBody: chatBody, streamBody: streamBody}
	up.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			io, _ := json.Marshal(map[string]any{"data": []any{map[string]string{"id": "fixture"}}})
			w.Write(io)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] == true && up.streamBody != nil {
			n := int(up.postCount.Add(1))
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(up.streamBody(n)))
			return
		}
		n := int(up.postCount.Add(1))
		if up.chatBody != nil {
			_, _ = w.Write([]byte(up.chatBody(n)))
			return
		}
		// Default structured translation reply: echo translations for the
		// segment indices found in the request, with a fixed usage triple.
		var segments []struct {
			Index int `json:"index"`
		}
		if messages, ok := body["messages"].([]any); ok && len(messages) > 0 {
			if last, ok := messages[len(messages)-1].(map[string]any); ok {
				var input struct {
					Segments []struct {
						Index int `json:"index"`
					} `json:"segments"`
				}
				if json.Unmarshal([]byte(fmt.Sprint(last["content"])), &input) == nil {
					segments = input.Segments
				}
			}
		}
		translations := make([]map[string]any, len(segments))
		for i, s := range segments {
			translations[i] = map[string]any{"index": s.Index, "text": fmt.Sprintf("译%d", s.Index)}
		}
		content, _ := json.Marshal(map[string]any{"translations": translations})
		_, _ = fmt.Fprintf(w, `{"choices":[{"finish_reason":"stop","message":{"content":%q}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`, string(content))
	}))
	t.Cleanup(up.server.Close)
	return up
}

func usageChat(text string, prompt, output int) string {
	return fmt.Sprintf(`{"choices":[{"finish_reason":"stop","message":{"content":%q}}],"usage":{"prompt_tokens":%d,"completion_tokens":%d,"total_tokens":%d}}`, text, prompt, output, prompt+output)
}

func budgetHTTPService(t *testing.T, upstream *providerUpstream) http.Handler {
	t.Helper()
	service, err := ai.New(filepath.Join(t.TempDir(), "ai"), ai.NewMemoryResults(), aichat.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	summaryModel := "fixture-summary"
	if _, err := service.Configure(ai.ConfigureInput{BaseURL: upstream.server.URL, Model: "fixture", SummaryModel: &summaryModel, Revision: 1}); err != nil {
		t.Fatal(err)
	}
	return New(nil, nil, nil, Options{AI: service})
}

func translationBody(revision int64, cues int, limitVal *int) []byte {
	in := ai.Input{ContentID: "youtube:jNQXAC9IVRw", Revision: revision, MaxProviderCalls: limitVal, Track: subtitles.Track{Language: "en"}}
	for i := 0; i < cues; i++ {
		in.Track.Cues = append(in.Track.Cues, subtitles.Cue{Start: float64(i), End: float64(i) + 1, Text: fmt.Sprintf("segment %d", i)})
	}
	body, _ := json.Marshal(in)
	return body
}

func ptr(v int) *int { return &v }

// A-T13 / A-T15: invalid limits are rejected with a JSON 400 before any
// provider POST, including the SSE path (no stream headers committed). Boundary
// and omitted limits are accepted with unchanged wire shapes.
func TestAIBudgetInvalidLimitRejectedBeforeProviderWork(t *testing.T) {
	upstream := newProviderUpstream(t, nil, nil)
	h := budgetHTTPService(t, upstream)
	rev := int64(2)
	for _, bad := range []int{0, -1, 101} {
		body := translationBody(rev, 1, ptr(bad))
		w := storageRequest(h, "POST", "/api/ai/subtitles/translate", string(body), "application/json")
		var out apiError
		if w.Code != 400 || json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Code != "invalid_ai_input" {
			t.Fatalf("limit %d not rejected: %d %s", bad, w.Code, w.Body.String())
		}
		// Chat SSE invalid limit must be a plain JSON 400, not a stream.
		chatBody, _ := json.Marshal(ai.ChatRequest{ContentID: "rss:abc", Revision: rev, Model: "fixture", Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}}, MaxProviderCalls: ptr(bad)})
		r := httptest.NewRequest("POST", "http://127.0.0.1/api/ai/chat", bytes.NewReader(chatBody))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "text/event-stream")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != 400 || strings.Contains(rec.Body.String(), "data:") {
			t.Fatalf("SSE limit %d not rejected before headers: %d %q", bad, rec.Code, rec.Body.String())
		}
	}
	if upstream.postCount.Load() != 0 {
		t.Fatalf("invalid limit made %d provider POSTs", upstream.postCount.Load())
	}
	// Omitted limit preserves behavior: no budget field on the wire.
	w := storageRequest(h, "POST", "/api/ai/subtitles/translate", string(translationBody(rev, 1, nil)), "application/json")
	if w.Code != 200 || strings.Contains(w.Body.String(), `"budget"`) {
		t.Fatalf("omitted limit wire shape changed: %d %s", w.Code, w.Body.String())
	}
}

// A-T14 / limited success: exact-limit translation completes (not
// budget_exhausted) and exposes a budget object with known usage.
func TestAIBudgetExactLimitSuccessIncludesReport(t *testing.T) {
	upstream := newProviderUpstream(t, nil, nil)
	h := budgetHTTPService(t, upstream)
	w := storageRequest(h, "POST", "/api/ai/subtitles/translate", string(translationBody(2, 1, ptr(1))), "application/json")
	if w.Code != 200 {
		t.Fatalf("exact-limit translation failed: %d %s", w.Code, w.Body.String())
	}
	var result ai.Result
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result.Budget == nil {
		t.Fatalf("missing budget report: %v %s", err, w.Body.String())
	}
	if result.Budget.Max != 1 || result.Budget.Started != 1 || result.Budget.Status != "completed" || !result.Budget.UsageKnown {
		t.Fatalf("report = %+v", result.Budget)
	}
	if result.Budget.Cumulative.Total == nil || *result.Budget.Cumulative.Total != 5 {
		t.Fatalf("cumulative = %+v", result.Budget.Cumulative)
	}
	if upstream.postCount.Load() != 1 {
		t.Fatalf("provider POSTs = %d", upstream.postCount.Load())
	}
}

// A-T12: a limited translation that exhausts the budget mid-way returns 429
// with budget status/counters and the valid partial progress.
func TestAIBudgetExhaustedMidwayReturnsPartialAndReport(t *testing.T) {
	upstream := newProviderUpstream(t, nil, nil)
	h := budgetHTTPService(t, upstream)
	// 24 article cues => 3 batches of 8; limit 2 commits two batches then stops.
	body := translationBody(2, 24, ptr(2))
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)
	raw["source"] = "article"
	raw["contentId"] = "rss:" + strings.Repeat("a", 64)
	raw["track"].(map[string]any)["language"] = "text-article"
	body, _ = json.Marshal(raw)
	w := storageRequest(h, "POST", "/api/ai/subtitles/translate", string(body), "application/json")
	if w.Code != 429 {
		t.Fatalf("budget exhaustion status: %d %s", w.Code, w.Body.String())
	}
	var out struct {
		Code     string           `json:"code"`
		Budget   *ai.BudgetReport `json:"budget"`
		Progress *aiPartialState  `json:"progress"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Code != "ai_call_budget_exhausted" || out.Budget == nil || out.Budget.Status != "budget_exhausted" || out.Budget.Started != 2 {
		t.Fatalf("budget error = %+v", out)
	}
	if out.Progress == nil || len(out.Progress.Texts) != 16 {
		t.Fatalf("partial progress missing: %+v", out.Progress)
	}
	if upstream.postCount.Load() != 2 {
		t.Fatalf("provider POSTs = %d, want 2", upstream.postCount.Load())
	}
}

// A-T10 over HTTP: a cached translation and a chat clear make zero provider
// POSTs even when a limit is supplied.
func TestAIBudgetCacheAndClearMakeZeroProviderPosts(t *testing.T) {
	upstream := newProviderUpstream(t, nil, nil)
	h := budgetHTTPService(t, upstream)
	if w := storageRequest(h, "POST", "/api/ai/subtitles/translate", string(translationBody(2, 1, ptr(1))), "application/json"); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	before := upstream.postCount.Load()
	if w := storageRequest(h, "POST", "/api/ai/subtitles/translate", string(translationBody(2, 1, ptr(1))), "application/json"); w.Code != 200 {
		t.Fatal("cache hit", w.Code, w.Body)
	}
	clearBody, _ := json.Marshal(ai.ChatRequest{ContentID: "rss:clear", Revision: 2, Model: "fixture", Clear: true, MaxProviderCalls: ptr(1)})
	if w := storageRequest(h, "POST", "/api/ai/chat", string(clearBody), "application/json"); w.Code != 200 {
		t.Fatal("clear", w.Code, w.Body)
	}
	if upstream.postCount.Load() != before {
		t.Fatalf("cache/clear made provider POSTs: %d -> %d", before, upstream.postCount.Load())
	}
}

func sseEvents(body string) []map[string]any {
	var events []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event map[string]any
		if json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event) == nil {
			events = append(events, event)
		}
	}
	return events
}

// A-T12 / A-T18: a stream that emits text but never reports usage ends
// usage_unknown: deltas are forwarded, the error terminal carries the budget
// report, and event: done is never sent.
func TestAIBudgetSSEUnknownUsageAfterDeltas(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"部分\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"回答\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"
	upstream := newProviderUpstream(t, func(int) string { return usageChat("x", 1, 1) }, func(int) string { return stream })
	h := budgetHTTPService(t, upstream)
	chatBody, _ := json.Marshal(ai.ChatRequest{ContentID: "rss:sse-unknown", Revision: 2, Model: "fixture", Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}}, MaxProviderCalls: ptr(1)})
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/ai/chat", bytes.NewReader(chatBody))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("stream status: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if strings.Contains(body, "event: done") {
		t.Fatalf("sent event: done after unknown usage:\n%s", body)
	}
	var text strings.Builder
	var terminal map[string]any
	for _, event := range sseEvents(body) {
		if delta, ok := event["delta"].(string); ok {
			text.WriteString(delta)
		}
		if event["status"] != nil && event["delta"] == nil {
			terminal = event
		}
	}
	if text.String() != "部分回答" {
		t.Fatalf("visible partial text lost: %q", text.String())
	}
	if terminal == nil || terminal["status"] != "usage_unknown" {
		t.Fatalf("missing usage_unknown terminal: %+v", terminal)
	}
	if terminal["budget"] == nil {
		t.Fatalf("terminal missing budget report: %+v", terminal)
	}
	if upstream.postCount.Load() != 1 {
		t.Fatalf("provider POSTs = %d", upstream.postCount.Load())
	}
}

// A-T12: a successful limited stream sends event: done with a completed status
// and a budget report, and never a terminal error.
func TestAIBudgetSSESuccessSendsDoneWithReport(t *testing.T) {
	stream := "data: {\"choices\":[{\"delta\":{\"content\":\"你好\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\n" +
		"data: [DONE]\n\n"
	upstream := newProviderUpstream(t, func(int) string { return usageChat("x", 1, 1) }, func(int) string { return stream })
	h := budgetHTTPService(t, upstream)
	chatBody, _ := json.Marshal(ai.ChatRequest{ContentID: "rss:sse-ok", Revision: 2, Model: "fixture", Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}}, MaxProviderCalls: ptr(1)})
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/ai/chat", bytes.NewReader(chatBody))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	body := rec.Body.String()
	if !strings.Contains(body, "event: done") {
		t.Fatalf("missing done:\n%s", body)
	}
	var done map[string]any
	for _, event := range sseEvents(body) {
		if event["status"] == "completed" {
			done = event
		}
	}
	if done == nil || done["budget"] == nil {
		t.Fatalf("done frame missing report: %s", body)
	}
}

// A-T02/T15 over HTTP: a limited summary returns a budget report on success,
// and an invalid limit is rejected before any POST.
func TestAIBudgetSummaryReportAndInvalidLimit(t *testing.T) {
	// One summary group returns a valid draft; usage carries a known triple.
	upstream := newProviderUpstream(t, func(int) string {
		return `{"choices":[{"finish_reason":"stop","message":{"content":"{\"overview\":\"摘要\",\"points\":[{\"text\":\"要点\",\"indices\":[0]}]}"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`
	}, nil)
	h := budgetHTTPService(t, upstream)
	rev := int64(2)
	summaryBody, _ := json.Marshal(ai.SummaryInput{ContentID: "rss:summary-limited", Source: "article:original", Text: "short source", Revision: rev, MaxProviderCalls: ptr(1)})
	w := storageRequest(h, "POST", "/api/ai/summaries/generate", string(summaryBody), "application/json")
	if w.Code != 200 {
		t.Fatalf("limited summary: %d %s", w.Code, w.Body.String())
	}
	var result ai.SummaryResult
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Budget == nil || result.Budget.Status != "completed" {
		t.Fatalf("summary report missing: %s", w.Body.String())
	}
	before := upstream.postCount.Load()
	badBody, _ := json.Marshal(ai.SummaryInput{ContentID: "rss:summary-badlimit", Source: "article:original", Text: "x", Revision: rev, MaxProviderCalls: ptr(101)})
	if w := storageRequest(h, "POST", "/api/ai/summaries/generate", string(badBody), "application/json"); w.Code != 400 {
		t.Fatalf("invalid summary limit accepted: %d", w.Code)
	}
	if upstream.postCount.Load() != before {
		t.Fatal("invalid summary limit made a POST")
	}
}

// A-T22 over HTTP: a limited Check starts exactly one synthetic POST and
// reports it; a zero-call invalid Check consumes none.
func TestAIBudgetCheckReportAndZeroCall(t *testing.T) {
	upstream := newProviderUpstream(t, nil, nil)
	h := budgetHTTPService(t, upstream)
	rev := int64(2)
	checkBody, _ := json.Marshal(ai.CheckInput{Task: "translation", Revision: rev, MaxProviderCalls: ptr(1)})
	w := storageRequest(h, "POST", "/api/ai/check", string(checkBody), "application/json")
	if w.Code != 200 {
		t.Fatalf("limited check: %d %s", w.Code, w.Body.String())
	}
	var result ai.CheckResult
	if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Budget == nil || result.Budget.Started != 1 {
		t.Fatalf("check report missing: %s", w.Body.String())
	}
	before := upstream.postCount.Load()
	zeroBody, _ := json.Marshal(ai.CheckInput{Task: "nonsense", Revision: rev, MaxProviderCalls: ptr(1)})
	if w := storageRequest(h, "POST", "/api/ai/check", string(zeroBody), "application/json"); w.Code != 400 {
		t.Fatalf("invalid check task: %d", w.Code)
	}
	if upstream.postCount.Load() != before {
		t.Fatal("zero-call check made a POST")
	}
}

// A-T18 handler-level disconnect: a client cancels the real HTTP SSE request
// after the first delta. The handler must not emit event: done, the upstream
// POST stays consumed, and no completed answer is stored. This exercises the
// real handler over a real server, not a recorder.
func TestAIBudgetSSEClientDisconnectNoDone(t *testing.T) {
	var posts atomic.Int64
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/models" {
			_, _ = w.Write([]byte(`{"data":[{"id":"fixture"}]}`))
			return
		}
		posts.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"部分\"}}]}\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		close(started)
		// Block until the client disconnects, then let the handler observe the
		// cancelled context.
		<-r.Context().Done()
	}))
	t.Cleanup(upstream.Close)

	service, err := ai.New(filepath.Join(t.TempDir(), "ai"), ai.NewMemoryResults(), aichat.New())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	if _, err := service.Configure(ai.ConfigureInput{BaseURL: upstream.URL, Model: "fixture", Revision: 1}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(nil, nil, nil, Options{AI: service}))
	t.Cleanup(server.Close)

	body, _ := json.Marshal(ai.ChatRequest{ContentID: "rss:disconnect", Revision: service.Status().Revision, Model: "fixture", Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}}, MaxProviderCalls: ptr(1)})
	ctx, cancel := context.WithCancel(context.Background())
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/ai/chat", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	// Read until we have seen the delta, then cancel to simulate disconnect.
	sawDelta := false
	for !sawDelta {
		line, readErr := reader.ReadString('\n')
		if strings.Contains(line, "\"delta\"") {
			sawDelta = true
		}
		if readErr != nil {
			break
		}
	}
	cancel()
	_ = response.Body.Close()
	// Allow the handler to finish its cancellation path.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if posts.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !sawDelta {
		t.Fatal("client never received the partial delta")
	}
	if posts.Load() != 1 {
		t.Fatalf("started POSTs = %d, want exactly 1 (no retry)", posts.Load())
	}
	history, err := service.ChatHistory(context.Background(), "rss:disconnect")
	if err != nil || len(history.Messages) != 0 {
		t.Fatalf("disconnect stored a completed answer: %v %+v", err, history.Messages)
	}
}

// nonFlushWriter hides http.Flusher so the handler takes its non-SSE fallback.
type nonFlushWriter struct {
	header http.Header
	body   bytes.Buffer
	code   int
}

func (w *nonFlushWriter) Header() http.Header         { return w.header }
func (w *nonFlushWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *nonFlushWriter) WriteHeader(code int)        { w.code = code }

// A-T12 fallback: without a Flusher, a limited budget-exhausted chat must still
// return the additive budget report and partial progress, and the unbudgeted
// path must keep the plain apiError shape (no budget key).
func TestAIBudgetNoFlusherFallbackPreservesReport(t *testing.T) {
	// Article translation with 24 cues and limit 2 exhausts mid-way.
	upstream := newProviderUpstream(t, nil, nil)
	h := budgetHTTPService(t, upstream)
	body := translationBody(2, 24, ptr(2))
	var raw map[string]any
	_ = json.Unmarshal(body, &raw)
	raw["source"] = "article"
	raw["contentId"] = "rss:" + strings.Repeat("c", 64)
	raw["track"].(map[string]any)["language"] = "text-article"
	body, _ = json.Marshal(raw)

	chatBody, _ := json.Marshal(ai.ChatRequest{ContentID: "rss:noflush", Revision: 2, Model: "fixture", Messages: []ai.ChatMessage{{Role: "user", Content: "hi"}}, MaxProviderCalls: ptr(1)})
	r := httptest.NewRequest("POST", "http://127.0.0.1/api/ai/chat", bytes.NewReader(chatBody))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "text/event-stream")
	w := &nonFlushWriter{header: http.Header{}}
	h.ServeHTTP(w, r)
	// Non-stream chat fallback succeeds (real Chat with one POST) and completes.
	if w.code != 200 {
		t.Fatalf("no-flusher limited chat: %d %s", w.code, w.body.String())
	}
	var result ai.ChatResult
	if json.Unmarshal(w.body.Bytes(), &result) != nil || result.Budget == nil || result.Budget.Status != "completed" {
		t.Fatalf("no-flusher fallback lost the budget report: %s", w.body.String())
	}
	// Unbudgeted translation error keeps the plain wire shape (no budget key).
	unbudgeted := translationBody(2, 1, nil)
	var plain map[string]any
	_ = json.Unmarshal(unbudgeted, &plain)
	plain["contentId"] = "rss:" + strings.Repeat("d", 64)
	plain["source"] = "article"
	plain["track"].(map[string]any)["language"] = "text-article"
	// Force a failure: an invalid model triggers ErrInvalid before provider work.
	plain["model"] = "not-a-valid model name"
	encoded, _ := json.Marshal(plain)
	w2 := storageRequest(h, "POST", "/api/ai/subtitles/translate", string(encoded), "application/json")
	if w2.Code != 400 || strings.Contains(w2.Body.String(), `"budget"`) {
		t.Fatalf("unbudgeted error shape changed: %d %s", w2.Code, w2.Body.String())
	}
}
