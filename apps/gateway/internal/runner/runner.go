package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"copaw-next/apps/gateway/internal/domain"
	"copaw-next/apps/gateway/internal/provider"
)

const (
	ProviderDemo   = "demo"
	ProviderOpenAI = "openai"

	defaultOpenAIBaseURL = "https://api.openai.com/v1"

	ErrorCodeProviderNotConfigured = "provider_not_configured"
	ErrorCodeProviderNotSupported  = "provider_not_supported"
	ErrorCodeProviderRequestFailed = "provider_request_failed"
	ErrorCodeProviderInvalidReply  = "provider_invalid_reply"
)

type RunnerError struct {
	Code    string
	Message string
	Err     error
}

func (e *RunnerError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

func (e *RunnerError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type GenerateConfig struct {
	ProviderID string
	Model      string
	APIKey     string
	BaseURL    string
	AdapterID  string
	Headers    map[string]string
	TimeoutMS  int
}

type ToolDefinition struct {
	Name        string
	Description string
	Parameters  map[string]interface{}
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]interface{}
}

type TurnResult struct {
	Text      string
	ToolCalls []ToolCall
}

type ProviderAdapter interface {
	ID() string
	GenerateTurn(ctx context.Context, req domain.AgentProcessRequest, cfg GenerateConfig, tools []ToolDefinition, runner *Runner) (TurnResult, error)
}

type Runner struct {
	httpClient *http.Client
	adapters   map[string]ProviderAdapter
}

func New() *Runner {
	return NewWithHTTPClient(&http.Client{Timeout: 30 * time.Second})
}

func NewWithHTTPClient(client *http.Client) *Runner {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	r := &Runner{
		httpClient: client,
		adapters:   map[string]ProviderAdapter{},
	}
	r.registerAdapter(&demoAdapter{})
	r.registerAdapter(&openAICompatibleAdapter{})
	return r
}

func (r *Runner) registerAdapter(adapter ProviderAdapter) {
	if adapter == nil {
		return
	}
	id := strings.TrimSpace(adapter.ID())
	if id == "" {
		return
	}
	r.adapters[id] = adapter
}

func (r *Runner) GenerateTurn(ctx context.Context, req domain.AgentProcessRequest, cfg GenerateConfig, tools []ToolDefinition) (TurnResult, error) {
	providerID := strings.ToLower(strings.TrimSpace(cfg.ProviderID))
	if providerID == "" {
		providerID = ProviderDemo
	}

	adapterID := strings.TrimSpace(cfg.AdapterID)
	if adapterID == "" {
		adapterID = defaultAdapterForProvider(providerID)
	}
	if adapterID == "" {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderNotSupported,
			Message: fmt.Sprintf("provider %q is not supported", providerID),
		}
	}

	if adapterID != provider.AdapterDemo && strings.TrimSpace(cfg.Model) == "" {
		return TurnResult{}, &RunnerError{Code: ErrorCodeProviderNotConfigured, Message: "model is required for active provider"}
	}

	adapter, ok := r.adapters[adapterID]
	if !ok {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderNotSupported,
			Message: fmt.Sprintf("adapter %q is not supported", adapterID),
		}
	}
	return adapter.GenerateTurn(ctx, req, cfg, tools, r)
}

func (r *Runner) GenerateReply(ctx context.Context, req domain.AgentProcessRequest, cfg GenerateConfig) (string, error) {
	turn, err := r.GenerateTurn(ctx, req, cfg, nil)
	if err != nil {
		return "", err
	}
	if len(turn.ToolCalls) > 0 {
		return "", &RunnerError{Code: ErrorCodeProviderInvalidReply, Message: "provider response contains tool calls but tool support is disabled"}
	}
	text := strings.TrimSpace(turn.Text)
	if text == "" {
		return "", &RunnerError{Code: ErrorCodeProviderInvalidReply, Message: "provider response has empty content"}
	}
	return text, nil
}

type demoAdapter struct{}

func (a *demoAdapter) ID() string {
	return provider.AdapterDemo
}

func (a *demoAdapter) GenerateTurn(_ context.Context, req domain.AgentProcessRequest, _ GenerateConfig, _ []ToolDefinition, _ *Runner) (TurnResult, error) {
	return TurnResult{Text: generateDemoReply(req)}, nil
}

type openAICompatibleAdapter struct{}

