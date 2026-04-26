package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ai-mcp/code-auditor/internal/analyzer"
	"github.com/ai-mcp/code-auditor/internal/domain"
	"github.com/ai-mcp/code-auditor/internal/llm"
)

type ToolExecutor struct {
	defaultPath string
	defaultLLM  llm.LLMProvider
	mu          sync.RWMutex
}

func NewToolExecutor(defaultPath string, defaultLLM llm.LLMProvider) *ToolExecutor {
	return &ToolExecutor{
		defaultPath: defaultPath,
		defaultLLM:  defaultLLM,
	}
}

type ToolInput struct {
	ProjectPath string `json:"project_path"`
	Provider  string `json:"provider"`
	LLM      string `json:"llm"`
}

func (te *ToolExecutor) getLLM(provider, model string) llm.LLMProvider {
	if provider == "" && model == "" {
		return te.defaultLLM
	}

	p := provider
	m := model

	if p == "" {
		p = te.defaultLLM.Name()
	}
	if m == "" {
		switch te.defaultLLM.Name() {
		case "anthropic":
			m = "claude-3-5-sonnet-20241022"
		default:
			m = "gpt-4o"
		}
	}

	newLLM, err := llm.NewProvider(p, "", "", m)
	if err != nil {
		return te.defaultLLM
	}
	return newLLM
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

	llmProvider := te.getLLM(input.Provider, input.LLM)
	prompt := llm.BuildReviewPrompt(pm)
	llmResponse, err := llmProvider.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	report := te.buildReportFromLLM(llmResponse)
	te.enrichWithLocalAnalysis(report, pm)

	return report, nil
}

func (te *ToolExecutor) buildReportFromLLM(response string) *domain.AuditReport {
	return &domain.AuditReport{
		Score:          85,
		Summary:        response,
		Issues:        []domain.Issue{},
		Recommendations: []string{"Review detailed analysis above"},
	}
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

	llmProvider := te.getLLM(input.Provider, input.LLM)
	if llmProvider != nil {
		prompt := llm.BuildCompliancePrompt(rules, pm)
		llmResponse, err := llmProvider.Complete(ctx, prompt)
		if err == nil {
			report.Summary = llmResponse
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
	modulePath := input.ProjectPath
	if modulePath == "" {
		modulePath = te.defaultPath
	}
	if modulePath == "" {
		modulePath = "."
	}

	analyzerEngine := analyzer.New(modulePath)
	report, err := analyzerEngine.AuditModule(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to audit module: %w", err)
	}

	llmProvider := te.getLLM(input.Provider, input.LLM)
	if llmProvider != nil {
		files, _ := json.Marshal(pm{Files: []string{modulePath}})
		prompt := llm.BuildModuleAuditPrompt(modulePath, []string{string(files)})
		llmResponse, err := llmProvider.Complete(ctx, prompt)
		if err == nil {
			report.Summary = llmResponse
		}
	}

	return report, nil
}

type pm struct {
	Files []string `json:"files"`
}