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

	systemLang := "You are a senior software architect performing an architecture review."
	responseLang := "Return your response as a structured audit report in"
	modulesLabel := "Modules:"
	layersLabel := "Layers:"

	if language == "ru" {
		systemLang = "Вы - опытный архитектор программного обеспечения, выполняющий обзор архитектуры."
		responseLang = "Верните ответ в виде структурированного отчета об аудите на"
		modulesLabel = "Модули:"
		layersLabel = "Слои:"
	}

	sb.WriteString(systemLang)
	sb.WriteString("\n\n## Project Structure\n")
	sb.WriteString("Root: ")
	sb.WriteString(projectMap.Root)
	sb.WriteString("\n\n")
	sb.WriteString(modulesLabel)
	sb.WriteString("\n")
	for _, m := range projectMap.Modules {
		sb.WriteString("- ")
		sb.WriteString(m.Name)
		sb.WriteString(": ")
		sb.WriteString(m.Path)
		sb.WriteString("\n")
	}
	sb.WriteString("\n" + layersLabel + "\n")
	for _, l := range projectMap.Layers {
		sb.WriteString("- ")
		sb.WriteString(l.Name)
		sb.WriteString(": ")
		sb.WriteString(strings.Join(l.Paths, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("\nProvide a structured architecture review focusing on:\n")
	sb.WriteString("1. Architecture style identification\n")
	sb.WriteString("2. Coupling and cohesion analysis\n")
	sb.WriteString("3. Maintainability assessment\n")
	sb.WriteString("4. Scalability evaluation\n")
	sb.WriteString("5. Issues and recommendations\n\n")
	sb.WriteString(responseLang)
	sb.WriteString(" ")
	sb.WriteString(getLangLabel(language))
	sb.WriteString(".")
	return sb.String()
}

func getLangLabel(lang string) string {
	switch lang {
	case "ru":
		return "Russian"
	default:
		return "English"
	}
}

func BuildCompliancePrompt(rules *ArchitectureRules, projectMap *ProjectMap, language string) string {
	var sb strings.Builder

	systemLang := "You are performing an architecture compliance check."
	identifyViolations := "Identify any architecture violations."
	returnFormat := "Return a structured report of violations with severity levels."

	if language == "ru" {
		systemLang = "Вы выполняете проверку соответствия архитектуры."
		identifyViolations = "Выявите любые нарушения архитектуры."
		returnFormat = "Верните структурированный отчет о нарушениях с уровнями серьезности."
	}

	sb.WriteString(systemLang)
	sb.WriteString("\n\n## Target Architecture Rules\n")
	for _, layer := range rules.Layers {
		sb.WriteString("Layer: ")
		sb.WriteString(layer.Name)
		sb.WriteString("\n  Paths: ")
		sb.WriteString(strings.Join(layer.Paths, ", "))
		sb.WriteString("\n  Allowed: ")
		sb.WriteString(strings.Join(layer.Allow, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("\n## Current Project Structure\n")
	sb.WriteString("Root: ")
	sb.WriteString(projectMap.Root)
	sb.WriteString("\n\nLayers:\n")
	for _, l := range projectMap.Layers {
		sb.WriteString("- ")
		sb.WriteString(l.Name)
		sb.WriteString(": ")
		sb.WriteString(strings.Join(l.Paths, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("\n" + identifyViolations)
	sb.WriteString("\n" + returnFormat)
	sb.WriteString(" in ")
	sb.WriteString(getLangLabel(language))
	sb.WriteString(".")
	return sb.String()
}

func BuildModuleAuditPrompt(modulePath string, files map[string]string, language string) string {
	var sb strings.Builder

	systemLang := "You are performing a module audit. Analyze the provided source code for correctness, design quality, coupling/cohesion, potential bugs, and complexity issues."
	returnFormat := "Return a structured audit report."

	if language == "ru" {
		systemLang = "Вы выполняете аудит модуля. Проанализируйте предоставленный исходный код на корректность, качество дизайна, связность/зацепление, потенциальные баги и проблемы сложности."
		returnFormat = "Верните структурированный отчет об аудите."
	}

	sb.WriteString(systemLang)
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
	sb.WriteString(returnFormat)
	sb.WriteString(" in ")
	sb.WriteString(getLangLabel(language))
	sb.WriteString(".")
	return sb.String()
}