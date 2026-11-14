package retrieval

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// engineArticle builds a small article whose blocks share the question terms.
func engineArticle(blocks int) Snapshot {
	snap := Snapshot{
		ContentID: "fixture:engine",
		Kind:      SourceArticle,
		Version:   "content-revision:1",
	}
	for i := 0; i < blocks; i++ {
		snap.Blocks = append(snap.Blocks, Block{Index: i, Kind: "paragraph", Text: "答案证据文本段落。"})
	}
	return snap
}

// TestEngineExplicitZeroDisablesEmbedding: the wire request
// embeddingBudget.maxRequests=0 is preserved (zero = off), never replaced by
// the 24/25 default: zero embedder calls and a disabled budget status. Hybrid
// keeps the lexical fallback passages; embedding-only has no evidence path
// and reports the terminal no_match (still zero embedding requests).
func TestEngineExplicitZeroDisablesEmbedding(t *testing.T) {
	for _, tc := range []struct {
		mode       string
		wantErr    bool
		wantStatus string
	}{
		{"hybrid", false, string(SelectionLexicalOnly)},
		{"embedding", true, string(SelectionNoMatch)},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			var req Request
			body := `{"schemaVersion":1,"enabled":true,"mode":"` + tc.mode + `","embeddingBudget":{"maxRequests":0}}`
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Fatal(err)
			}
			if req.EmbeddingBudget.MaxRequests == nil || *req.EmbeddingBudget.MaxRequests != 0 {
				t.Fatalf("explicit zero must decode to a present 0: %+v", req.EmbeddingBudget.MaxRequests)
			}
			src := &fakeSource{snap: engineArticle(4)}
			emb := newFakeEmbedder(BatchingShared)
			engine := &Engine{Source: src, Embedder: emb, Assembler: NewContextAssembler()}
			result, err := engine.Run(context.Background(), RunInput{ContentID: "fixture:engine", Question: "答案证据", Request: req})
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if result.Response.Status != tc.wantStatus {
				t.Fatalf("status = %s, want %s", result.Response.Status, tc.wantStatus)
			}
			if emb.requestCount() != 0 {
				t.Fatalf("explicit zero started %d embedding requests, want 0", emb.requestCount())
			}
			if result.Response.Embedding.BudgetStatus != BudgetDisabled || result.Response.Embedding.Requests != 0 {
				t.Fatalf("embedding summary: %+v", result.Response.Embedding)
			}
			if !tc.wantErr && len(result.Response.Passages) == 0 {
				t.Fatal("lexical fallback must still return passages")
			}
		})
	}
}

