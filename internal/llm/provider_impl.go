package llm

import (
	"context"
	"net/http"
	"strings"
)

type OpenAIProvider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
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
		model:    model,
		client:   &http.Client{},
	}
}

func (p *OpenAIProvider) Complete(ctx context.Context, prompt string) (string, error) {
	return "", &notImplementedError{"openai provider needs implementation"}
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

func (p *AnthropicProvider) Complete(ctx context.Context, prompt string) (string, error) {
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

func BuildReviewPrompt(projectMap *ProjectMap) string {
	var sb strings.Builder
	sb.WriteString("You are a senior software architect performing an architecture review.\n\n")
	sb.WriteString("## Project Structure\n")
	sb.WriteString("Root: ")
	sb.WriteString(projectMap.Root)
	sb.WriteString("\n\n")
	sb.WriteString("Modules:\n")
	for _, m := range projectMap.Modules {
		sb.WriteString("- ")
		sb.WriteString(m.Name)
		sb.WriteString(": ")
		sb.WriteString(m.Path)
		sb.WriteString("\n")
	}
	sb.WriteString("\nLayers:\n")
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
	sb.WriteString("Return your response as a structured audit report.")
	return sb.String()
}

func BuildCompliancePrompt(rules *ArchitectureRules, projectMap *ProjectMap) string {
	var sb strings.Builder
	sb.WriteString("You are performing an architecture compliance check.\n\n")
	sb.WriteString("## Target Architecture Rules\n")
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
	sb.WriteString("\nIdentify any architecture violations.")
	sb.WriteString("\nReturn a structured report of violations with severity levels.")
	return sb.String()
}

func BuildModuleAuditPrompt(modulePath string, files []string) string {
	var sb strings.Builder
	sb.WriteString("You are performing a module audit.\n\n")
	sb.WriteString("## Module Path: ")
	sb.WriteString(modulePath)
	sb.WriteString("\n\n## Files:\n")
	for _, f := range files {
		sb.WriteString("- ")
		sb.WriteString(f)
		sb.WriteString("\n")
	}
	sb.WriteString("\nAnalyze for:\n")
	sb.WriteString("1. Correctness\n")
	sb.WriteString("2. Design quality\n")
	sb.WriteString("3. Coupling and cohesion\n")
	sb.WriteString("4. Potential bugs\n")
	sb.WriteString("5. Complexity issues\n\n")
	sb.WriteString("Return a structured audit report.")
	return sb.String()
}