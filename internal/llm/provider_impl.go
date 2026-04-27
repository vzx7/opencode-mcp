package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTimeout = 10 * time.Minute
	MaxRetries   = 3
	InitialDelay = 1 * time.Second
)

type OpenAIProvider struct {
	apiKey   string
	endpoint string
	model   string
	client  *http.Client
}

func NewOpenAIProvider(apiKey string, endpoint string, model string) *OpenAIProvider {
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIProvider{
		apiKey:   apiKey,
		endpoint: endpoint,
		model:   model,
		client:  &http.Client{Timeout: DefaultTimeout},
	}
}

type openAIRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	MaxTokens int      `json:"max_tokens,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message message `json:"message"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, prompt string, language string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("API key required for OpenAI provider")
	}

	reqBody := openAIRequest{
		Model: p.model,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 4000,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	delay := InitialDelay

	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = nil
		var result string
		result, lastErr = p.doRequest(ctx, body)
		if lastErr == nil {
			return result, nil
		}

		if rErr, ok := lastErr.(*retryableError); ok && rErr.retryAfter > 0 {
			delay = rErr.retryAfter
		} else if !isRetryableError(lastErr) {
			return "", lastErr
		} else if delay < InitialDelay*4 {
			delay *= 2
		}
	}

	return "", fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (p *OpenAIProvider) doRequest(ctx context.Context, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode >= 500 {
		return "", &retryableError{statusCode: resp.StatusCode, retryAfter: parseRetryAfter(resp.Header)}
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error: %s", string(respBody))
	}

	var result openAIResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	return result.Choices[0].Message.Content, nil
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*retryableError); ok {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "request failed") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "i/o timeout")
}

type retryableError struct {
	statusCode int
	retryAfter time.Duration
}

func (e *retryableError) Error() string {
	if e.retryAfter > 0 {
		return fmt.Sprintf("retryable error: status %d, retry after %v", e.statusCode, e.retryAfter)
	}
	return fmt.Sprintf("retryable error: status %d", e.statusCode)
}

func parseRetryAfter(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			return time.Duration(d) * time.Second
		}
	}
	return 0
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

type AnthropicProvider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

func NewAnthropicProvider(apiKey string, endpoint string, model string) *AnthropicProvider {
	if endpoint == "" {
		endpoint = "https://api.anthropic.com/v1/messages"
	}
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	return &AnthropicProvider{
		apiKey:   apiKey,
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{Timeout: DefaultTimeout},
	}
}

type anthropicRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	MaxTokens int     `json:"max_tokens,omitempty"`
}

type anthropicResponse struct {
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
}

func (p *AnthropicProvider) Complete(ctx context.Context, prompt string, language string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("API key required for Anthropic provider")
	}

	reqBody := anthropicRequest{
		Model: p.model,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 4096,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	delay := InitialDelay

	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = nil
		var result string
		result, lastErr = p.doRequest(ctx, body)
		if lastErr == nil {
			return result, nil
		}

		if rErr, ok := lastErr.(*retryableError); ok && rErr.retryAfter > 0 {
			delay = rErr.retryAfter
		} else if !isRetryableError(lastErr) {
			return "", lastErr
		} else if delay < InitialDelay*4 {
			delay *= 2
		}
	}

	return "", fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (p *AnthropicProvider) doRequest(ctx context.Context, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode >= 500 {
		return "", &retryableError{statusCode: resp.StatusCode, retryAfter: parseRetryAfter(resp.Header)}
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error: %s", string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in response")
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

type notImplementedError struct {
	msg string
}

func (e *notImplementedError) Error() string {
	return e.msg
}

func BuildReviewPrompt(projectMap *ProjectMap, language string) string {
	var sb strings.Builder

	p := GetPrompts(language)

	sb.WriteString(ToolMarkerReview)
	sb.WriteString("\n")
	sb.WriteString(p.SystemRole)
	sb.WriteString("\n\n## " + p.Labels["root"] + " ")
	sb.WriteString(projectMap.Root)
	sb.WriteString("\n\n")
	sb.WriteString(p.Labels["modules"])
	sb.WriteString("\n")
	for _, m := range projectMap.Modules {
		sb.WriteString("- ")
		sb.WriteString(m.Name)
		sb.WriteString(": ")
		sb.WriteString(m.Path)
		sb.WriteString("\n")
	}
	sb.WriteString("\n" + p.Labels["layers"] + "\n")
	for _, l := range projectMap.Layers {
		sb.WriteString("- ")
		sb.WriteString(l.Name)
		sb.WriteString(": ")
		sb.WriteString(strings.Join(l.Paths, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("\nProvide a structured architecture review focusing on:\n")
	for i, focus := range p.ReviewFocus {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, focus))
	}
	sb.WriteString("\n")
	sb.WriteString(p.JSONSchemaInstruction)
	sb.WriteString("\n")
	sb.WriteString(GetJSONSchema(language))
	return sb.String()
}

func appendJSONSchema(sb *strings.Builder, language string) {
	p := GetPrompts(language)
	sb.WriteString("\n\n" + p.JSONSchemaInstruction)
	sb.WriteString("\n")
	sb.WriteString(GetJSONSchema(language))
}

func BuildCompliancePrompt(rules *ArchitectureRules, projectMap *ProjectMap, language string) string {
	var sb strings.Builder

	p := GetPrompts(language)
	c := p.Compliance

	sb.WriteString(ToolMarkerCompliance)
	sb.WriteString("\n")
	sb.WriteString(c.SystemRole)
	sb.WriteString("\n\n## Target Architecture Rules\n")
	for _, layer := range rules.Layers {
		sb.WriteString("Layer: ")
		sb.WriteString(layer.Name)
		sb.WriteString("\n  " + p.Labels["paths"] + " ")
		sb.WriteString(strings.Join(layer.Paths, ", "))
		sb.WriteString("\n  " + p.Labels["allowed"] + " ")
		sb.WriteString(strings.Join(layer.Allow, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Current Project Structure\n")
	sb.WriteString(p.Labels["root"] + " ")
	sb.WriteString(projectMap.Root)
	sb.WriteString("\n\n" + p.Labels["layers"] + "\n")
	for _, l := range projectMap.Layers {
		sb.WriteString("- ")
		sb.WriteString(l.Name)
		sb.WriteString(": ")
		sb.WriteString(strings.Join(l.Paths, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("\n" + c.IdentifyViolations)
	sb.WriteString("\n" + c.ReturnFormat)
	sb.WriteString("\n")
	sb.WriteString(p.JSONSchemaInstruction)
	sb.WriteString("\n")
	sb.WriteString(GetJSONSchema(language))
	return sb.String()
}

func BuildModuleAuditPrompt(modulePath string, files map[string]string, language string) string {
	var sb strings.Builder

	p := GetPrompts(language)
	m := p.ModuleAudit

	sb.WriteString(ToolMarkerModuleAudit)
	sb.WriteString("\n")
	sb.WriteString(m.SystemRole)
	sb.WriteString("\n\n## Module Path: ")
	sb.WriteString(modulePath)
	sb.WriteString("\n\n## Source Code:\n")
	for path, content := range files {
		sb.WriteString("### ")
		sb.WriteString(path)
		sb.WriteString("\n```go\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n\n")
	}
	sb.WriteString(m.ReturnFormat)
	sb.WriteString("\n")
	sb.WriteString(p.JSONSchemaInstruction)
	sb.WriteString("\n")
	sb.WriteString(GetJSONSchema(language))
	return sb.String()
}