// TestEngineOmittedBudgetUsesDefaults: an omitted maxRequests keeps the
// mode-aware default (24 shared / 25 split) and embedding runs.
func TestEngineOmittedBudgetUsesDefaults(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode BatchingMode
		body string
	}{
		{"shared", BatchingShared, `{"schemaVersion":1,"enabled":true,"mode":"hybrid"}`},
		{"split", BatchingSplit, `{"schemaVersion":1,"enabled":true,"mode":"hybrid","embeddingBudget":{"batchingMode":"split"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req Request
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatal(err)
			}
			if req.EmbeddingBudget.MaxRequests != nil {
				t.Fatal("omitted maxRequests must stay nil")
			}
			src := &fakeSource{snap: engineArticle(4)}
			emb := newFakeEmbedder(tc.mode)
			engine := &Engine{Source: src, Embedder: emb, Assembler: NewContextAssembler()}
			result, err := engine.Run(context.Background(), RunInput{ContentID: "fixture:engine", Question: "答案证据", Request: req})
			if err != nil {
				t.Fatal(err)
			}
			if emb.requestCount() == 0 {
				t.Fatal("omitted budget must use the default and embed")
			}
			if result.Response.Embedding.BudgetStatus == BudgetDisabled {
				t.Fatalf("omitted budget must not be disabled: %+v", result.Response.Embedding)
			}
		})
	}
}

// TestEngineExplicitPositiveBudgetRespected: an explicit small limit shapes
// the allocation (slot budget R*B-1) instead of being replaced by the
// default: limit 1 admits at most 7 candidates and one shared batch, where
// the default 24 would embed the full 20-block union in three batches.
func TestEngineExplicitPositiveBudgetRespected(t *testing.T) {
	var req Request
	body := `{"schemaVersion":1,"enabled":true,"mode":"hybrid","embeddingBudget":{"maxRequests":1}}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	snap := engineArticle(0)
	for i := 0; i < 20; i++ {
		snap.Blocks = append(snap.Blocks, Block{Index: i, Kind: "paragraph", Text: "答案证据文本段落编号" + string(rune('A'+i)) + "。"})
	}
	src := &fakeSource{snap: snap}
	emb := newFakeEmbedder(BatchingShared)
	engine := &Engine{Source: src, Embedder: emb, Assembler: NewContextAssembler()}
	result, err := engine.Run(context.Background(), RunInput{ContentID: "fixture:engine", Question: "答案证据", Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if emb.requestCount() != 1 || result.Response.Embedding.Requests != 1 {
		t.Fatalf("explicit limit 1: embedder=%d summary=%d", emb.requestCount(), result.Response.Embedding.Requests)
	}
	if len(result.Response.Passages) == 0 || len(result.Response.Passages) > 7 {
		t.Fatalf("explicit limit 1 must admit at most slotBudget=7 candidates, got %d passages", len(result.Response.Passages))
	}
}

// TestEngineNegativeBudgetRejected: a negative explicit maxRequests is
// invalid_budget before any work.
func TestEngineNegativeBudgetRejected(t *testing.T) {
	var req Request
	if err := json.Unmarshal([]byte(`{"enabled":true,"embeddingBudget":{"maxRequests":-1}}`), &req); err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{snap: engineArticle(2)}
	emb := newFakeEmbedder(BatchingShared)
	engine := &Engine{Source: src, Embedder: emb, Assembler: NewContextAssembler()}
	result, err := engine.Run(context.Background(), RunInput{ContentID: "fixture:engine", Question: "答案证据", Request: req})
	if err == nil || result.Response.Status != string(SelectionInvalidBudget) {
		t.Fatalf("negative budget: status=%s err=%v", result.Response.Status, err)
	}
	if emb.requestCount() != 0 {
		t.Fatal("invalid budget must start zero embedding requests")
	}
}

// TestEngineSetEmbedderRuntimeSwap: saved settings swap the adapter without a
// restart; each run uses the adapter present at its start.
func TestEngineSetEmbedderRuntimeSwap(t *testing.T) {
	src := &fakeSource{snap: engineArticle(2)}
	first := newFakeEmbedder(BatchingShared)
	engine := &Engine{Source: src, Embedder: first, Assembler: NewContextAssembler()}
	req := Request{SchemaVersion: 1, Enabled: true, Mode: ModeHybrid}
	if _, err := engine.Run(context.Background(), RunInput{ContentID: "fixture:engine", Question: "答案证据", Request: req}); err != nil {
		t.Fatal(err)
	}
	if first.requestCount() == 0 {
		t.Fatal("first adapter must be used")
	}
	second := newFakeEmbedder(BatchingShared)
	second.model = "replacement-model"
	engine.SetEmbedder(second)
	result, err := engine.Run(context.Background(), RunInput{ContentID: "fixture:engine", Question: "答案证据", Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if second.requestCount() == 0 || result.Response.Embedding.Model != "replacement-model" {
		t.Fatalf("swapped adapter not used: %+v", result.Response.Embedding)
	}
	engine.SetEmbedder(nil)
	result, err = engine.Run(context.Background(), RunInput{ContentID: "fixture:engine", Question: "答案证据", Request: req})
	if err != nil {
		t.Fatal(err)
	}
	if result.Response.Embedding.BudgetStatus != BudgetDisabled || !strings.Contains(result.Context, "答案证据") {
		t.Fatalf("nil adapter must fall back to lexical: %+v", result.Response.Embedding)
	}
}
