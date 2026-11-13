package aichat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"tangentry/internal/ai"
)

// Client owns only the provider transport. Prompt construction and response
// decoding live in the task-specific files next to it.
type Client struct{ HTTP *http.Client }

func (c *Client) ListModels(ctx context.Context, config ai.Config) ([]string, error) {
	if !ai.ValidEndpoint(config.BaseURL) {
		return nil, ai.ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(config.BaseURL, "/")+"/models", nil)
	if err != nil {
		return nil, ai.ErrProvider
	}
	if config.Key != "" {
		req.Header.Set("Authorization", "Bearer "+config.Key)
	}
	client := c.HTTP
	if client == nil {
		client = New().HTTP
	}
	safe := *client
	safe.Jar = nil
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := safe.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ai.ErrProvider
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, ai.ErrAuth
	}
	if response.StatusCode == http.StatusNotFound {
		if ai.ValidModelName(config.Model) {
			return []string{config.Model}, nil
		}
		return nil, ai.ErrProvider
	}
	if response.StatusCode != http.StatusOK {
		return nil, ai.ErrProvider
	}
	var envelope struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&envelope) != nil {
		return nil, ai.ErrFormat
	}
	models := make([]string, 0, len(envelope.Data))
	seen := map[string]bool{}
	for _, item := range envelope.Data {
		if ai.ValidModelName(item.ID) && !seen[item.ID] {
			seen[item.ID] = true
			models = append(models, item.ID)
		}
	}
	return models, nil
}

