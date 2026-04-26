package llm

import (
	"context"
	"fmt"
)

type LLMProvider interface {
	Complete(ctx context.Context, prompt string) (string, error)
	Name() string
}

func NewProvider(providerType string, apiKey string, endpoint string, model string) (LLMProvider, error) {
	switch providerType {
	case "mock":
		return NewMockProvider(), nil
	case "openai":
		return NewOpenAIProvider(apiKey, endpoint, model), nil
	case "anthropic":
		return NewAnthropicProvider(apiKey, endpoint, model), nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", providerType)
	}
}

type MockProvider struct {
	model     string
	responses map[string]string
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		model: "mock",
		responses: map[string]string{
			"architecture_review":      "Mock: Architecture review shows good layering with clear separation of concerns. Score: 85/100",
			"architecture_compliance":  "Mock: Architecture compliance check passed. All layers properly isolated.",
			"module_audit":             "Mock: Module audit complete. Code quality is good with minor improvements suggested.",
		},
	}
}

func (p *MockProvider) Complete(ctx context.Context, prompt string) (string, error) {
	return p.responses["architecture_review"], nil
}

func (p *MockProvider) Name() string {
	return "mock"
}