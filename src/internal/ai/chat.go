package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const chatResultKind = "chat"

func chatKey(contentID string) string {
	return hashSource(struct{ Kind, ContentID string }{chatResultKind, contentID})
}

func (s *Service) ChatHistory(ctx context.Context, contentID string) (ChatResult, error) {
	if !contentIDValid(contentID) {
		return ChatResult{}, ErrInvalid
	}
	row, err := s.results.AIResult(ctx, chatResultKind, chatKey(contentID), "")
	if errors.Is(err, ErrNotFound) {
		return ChatResult{ContentID: contentID, Messages: []ChatMessage{}}, nil
	}
	if err != nil {
		return ChatResult{}, ErrStorage
	}
	var messages []ChatMessage
	if len(row.Value) > 512<<10 || json.Unmarshal([]byte(row.Value), &messages) != nil || !utf8ValidChat("", messages) {
		return ChatResult{}, ErrStorage
	}
	return ChatResult{ContentID: contentID, Messages: messages, UpdatedAt: row.CreatedAt}, nil
}

func (s *Service) Chat(ctx context.Context, in ChatRequest) (ChatResult, error) {
	return s.chat(ctx, in, nil)
}

// ChatStream stores the completed answer while forwarding deltas to emit. A
// provider without streaming support transparently emits one final chunk.
func (s *Service) ChatStream(ctx context.Context, in ChatRequest, emit func(string) error) (ChatResult, error) {
	return s.chat(ctx, in, emit)
}

func (s *Service) chat(ctx context.Context, in ChatRequest, emit func(string) error) (ChatResult, error) {
	if !validBudgetLimit(in.MaxProviderCalls) {
		return ChatResult{}, ErrInvalid
	}
	if !contentIDValid(in.ContentID) || !utf8ValidChat(in.Context, in.Messages) || len(in.Messages) > 40 {
		return ChatResult{}, ErrInvalid
	}
	s.mu.Lock()
	c, provider := s.config, s.provider
	s.mu.Unlock()
	if in.Model != "" {
		c.Model = in.Model
	} else {
		c.Model = chatModel(c)
	}
	if !ValidModelName(c.Model) || (len(c.EnabledModels) > 0 && !containsModel(c.EnabledModels, c.Model)) {
		return ChatResult{}, ErrInvalid
	}
	if in.Revision != c.Revision {
		return ChatResult{}, ErrConflict
	}
	chatter, ok := provider.(Chatter)
	if !ok || !public(c).Ready {
		return ChatResult{}, ErrNotReady
	}
	// A budget is attached only for an explicit limit. Cache reads and history
	// clear make no completion POST and therefore consume no slot.
	ctx, err := withBudget(ctx, in.MaxProviderCalls)
	if err != nil {
		return ChatResult{}, err
	}
	key := chatKey(in.ContentID)
	if in.Clear {
		if err := s.results.DeleteAIResult(ctx, chatResultKind, key, ""); err != nil {
			return ChatResult{}, ErrStorage
		}
		return ChatResult{ContentID: in.ContentID, Messages: []ChatMessage{}, Budget: budgetReportFor(ctx)}, nil
	}
	var history []ChatMessage
	if row, err := s.results.AIResult(ctx, chatResultKind, key, ""); err == nil {
		_ = json.Unmarshal([]byte(row.Value), &history)
	} else if !errors.Is(err, ErrNotFound) {
		return ChatResult{}, ErrStorage
	}
	if len(in.Messages) > 0 {
		history = append([]ChatMessage(nil), in.Messages...)
	}
	if len(history) == 0 || history[len(history)-1].Role != "user" {
		return ChatResult{}, ErrInvalid
	}
	var answer string
	if stream, ok := provider.(StreamChatter); ok && emit != nil {
		var builder strings.Builder
		err := stream.ChatStream(ctx, c, in.Context, history, func(chunk string) error {
			builder.WriteString(chunk)
			return emit(chunk)
		})
		if err == nil {
			answer = builder.String()
		} else if builder.Len() == 0 && fallbackAllowed(ctx, err) {
			// A fresh reservation is taken inside the non-stream Chat call.
			answer, err = chatter.Chat(ctx, c, in.Context, history)
			if err != nil {
				if terminal := terminalOutcome(err); terminal != OutcomeActive {
					SetOutcome(ctx, terminal)
					MarkInvocationFailed(ctx, terminal)
					return ChatResult{Budget: budgetReportFor(ctx)}, err
				}
			} else if emitErr := emit(answer); emitErr != nil {
				return ChatResult{}, emitErr
			}
		} else {
			SetOutcome(ctx, terminalOutcome(err))
			MarkInvocationFailed(ctx, terminalOutcome(err))
			return ChatResult{Budget: budgetReportFor(ctx)}, err
		}
	} else {
		var err error
		answer, err = chatter.Chat(ctx, c, in.Context, history)
		if err != nil {
			SetOutcome(ctx, terminalOutcome(err))
			MarkInvocationFailed(ctx, terminalOutcome(err))
			return ChatResult{Budget: budgetReportFor(ctx)}, err
		}
		if emit != nil {
			if err := emit(answer); err != nil {
				return ChatResult{}, err
			}
		}
	}
	if strings.TrimSpace(answer) == "" {
		SetOutcome(ctx, OutcomeProviderError)
		return ChatResult{Budget: budgetReportFor(ctx)}, ErrProvider
	}
	history = append(history, ChatMessage{Role: "assistant", Content: strings.TrimSpace(answer), Model: c.Model})
	value, _ := json.Marshal(history)
	if err := s.results.PutAIResult(ctx, StoredResult{Kind: chatResultKind, SourceHash: key, Value: string(value), Target: "", Model: c.Model, CreatedAt: time.Now().Unix()}); err != nil {
		return ChatResult{}, ErrStorage
	}
	SetOutcome(ctx, OutcomeCompleted)
	return ChatResult{ContentID: in.ContentID, Messages: history, UpdatedAt: time.Now().Unix(), Budget: budgetReportFor(ctx)}, nil
}

func containsModel(models []string, model string) bool {
	for _, value := range models {
		if value == model {
			return true
		}
	}
	return false
}

func utf8ValidChat(contextText string, messages []ChatMessage) bool {
	if len(contextText) > 200000 || strings.ContainsAny(contextText, "\x00") {
		return false
	}
	for _, message := range messages {
		if (message.Role != "user" && message.Role != "assistant") || strings.TrimSpace(message.Content) == "" || len(message.Content) > 12000 {
			return false
		}
	}
	return true
}

func chatContext(content string, history []ChatMessage) string {
	return fmt.Sprintf("内容上下文：\n%s\n\n对话：%v", content, history)
}