func (a *openAICompatibleAdapter) ID() string {
	return provider.AdapterOpenAICompatible
}

func (a *openAICompatibleAdapter) GenerateTurn(ctx context.Context, req domain.AgentProcessRequest, cfg GenerateConfig, tools []ToolDefinition, runner *Runner) (TurnResult, error) {
	return runner.generateOpenAICompatibleTurn(ctx, req, cfg, tools)
}

func defaultAdapterForProvider(providerID string) string {
	switch providerID {
	case "", ProviderDemo:
		return provider.AdapterDemo
	case ProviderOpenAI:
		return provider.AdapterOpenAICompatible
	default:
		return ""
	}
}

func generateDemoReply(req domain.AgentProcessRequest) string {
	parts := make([]string, 0, len(req.Input))
	for _, msg := range req.Input {
		if msg.Role != "user" {
			continue
		}
		for _, c := range msg.Content {
			if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
				parts = append(parts, strings.TrimSpace(c.Text))
			}
		}
	}
	if len(parts) == 0 {
		return "Echo: (empty input)"
	}
	return "Echo: " + strings.Join(parts, " ")
}

func (r *Runner) generateOpenAICompatibleTurn(ctx context.Context, req domain.AgentProcessRequest, cfg GenerateConfig, tools []ToolDefinition) (TurnResult, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return TurnResult{}, &RunnerError{Code: ErrorCodeProviderNotConfigured, Message: "provider api_key is required"}
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}

	payload := openAIChatRequest{
		Model:    cfg.Model,
		Messages: toOpenAIMessages(req.Input),
		Tools:    toOpenAITools(tools),
	}
	if len(payload.Messages) == 0 {
		return TurnResult{Text: generateDemoReply(req)}, nil
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderRequestFailed,
			Message: "failed to encode provider request",
			Err:     err,
		}
	}

	requestCtx := ctx
	cancel := func() {}
	if cfg.TimeoutMS > 0 {
		requestCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutMS)*time.Millisecond)
	}
	defer cancel()

	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderRequestFailed,
			Message: "failed to create provider request",
			Err:     err,
		}
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		k := strings.TrimSpace(key)
		v := strings.TrimSpace(value)
		if k == "" || v == "" {
			continue
		}
		httpReq.Header.Set(k, v)
	}

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderRequestFailed,
			Message: "provider request failed",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderRequestFailed,
			Message: "failed to read provider response",
			Err:     err,
		}
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderRequestFailed,
			Message: fmt.Sprintf("provider returned status %d", resp.StatusCode),
		}
	}

	var completion openAIChatResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderInvalidReply,
			Message: "provider response is not valid json",
			Err:     err,
		}
	}
	if len(completion.Choices) == 0 {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderInvalidReply,
			Message: "provider response has no choices",
		}
	}

	message := completion.Choices[0].Message
	text := strings.TrimSpace(extractOpenAIContent(message.Content))
	toolCalls, err := parseOpenAIToolCalls(message.ToolCalls)
	if err != nil {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderInvalidReply,
			Message: err.Error(),
			Err:     err,
		}
	}
	if text == "" && len(toolCalls) == 0 {
		return TurnResult{}, &RunnerError{
			Code:    ErrorCodeProviderInvalidReply,
			Message: "provider response has empty content",
		}
	}

	return TurnResult{Text: text, ToolCalls: toolCalls}, nil
}

