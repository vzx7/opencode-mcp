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
				"en": `{"score":85,"summary":"Architecture review: good layering with clear separation of concerns. Standard Go project structure detected.","issues":[{"severity":"low","message":"No pkg/ layer for shared utilities","location":"project root","suggestion":"Consider adding pkg/ for reusable packages"}],"recommendations":["Add integration tests","Document public APIs"]}`,
				"ru": `{"score":85,"summary":"Анализ архитектуры: хорошее разделение слоев и четкое разграничение ответственности. Обнаружена стандартная структура Go-проекта.","issues":[{"severity":"low","message":"Отсутствует слой pkg/ для общих утилит","location":"корень проекта","suggestion":"Рассмотрите добавление pkg/ для переиспользуемых пакетов"}],"recommendations":["Добавить интеграционные тесты","Задокументировать публичные API"]}`,
			},
			"architecture_compliance": {
				"en": `{"score":90,"summary":"Architecture compliance check passed. All required layers are properly isolated.","issues":[],"recommendations":["Verify dependency direction between layers"]}`,
				"ru": `{"score":90,"summary":"Проверка соответствия архитектуре пройдена. Все необходимые слои правильно изолированы.","issues":[],"recommendations":["Проверить направление зависимостей между слоями"]}`,
			},
			"module_audit": {
				"en": `{"score":80,"summary":"Module audit complete. Code quality is acceptable with minor improvements suggested.","issues":[{"severity":"low","message":"Missing unit tests","location":"module","suggestion":"Add table-driven tests for edge cases"}],"recommendations":["Increase test coverage","Add godoc comments to exported functions"]}`,
				"ru": `{"score":80,"summary":"Аудит модуля завершен. Качество кода приемлемое, предложены незначительные улучшения.","issues":[{"severity":"low","message":"Отсутствуют юнит-тесты","location":"модуль","suggestion":"Добавить табличные тесты для граничных случаев"}],"recommendations":["Увеличить тестовое покрытие","Добавить godoc-комментарии к экспортируемым функциям"]}`,
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