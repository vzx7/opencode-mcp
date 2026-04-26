package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ai-mcp/code-auditor/internal/analyzer"
	"github.com/ai-mcp/code-auditor/internal/domain"
	"github.com/ai-mcp/code-auditor/internal/llm"
)

const (
	defaultModelAnthropic = "claude-3-5-sonnet-20241022"
	defaultModelOpenAI    = "gpt-4o"
)

type ToolExecutor struct {
	defaultPath string
	defaultLLM llm.LLMProvider
	defaultLang string
	apiKey    string
	endpoint  string
	mu        sync.RWMutex
	debug     bool
	logger    *log.Logger
}

type ToolExecutorConfig struct {
	DefaultPath string
	LLM       llm.LLMProvider
	APIKey    string
	Endpoint  string
	Language  string
	Debug    bool
	Logger   *log.Logger
}

func NewToolExecutor(cfg ToolExecutorConfig) *ToolExecutor {
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	if cfg.Logger == nil {
		cfg.Logger = log.New(os.Stdout, "[TOOL] ", log.LstdFlags)
	}
	return &ToolExecutor{
		defaultPath: cfg.DefaultPath,
		defaultLLM: cfg.LLM,
		defaultLang: cfg.Language,
		apiKey:     cfg.APIKey,
		endpoint:   cfg.Endpoint,
		debug:     cfg.Debug,
		logger:    cfg.Logger,
	}
}

func (te *ToolExecutor) SetDebug(debug bool) {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.debug = debug
	if debug && te.logger == nil {
		te.logger = log.New(os.Stdout, "[TOOL] ", log.LstdFlags)
	}
}

type ToolInput struct {
	ProjectPath string `json:"project_path"`
	ModulePath  string `json:"module_path,omitempty"`
	Provider    string `json:"provider"`
	LLM         string `json:"llm"`
	Language    string `json:"language,omitempty"`
}

func (te *ToolExecutor) getLLM(provider, model string) (llm.LLMProvider, error) {
	if provider == "" && model == "" {
		return te.defaultLLM, nil
	}

	p := provider
	m := model

	if p == "" {
		p = te.defaultLLM.Name()
	}
	if m == "" {
		switch te.defaultLLM.Name() {
		case "anthropic":
			m = defaultModelAnthropic
		default:
			m = defaultModelOpenAI
		}
	}

	newLLM, err := llm.NewProvider(p, te.apiKey, te.endpoint, m)
	if err != nil {
		return te.defaultLLM, fmt.Errorf("failed to create LLM provider: %w", err)
	}
	return newLLM, nil
}

func (te *ToolExecutor) getLanguage(language string) string {
	if language != "" {
		return language
	}
	return te.defaultLang
}

type ArchitectureReviewInput struct {
	ToolInput
}

func (te *ToolExecutor) ArchitectureReview(ctx context.Context, input ArchitectureReviewInput) (*domain.AuditReport, error) {
	projectPath := input.ProjectPath
	if projectPath == "" {
		projectPath = te.defaultPath
	}
	if projectPath == "" {
		projectPath = "."
	}

	analyzerEngine := analyzer.New(projectPath)
	pm, err := analyzerEngine.BuildProjectMap()
	if err != nil {
		return nil, fmt.Errorf("failed to build project map: %w", err)
	}

	llmProvider, err := te.getLLM(input.Provider, input.LLM)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM: %w", err)
	}
	language := te.getLanguage(input.Language)
	prompt := llm.BuildReviewPrompt(pm, language)

	te.mu.RLock()
	debug := te.debug
	logger := te.logger
	te.mu.RUnlock()

	if debug {
		logger.Printf(">>> LLM Request (%s): %s", llmProvider.Name(), prompt[:min(200, len(prompt))])
	}

	llmResponse, err := llmProvider.Complete(ctx, prompt, language)
	if err != nil {
		if debug {
			logger.Printf("<<< LLM Error: %v", err)
		}
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	if debug {
		te.logger.Printf("<<< LLM Response (%s): %s", llmProvider.Name(), llmResponse[:min(200, len(llmResponse))])
	}

	report := te.buildReportFromLLM(llmResponse)
	te.enrichWithLocalAnalysis(report, pm)

	return report, nil
}