type openAIChatRequest struct {
	Model    string                 `json:"model"`
	Messages []openAIMessage        `json:"messages"`
	Tools    []openAIToolDefinition `json:"tools,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content,omitempty"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	Name       string           `json:"name,omitempty"`
}

type openAIToolDefinition struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type openAIToolCall struct {
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openAIFunctionCall `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

type openAIChatResponse struct {
	Choices []struct {
		Message struct {
			Content   json.RawMessage  `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls,omitempty"`
		} `json:"message"`
	} `json:"choices"`
}

func toOpenAIMessages(input []domain.AgentInputMessage) []openAIMessage {
	out := make([]openAIMessage, 0, len(input))
	for _, msg := range input {
		role := normalizeRole(msg.Role)
		content := strings.TrimSpace(flattenText(msg.Content))

		switch role {
		case "assistant":
			toolCalls := parseToolCallsFromMetadata(msg.Metadata)
			item := openAIMessage{Role: role}
			if content != "" {
				item.Content = content
			}
			if len(toolCalls) > 0 {
				item.ToolCalls = toolCalls
			}
			if item.Content == nil && len(item.ToolCalls) == 0 {
				continue
			}
			out = append(out, item)
		case "tool":
			item := openAIMessage{
				Role:    role,
				Content: content,
			}
			if item.Content == nil {
				item.Content = ""
			}
			if toolCallID := metadataString(msg.Metadata, "tool_call_id"); toolCallID != "" {
				item.ToolCallID = toolCallID
			}
			if name := metadataString(msg.Metadata, "name"); name != "" {
				item.Name = name
			}
			out = append(out, item)
		default:
			if content == "" {
				continue
			}
			out = append(out, openAIMessage{Role: role, Content: content})
		}
	}
	return out
}

func toOpenAITools(tools []ToolDefinition) []openAIToolDefinition {
	if len(tools) == 0 {
		return nil
	}
	out := make([]openAIToolDefinition, 0, len(tools))
	for _, item := range tools {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		params := normalizeToolParameters(item.Parameters)
		out = append(out, openAIToolDefinition{
			Type: "function",
			Function: openAIToolFunction{
				Name:        name,
				Description: strings.TrimSpace(item.Description),
				Parameters:  params,
			},
		})
	}
	return out
}

func parseOpenAIToolCalls(in []openAIToolCall) ([]ToolCall, error) {
	if len(in) == 0 {
		return nil, nil
	}
	calls := make([]ToolCall, 0, len(in))
	for i, item := range in {
		name := strings.TrimSpace(item.Function.Name)
		if name == "" {
			return nil, fmt.Errorf("provider tool call[%d] name is empty", i)
		}
		callID := strings.TrimSpace(item.ID)
		if callID == "" {
			callID = fmt.Sprintf("call_%d", i+1)
		}
		argumentsRaw := strings.TrimSpace(item.Function.Arguments)
		if argumentsRaw == "" {
			argumentsRaw = "{}"
		}
		var arguments map[string]interface{}
		if err := json.Unmarshal([]byte(argumentsRaw), &arguments); err != nil {
			return nil, fmt.Errorf("provider tool call %q has invalid arguments: %w", name, err)
		}
		if arguments == nil {
			arguments = map[string]interface{}{}
		}
		calls = append(calls, ToolCall{ID: callID, Name: name, Arguments: arguments})
	}
	return calls, nil
}

func parseToolCallsFromMetadata(metadata map[string]interface{}) []openAIToolCall {
	if len(metadata) == 0 {
		return nil
	}
	raw, ok := metadata["tool_calls"]
	if !ok || raw == nil {
		return nil
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var out []openAIToolCall
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil
	}
	valid := make([]openAIToolCall, 0, len(out))
	for _, call := range out {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		if strings.TrimSpace(call.ID) == "" {
			continue
		}
		args := strings.TrimSpace(call.Function.Arguments)
		if args == "" {
			call.Function.Arguments = "{}"
		}
		if strings.TrimSpace(call.Type) == "" {
			call.Type = "function"
		}
		valid = append(valid, call)
	}
	return valid
}

func metadataString(metadata map[string]interface{}, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	raw, ok := metadata[key]
	if !ok || raw == nil {
		return ""
	}
	text, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func normalizeToolParameters(in map[string]interface{}) map[string]interface{} {
	if len(in) == 0 {
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	buf, err := json.Marshal(in)
	if err != nil {
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(buf, &out); err != nil {
		return map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
		}
	}
	if _, ok := out["type"]; !ok {
		out["type"] = "object"
	}
	return out
}

func flattenText(content []domain.RuntimeContent) string {
	parts := make([]string, 0, len(content))
	for _, c := range content {
		if c.Type != "text" {
			continue
		}
		text := strings.TrimSpace(c.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, "\n")
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "system", "assistant", "user", "tool":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "user"
	}
}

func extractOpenAIContent(raw json.RawMessage) string {
	var direct string
	if err := json.Unmarshal(raw, &direct); err == nil {
		return direct
	}
	var arr []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &arr); err == nil {
		parts := make([]string, 0, len(arr))
		for _, item := range arr {
			if item.Type != "text" {
				continue
			}
			text := strings.TrimSpace(item.Text)
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}
