package llm

import (
	"context"
	"fmt"
	"strings"
)

type LLMProvider interface {
	Complete(ctx context.Context, prompt string, language string) (string, error)
	Name() string
}

const (
	ToolMarkerReview       = "[TOOL: architecture_review]"
	ToolMarkerCompliance  = "[TOOL: architecture_compliance]"
	ToolMarkerModuleAudit = "[TOOL: module_audit]"
)

func getDefaultEndpoint(providerType string) string {
	switch providerType {
	case "anthropic":
		return "https://api.anthropic.com/v1/messages"
	case "openai", "":
		fallthrough
	default:
		return "https://api.openai.com/v1/chat/completions"
	}
}

var validProviders = map[string]bool{
	"mock":     true,
	"openai":   true,
	"anthropic": true,
}

func NewProvider(providerType string, apiKey string, endpoint string, model string) (LLMProvider, error) {
	if providerType != "mock" && providerType != "" && apiKey == "" {
		return nil, fmt.Errorf("API key is required for provider %q", providerType)
	}

	if providerType != "" && !validProviders[providerType] {
		return nil, fmt.Errorf("unknown provider %q", providerType)
	}

	if endpoint == "" {
		endpoint = getDefaultEndpoint(providerType)
	}

	switch providerType {
	case "openai":
		return NewOpenAIProvider(apiKey, endpoint, model), nil
	case "anthropic":
		return NewAnthropicProvider(apiKey, endpoint, model), nil
	case "mock", "":
		fallthrough
	default:
		return NewMockProvider(), nil
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
				"zh": `{"score":85,"summary":"架构审查：分层良好，职责清晰。检测到标准Go项目结构。","issues":[{"severity":"low","message":"缺少pkg/层用于共享工具","location":"项目根目录","suggestion":"考虑添加pkg/用于可重用包"}],"recommendations":["添加集成测试","记录公共API"]}`,
			},
			"architecture_compliance": {
				"en": `{"score":90,"summary":"Architecture compliance check passed. All required layers are properly isolated.","issues":[],"recommendations":["Verify dependency direction between layers"]}`,
				"ru": `{"score":90,"summary":"Проверка соответствия архитектуре пройдена. Все необходимые слои правильно изолированы.","issues":[],"recommendations":["Проверить направление зависимостей между слоями"]}`,
				"zh": `{"score":90,"summary":"架构合规性检查通过。所有必需的层都已正确隔离。","issues":[],"recommendations":["验证层之间的依赖方向"]}`,
			},
			"module_audit": {
				"en": `{"score":80,"summary":"Module audit complete. Code quality is acceptable with minor improvements suggested.","issues":[{"severity":"low","message":"Missing unit tests","location":"module","suggestion":"Add table-driven tests for edge cases"}],"recommendations":["Increase test coverage","Add godoc comments to exported functions"]}`,
				"ru": `{"score":80,"summary":"Аудит модуля завершен. Качество кода приемлемое, предложены незначительные улучшения.","issues":[{"severity":"low","message":"Отсутствуют юнит-тесты","location":"модуль","suggestion":"Добавить табличные тесты для граничных случаев"}],"recommendations":["Увеличить тестовое покрытие","Добавить godoc-комментарии к экспортируемым функциям"]}`,
				"zh": `{"score":80,"summary":"模块审计完成。代码质量可接受，建议小幅改进。","issues":[{"severity":"low","message":"缺少单元测试","location":"模块","suggestion":"添加边界情况的表格驱动测试"}],"recommendations":["增加测试覆盖率","为导出的函数添加godoc注释"]}`,
			},
		},
	}
}

func (p *MockProvider) Complete(ctx context.Context, prompt string, language string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("prompt cannot be empty")
	}

	toolName := extractToolName(prompt)

	if resp, ok := p.responses[toolName]; ok {
		if langResp, ok := resp[language]; ok {
			return langResp, nil
		}
		if langResp, ok := resp["en"]; ok {
			return langResp, nil
		}
	}
	return "", fmt.Errorf("mock provider: no response found for tool=%q, lang=%q", toolName, language)
}

func extractToolName(prompt string) string {
	if strings.Contains(prompt, ToolMarkerReview) {
		return "architecture_review"
	}
	if strings.Contains(prompt, ToolMarkerCompliance) {
		return "architecture_compliance"
	}
	if strings.Contains(prompt, ToolMarkerModuleAudit) {
		return "module_audit"
	}

	lines := strings.Split(prompt, "\n")
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "architecture review") || strings.Contains(lower, "обзор архитектуры") || strings.Contains(lower, "架构审查") {
			return "architecture_review"
		}
		if strings.Contains(lower, "compliance") || strings.Contains(lower, "соответстви") || strings.Contains(lower, "合规") {
			return "architecture_compliance"
		}
		if strings.Contains(lower, "module audit") || strings.Contains(lower, "аудит модуля") || strings.Contains(lower, "模块审计") {
			return "module_audit"
		}
	}
	return "architecture_review"
}

func (p *MockProvider) Name() string {
	return "mock"
}