// parseLLMResponse tries to unmarshal LLM JSON output into AuditReport.
// Falls back to a summary-only report if parsing fails.
func parseLLMResponse(response string) *domain.AuditReport {
	s := strings.TrimSpace(response)
	// Strip optional ```json … ``` fences
	if i := strings.Index(s, "```json"); i != -1 {
		s = s[i+7:]
		if j := strings.Index(s, "```"); j != -1 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
	}
	// Find outermost JSON object
	if i := strings.Index(s, "{"); i != -1 {
		if j := strings.LastIndex(s, "}"); j > i {
			s = s[i : j+1]
		}
	}

	var report domain.AuditReport
	if err := json.Unmarshal([]byte(s), &report); err == nil && report.Summary != "" {
		if report.Issues == nil {
			report.Issues = []domain.Issue{}
		}
		if report.Recommendations == nil {
			report.Recommendations = []string{}
		}
		return &report
	}

	// Fallback: raw text in summary, neutral score
	return &domain.AuditReport{
		Score:           75,
		Summary:         response,
		Issues:          []domain.Issue{},
		Recommendations: []string{},
	}
}

func (te *ToolExecutor) buildReportFromLLM(response string) *domain.AuditReport {
	return parseLLMResponse(response)
}

func (te *ToolExecutor) enrichWithLocalAnalysis(report *domain.AuditReport, pm *domain.ProjectMap) {
	if len(pm.Modules) == 0 {
		report.Issues = append(report.Issues, domain.Issue{
			Severity:   domain.SeverityHigh,
			Message:    "No Go modules found in project",
			Location:   pm.Root,
			Suggestion: "Ensure project has proper Go module structure",
		})
		report.Score -= 20
	}

	if len(pm.Layers) < 2 {
		report.Issues = append(report.Issues, domain.Issue{
			Severity:   domain.SeverityMedium,
			Message:   "Limited architecture layers detected",
			Location:  pm.Root,
			Suggestion: "Consider adopting layered architecture (cmd, internal, pkg)",
		})
		report.Score -= 10
	}

	hasInternal := false
	hasCmd := false
	for _, l := range pm.Layers {
		if l.Name == "internal" {
			hasInternal = true
		}
		if l.Name == "cmd" {
			hasCmd = true
		}
	}

	if !hasInternal {
		report.Issues = append(report.Issues, domain.Issue{
			Severity:   domain.SeverityLow,
			Message:   "No internal layer found",
			Location:  pm.Root,
			Suggestion: "Consider adding internal package for private code",
		})
		report.Score -= 5
	}

	if !hasCmd {
		report.Recommendations = append(report.Recommendations, "Consider adding cmd/ for entrypoints")
	}

	if report.Score < 0 {
		report.Score = 0
	}
}

type ArchitectureComplianceInput struct {
	ToolInput
	TargetArchitecture *domain.ArchitectureRules `json:"target_architecture"`
}

func (te *ToolExecutor) ArchitectureComplianceCheck(ctx context.Context, input ArchitectureComplianceInput) (*domain.AuditReport, error) {
	projectPath := input.ProjectPath
	if projectPath == "" {
		projectPath = te.defaultPath
	}
	if projectPath == "" {
		projectPath = "."
	}

	analyzerEngine := analyzer.New(projectPath)
	pm, err := analyzerEngine.BuildProjectMap()
	if err != nil {
		return nil, fmt.Errorf("failed to build project map: %w", err)
	}

	rules := input.TargetArchitecture
	if rules == nil {
		rules = te.getDefaultRules()
	}

	report := analyzerEngine.CheckCompliance(rules, pm)

	llmProvider, err := te.getLLM(input.Provider, input.LLM)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM: %w", err)
	}
	language := te.getLanguage(input.Language)
	if llmProvider != nil {
		prompt := llm.BuildCompliancePrompt(rules, pm, language)

		te.mu.RLock()
		debug := te.debug
		logger := te.logger
		te.mu.RUnlock()

		if debug {
			logger.Printf(">>> LLM Request (%s): %s", llmProvider.Name(), prompt[:min(200, len(prompt))])
		}

		llmResponse, err := llmProvider.Complete(ctx, prompt, language)
		if err == nil {
			if debug {
				logger.Printf("<<< LLM Response (%s): %s", llmProvider.Name(), llmResponse[:min(200, len(llmResponse))])
			}
			llmReport := parseLLMResponse(llmResponse)
			report.Summary = llmReport.Summary
			report.Issues = append(report.Issues, llmReport.Issues...)
			report.Recommendations = append(report.Recommendations, llmReport.Recommendations...)
		}
	}

	return report, nil
}

