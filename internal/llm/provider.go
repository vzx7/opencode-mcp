package llm

import (
	"context"
	"strings"
)

type LLMProvider interface {
	Complete(ctx context.Context, prompt string, language string) (string, error)
	Name() string
}

func NewProvider(providerType string, apiKey string, endpoint string, model string) (LLMProvider, error) {
	switch providerType {
	case "mock":
		return NewMockProvider(), nil
	case "openai":
		actualEndpoint := endpoint
		if endpoint == "" {
			actualEndpoint = "https://api.openai.com/v1/chat/completions"
		}
		return NewOpenAIProvider(apiKey, actualEndpoint, model), nil
	case "anthropic":
		return NewAnthropicProvider(apiKey, endpoint, model), nil
	default:
		// Any OpenAI-compatible provider
		actualEndpoint := endpoint
		if endpoint == "" {
			actualEndpoint = "https://api.openai.com/v1/chat/completions"
		}
		return NewOpenAIProvider(apiKey, actualEndpoint, model), nil
	}
}

type MockProvider struct {
	model     string
	responses map[string]map[string]string
}

func NewMockProvider() *MockProvider {
	return &MockProvider{
		model: "mock",
		responses: map[string]map[string]string{
			"architecture_review": {
				"en": "Mock: Architecture review shows good layering with clear separation of concerns. Score: 85/100",
				"ru": "Mock: Анализ архитектуры показывает хорошее разделение слоев и четкое разграничение ответственности. Оценка: 85/100",
			},
			"architecture_compliance": {
				"en": "Mock: Architecture compliance check passed. All layers properly isolated.",
				"ru": "Mock: Проверка соответствия архитектуре пройдена. Все слои правильно изолированы.",
			},
			"module_audit": {
				"en": "Mock: Module audit complete. Code quality is good with minor improvements suggested.",
				"ru": "Mock: Аудит модуля завершен. Качество кода хорошее, предложены незначительные улучшения.",
			},
		},
	}
}

func (p *MockProvider) Complete(ctx context.Context, prompt string, language string) (string, error) {
	toolName := extractToolName(prompt)
	if resp, ok := p.responses[toolName]; ok {
		if langResp, ok := resp[language]; ok {
			return langResp, nil
		}
		if langResp, ok := resp["en"]; ok {
			return langResp, nil
		}
	}
	return "Mock: No response found", nil
}

func extractToolName(prompt string) string {
	lines := strings.Split(prompt, "\n")
	for _, line := range lines {
		if strings.Contains(line, "You are performing") || strings.Contains(line, "Вы выполняете") {
			if strings.Contains(line, "architecture review") || strings.Contains(line, "обзор архитектуры") {
				return "architecture_review"
			}
			if strings.Contains(line, "compliance") || strings.Contains(line, "соответстви") {
				return "architecture_compliance"
			}
			if strings.Contains(line, "module audit") || strings.Contains(line, "аудит модуля") {
				return "module_audit"
			}
		}
	}
	return "architecture_review"
}

func (p *MockProvider) Name() string {
	return "mock"
}