// rejectedRequest classifies only known validation messages. Provider bodies
// may contain credentials or private source text and are never surfaced.
func rejectedRequest(body io.Reader) error {
	var envelope struct {
		Error struct {
			Message string `json:"message"`
			Param   string `json:"param"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if json.NewDecoder(io.LimitReader(body, 4096)).Decode(&envelope) == nil {
		message := strings.ToLower(envelope.Error.Message)
		if strings.Contains(message, "unable to determine target translation language") {
			return ai.ErrTranslationLanguage
		}
		if envelope.Error.Param == "response_format" && (envelope.Error.Code == "unsupported_parameter" || envelope.Error.Code == "unsupported_value") {
			return ai.ErrJSONModeUnsupported
		}
	}
	return ai.ErrRequestRejected
}

func (c *Client) Chat(ctx context.Context, config ai.Config, source string, messages []ai.ChatMessage) (string, error) {
	if !ai.ValidEndpoint(config.BaseURL) || strings.TrimSpace(config.Model) == "" {
		return "", ai.ErrInvalid
	}
	chat := make([]map[string]string, 0, len(messages)+1)
	chat = append(chat, chatSystemMessage(config, source))
	for _, message := range messages {
		chat = append(chat, map[string]string{"role": message.Role, "content": message.Content})
	}
	payload := map[string]any{"model": config.Model, "stream": false, "messages": chat}
	return c.complete(ctx, config, payload)
}

// ChatStream consumes an OpenAI-compatible text/event-stream. A provider that
// closes the stream without any content is treated as a format error so the
// HTTP layer can fall back to the regular completion endpoint.
func (c *Client) ChatStream(ctx context.Context, config ai.Config, source string, messages []ai.ChatMessage, emit func(string) error) error {
	if !ai.ValidEndpoint(config.BaseURL) || strings.TrimSpace(config.Model) == "" {
		return ai.ErrInvalid
	}
	chat := make([]map[string]string, 0, len(messages)+1)
	chat = append(chat, chatSystemMessage(config, source))
	for _, message := range messages {
		chat = append(chat, map[string]string{"role": message.Role, "content": message.Content})
	}
	payload := map[string]any{"model": config.Model, "stream": true, "messages": chat}
	if err := reserveCompletion(ctx); err != nil {
		return err
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(config.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ai.ErrProvider
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream, application/json")
	if config.Key != "" {
		req.Header.Set("Authorization", "Bearer "+config.Key)
	}
	client := c.HTTP
	if client == nil {
		client = New().HTTP
	}
	safe := *client
	safe.Jar = nil
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := safe.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ai.ErrProvider
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if response.StatusCode == http.StatusNotFound {
			return c.responsesStream(ctx, config, chat, source, emit)
		}
		if response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusUnprocessableEntity {
			return rejectedRequest(response.Body)
		}
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			return ai.ErrAuth
		}
		if response.StatusCode == http.StatusTooManyRequests {
			return ai.ErrRate
		}
		return ai.ErrProvider
	}
	usage, err := parseStream(response.Body, emit)
	if err != nil {
		return err
	}
	return reconcileUsage(ctx, usage)
}

func chatSystemMessage(config ai.Config, source string) map[string]string {
	templates := ai.PromptTemplatesFor(config)
	content, err := ai.RenderPromptForProvider(templates.Chat.User, map[string]string{"title": "", "source_type": "当前阅读内容", "content": source, "target_language": "简体中文"})
	if err != nil {
		content = source
	}
	return map[string]string{"role": "system", "content": templates.Chat.System + "\n\n" + content}
}

func (c *Client) responsesStream(ctx context.Context, config ai.Config, chat []map[string]string, source string, emit func(string) error) error {
	if err := reserveCompletion(ctx); err != nil {
		return err
	}
	return c.doResponsesStream(ctx, config, chat, source, emit)
}

// doResponsesStream performs one already-reserved /responses streaming POST.
func (c *Client) doResponsesStream(ctx context.Context, config ai.Config, chat []map[string]string, source string, emit func(string) error) error {
	input := append([]map[string]string{chatSystemMessage(config, source)}, chat[1:]...)
	body, _ := json.Marshal(map[string]any{"model": config.Model, "stream": true, "input": input})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(config.BaseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return ai.ErrProvider
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if config.Key != "" {
		req.Header.Set("Authorization", "Bearer "+config.Key)
	}
	client := c.HTTP
	if client == nil {
		client = New().HTTP
	}
	safe := *client
	safe.Jar = nil
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := safe.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ai.ErrProvider
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ai.ErrProvider
	}
	usage, err := parseResponsesStream(response.Body, emit)
	if err != nil {
		return err
	}
	return reconcileUsage(ctx, usage)
}

// parseResponsesStream consumes Responses SSE events and returns whatever usage
// block was observed. A completed stream without a usage block yields nil so
// the caller reports usage_unknown instead of a known zero.
func parseResponsesStream(body io.Reader, emit func(string) error) (*rawUsage, error) {
	reader := bufio.NewScanner(io.LimitReader(body, (256<<10)+1))
	reader.Buffer(make([]byte, 4096), 64<<10)
	emitted, done := false, false
	var usage *rawUsage
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if value == "[DONE]" {
			done = true
			break
		}
		var event struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Response struct {
				Usage json.RawMessage `json:"usage"`
			} `json:"response"`
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal([]byte(value), &event) != nil {
			return nil, ai.ErrFormat
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				emitted = true
				if err := emit(event.Delta); err != nil {
					return nil, err
				}
			}
		case "response.completed":
			done = true
			if len(event.Response.Usage) > 0 && string(event.Response.Usage) != "null" {
				usage = parseRawUsage(event.Response.Usage)
			} else if len(event.Usage) > 0 && string(event.Usage) != "null" {
				usage = parseRawUsage(event.Usage)
			}
		}
	}
	if err := reader.Err(); err != nil {
		return nil, err
	}
	if !emitted {
		// Only a completed/responses-terminated empty stream is demonstrably
		// empty. An ambiguous EOF must not trigger the fallback.
		if done {
			return nil, ai.ErrEmptyCompletion
		}
		return nil, ai.ErrFormat
	}
	return usage, nil
}

func (c *Client) completeResponses(ctx context.Context, config ai.Config, payload map[string]any) (string, error) {
	if err := reserveCompletion(ctx); err != nil {
		return "", err
	}
	return c.doCompleteResponses(ctx, config, payload)
}

// doCompleteResponses performs one already-reserved /responses POST. It is the
// compatibility target of the 404 fallback; the fallback reserves a fresh slot
// through the completeResponses wrapper before reaching this function.
func (c *Client) doCompleteResponses(ctx context.Context, config ai.Config, payload map[string]any) (string, error) {
	messages, _ := payload["messages"].([]map[string]string)
	body, _ := json.Marshal(map[string]any{"model": payload["model"], "stream": false, "input": messages})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(config.BaseURL, "/")+"/responses", bytes.NewReader(body))
	if err != nil {
		return "", ai.ErrProvider
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if config.Key != "" {
		req.Header.Set("Authorization", "Bearer "+config.Key)
	}
	client := c.HTTP
	if client == nil {
		client = New().HTTP
	}
	safe := *client
	safe.Jar = nil
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := safe.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ai.ErrProvider
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", ai.ErrProvider
	}
	var output struct {
		OutputText string `json:"output_text"`
		Output     []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 256<<10)).Decode(&output) != nil {
		return "", ai.ErrFormat
	}
	if err := reconcileUsage(ctx, parseRawUsage(output.Usage)); err != nil {
		return "", err
	}
	if output.OutputText != "" {
		return output.OutputText, nil
	}
	for _, item := range output.Output {
		for _, content := range item.Content {
			if content.Text != "" {
				return content.Text, nil
			}
		}
	}
	return "", ai.ErrFormat
}

// parseStream is deliberately small and line-oriented: OpenAI-compatible
// servers send one JSON object per data line and terminate with [DONE]. It
// returns the last observed usage block; absent usage yields nil.
func parseStream(body io.Reader, emit func(string) error) (*rawUsage, error) {
	reader := bufio.NewScanner(io.LimitReader(body, (256<<10)+1))
	reader.Buffer(make([]byte, 4096), 64<<10)
	emitted, done := false, false
	var usage *rawUsage
	for reader.Scan() {
		line := strings.TrimSpace(reader.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "[DONE]" {
			done = true
			break
		}
		var event struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage json.RawMessage `json:"usage"`
		}
		if json.Unmarshal([]byte(line), &event) != nil || len(event.Choices) != 1 {
			return nil, ai.ErrFormat
		}
		if len(event.Usage) > 0 && string(event.Usage) != "null" {
			usage = parseRawUsage(event.Usage)
		}
		if value := event.Choices[0].Delta.Content; value != "" {
			emitted = true
			if err := emit(value); err != nil {
				return nil, err
			}
		}
	}
	if err := reader.Err(); err != nil {
		return nil, err
	}
	if !emitted {
		// A clean [DONE] with no content is demonstrably empty, so the bounded
		// non-stream fallback is safe. EOF without [DONE] is an ambiguous
		// truncation and must not trigger a fallback.
		if done {
			return nil, ai.ErrEmptyCompletion
		}
		return nil, ai.ErrFormat
	}
	return usage, nil
}

func New() *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 30 * time.Second
	return &Client{HTTP: &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

func (c *Client) complete(ctx context.Context, config ai.Config, payload map[string]any) (string, error) {
	if err := reserveCompletion(ctx); err != nil {
		return "", err
	}
	return c.doComplete(ctx, config, payload)
}

// doComplete performs one already-reserved /chat/completions POST. A 404 with
// known /responses compatibility consumes a fresh reservation before the
// compatible attempt; a blocked reservation reports budget exhaustion.
func (c *Client) doComplete(ctx context.Context, config ai.Config, payload map[string]any) (string, error) {
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(config.BaseURL, "/")+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", ai.ErrProvider
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if config.Key != "" {
		req.Header.Set("Authorization", "Bearer "+config.Key)
	}
	// Copy the configured client so a supplied cookie jar or redirect policy
	// cannot send unrelated sessions or forward keys to another destination.
	client := c.HTTP
	if client == nil {
		client = New().HTTP
	}
	safe := *client
	safe.Jar = nil
	safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := safe.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "", context.DeadlineExceeded
		}
		return "", ai.ErrProvider
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return c.completeResponses(ctx, config, payload)
	}
	switch response.StatusCode {
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "", rejectedRequest(response.Body)
	case http.StatusUnauthorized, http.StatusForbidden:
		return "", ai.ErrAuth
	case http.StatusTooManyRequests:
		return "", ai.ErrRate
	case http.StatusGatewayTimeout:
		return "", ai.ErrGatewayTimeout
	}
	if response.StatusCode != http.StatusOK {
		return "", ai.ErrProvider
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return "", context.DeadlineExceeded
		}
		return "", ai.ErrProvider
	}
	if len(data) > 256<<10 {
		return "", ai.ErrFormat
	}
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if json.Unmarshal(data, &envelope) != nil || len(envelope.Choices) != 1 {
		return "", ai.ErrFormat
	}
	choice := envelope.Choices[0]
	if choice.Message.Refusal != "" || choice.FinishReason == "content_filter" {
		return "", ai.ErrRefused
	}
	if choice.FinishReason == "length" {
		return "", ai.ErrTruncated
	}
	if choice.FinishReason != "stop" {
		return "", ai.ErrFormat
	}
	if err := reconcileUsage(ctx, parseRawUsage(envelope.Usage)); err != nil {
		return "", err
	}
	return choice.Message.Content, nil
}

func structuredJSON(content string) string {
	text := strings.TrimSpace(content)
	for _, prefix := range []string{"```json\n", "```\n", "```json\r\n", "```\r\n"} {
		if strings.HasPrefix(text, prefix) && strings.HasSuffix(text, "\n```") {
			return strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, prefix), "\n```"))
		}
	}
	return text
}
