package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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
		client:  &http.Client{Timeout: 0},
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
		client:   &http.Client{},
	}
}

func (p *AnthropicProvider) Complete(ctx context.Context, prompt string, language string) (string, error) {
	return "", &notImplementedError{"anthropic provider needs implementation"}
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