func (te *ToolExecutor) getDefaultRules() *domain.ArchitectureRules {
	return &domain.ArchitectureRules{
		Layers: []domain.LayerRule{
			{Name: "cmd", Paths: []string{"cmd"}, Allow: []string{"main"}},
			{Name: "internal", Paths: []string{"internal"}, Allow: []string{"api", "domain", "service", "repository"}},
			{Name: "pkg", Paths: []string{"pkg"}, Allow: []string{}},
		},
		Dependencies: []domain.DependencyRule{},
		Constraints:  []string{},
	}
}

type ModuleAuditInput struct {
	ToolInput
}

func (te *ToolExecutor) ModuleAudit(ctx context.Context, input ModuleAuditInput) (*domain.AuditReport, error) {
	projectPath := input.ProjectPath
	if projectPath == "" {
		projectPath = te.defaultPath
	}
	if projectPath == "" {
		projectPath = "."
	}

	modulePath := input.ModulePath
	if modulePath == "" {
		modulePath = projectPath
	}

	analyzerEngine := analyzer.New(projectPath)
	report, err := analyzerEngine.AuditModule(modulePath, projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to audit module: %w", err)
	}

	moduleContent, err := te.readModuleContent(modulePath, projectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read module content: %w", err)
	}

	llmProvider, err := te.getLLM(input.Provider, input.LLM)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM: %w", err)
	}
	language := te.getLanguage(input.Language)
	if llmProvider != nil {
		prompt := llm.BuildModuleAuditPrompt(modulePath, moduleContent, language)

		te.mu.RLock()
		debug := te.debug
		logger := te.logger
		te.mu.RUnlock()

		if debug {
			logger.Printf(">>> LLM Request (%s): %s", llmProvider.Name(), prompt[:min(200, len(prompt))])
		}

		llmResponse, err := llmProvider.Complete(ctx, prompt, language)
		if err == nil {
			if debug {
				logger.Printf("<<< LLM Response (%s): %s", llmProvider.Name(), llmResponse[:min(200, len(llmResponse))])
			}
			llmReport := parseLLMResponse(llmResponse)
			report.Score = llmReport.Score
			report.Summary = llmReport.Summary
			report.Issues = append(report.Issues, llmReport.Issues...)
			report.Recommendations = append(report.Recommendations, llmReport.Recommendations...)
		}
	}

	return report, nil
}

func (te *ToolExecutor) readModuleContent(modulePath, projectRoot string) (map[string]string, error) {
	content := make(map[string]string)

	absModulePath := modulePath
	if !filepath.IsAbs(modulePath) {
		absModulePath = filepath.Join(projectRoot, modulePath)
	}
	absModulePath, _ = filepath.Abs(absModulePath)

	info, err := os.Stat(absModulePath)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		err := filepath.Walk(absModulePath, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				if strings.HasPrefix(info.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
				relPath, _ := filepath.Rel(projectRoot, path)
				data, err := os.ReadFile(path)
				if err == nil {
					content[relPath] = string(data)
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			relPath, _ := filepath.Rel(projectRoot, absModulePath)
			data, err := os.ReadFile(absModulePath)
			if err == nil {
				content[relPath] = string(data)
			}
		}
	}

	return content, nil
}