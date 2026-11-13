package ai

import (
	"context"
	"errors"
	"testing"
)

// streamFallbackProvider reports a demonstrably empty stream (ErrEmptyCompletion), so
// the service tries the non-stream fallback, and counts each started POST.
type streamFallbackProvider struct {
	translatorFunc
	streamCalls int
	chatCalls   int
}

func (p *streamFallbackProvider) Chat(ctx context.Context, c Config, source string, messages []ChatMessage) (string, error) {
	if err := ReserveCompletion(ctx); err != nil {
		return "", err
	}
	p.chatCalls++
	return "fallback answer", nil
}

func (p *streamFallbackProvider) ChatStream(ctx context.Context, c Config, source string, messages []ChatMessage, emit func(string) error) error {
	if err := ReserveCompletion(ctx); err != nil {
		return err
	}
	p.streamCalls++
	return ErrEmptyCompletion
}

// A-T06: the service empty-stream fallback must reserve a fresh slot. With limit
// 1 the stream attempt consumes the only slot and the fallback is blocked, so no
// success result is produced.
func TestAT06ServiceStreamFallbackNeedsFreshReservation(t *testing.T) {
	provider := &streamFallbackProvider{}
	service, _, _ := configured(t, provider)
	ctx, err := NewBudgetContext(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	request := ChatRequest{ContentID: "rss:at06", Revision: service.Status().Revision, Model: "fixture", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}
	result, err := service.ChatStream(ctx, request, func(string) error { return nil })
	if !errors.Is(err, ErrBudgetExhausted) {
		t.Fatalf("fallback not blocked: err=%v result=%+v", err, result)
	}
	if provider.streamCalls != 1 || provider.chatCalls != 0 {
		t.Fatalf("stream=%d chat=%d", provider.streamCalls, provider.chatCalls)
	}
	if snapshot := BudgetStatus(ctx); snapshot.Started != 1 || snapshot.Terminal != TerminalBudgetExhausted {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

// A-T06 (positive): with a second slot the fallback completes and stores the
// answer.
func TestAT06ServiceStreamFallbackSucceedsWithSecondSlot(t *testing.T) {
	provider := &streamFallbackProvider{}
	service, _, _ := configured(t, provider)
	ctx, err := NewBudgetContext(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	request := ChatRequest{ContentID: "rss:at06-ok", Revision: service.Status().Revision, Model: "fixture", Messages: []ChatMessage{{Role: "user", Content: "hi"}}}
	result, err := service.ChatStream(ctx, request, func(string) error { return nil })
	if err != nil || len(result.Messages) != 2 || result.Messages[1].Content != "fallback answer" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if provider.streamCalls != 1 || provider.chatCalls != 1 {
		t.Fatalf("stream=%d chat=%d", provider.streamCalls, provider.chatCalls)
	}
	if snapshot := BudgetStatus(ctx); snapshot.Started != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}
