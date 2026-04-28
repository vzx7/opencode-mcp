package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/vzx7/opencode-mcp/internal/analyzer"
	"github.com/vzx7/opencode-mcp/internal/domain"
	"github.com/vzx7/opencode-mcp/internal/llm"
)

func validatePath(p string) error {
	if p == "" {
		return nil
	}
	cleaned := filepath.Clean(p)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("path traversal not allowed: %q", p)
	}
	return nil
}

const (
	defaultModelAnthropic = "claude-3-5-sonnet-20241022"
	defaultModelOpenAI    = "gpt-4o"
	warnPromptChars       = 200_000
)

type ToolExecutor struct {
	defaultPath string
	defaultLLM llm.LLMProvider
	defaultLang string
	endpoint  string
	mu        sync.RWMutex
	debug     bool
	logger    *log.Logger
	llmSem    chan struct{} // semaphore for LLM call concurrency limit
}

type ToolExecutorConfig struct {
	DefaultPath string
	LLM       llm.LLMProvider
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
		endpoint:   cfg.Endpoint,
		debug:     cfg.Debug,
		logger:    cfg.Logger,
		llmSem:    make(chan struct{}, 3),
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

func (te *ToolExecutor) acquireLLM(ctx context.Context) error {
	select {
	case te.llmSem <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (te *ToolExecutor) releaseLLM() {
	<-te.llmSem
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

	newLLM, err := llm.NewProvider(p, "", te.endpoint, m)
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
	IncludePaths []string `json:"include_paths,omitempty"`
}

func (te *ToolExecutor) ArchitectureReview(ctx context.Context, input ArchitectureReviewInput) (*domain.AuditReport, error) {
	projectPath := input.ProjectPath
	if projectPath == "" {
		projectPath = te.defaultPath
	}
	if projectPath == "" {
		projectPath = "."
	}

	if err := validatePath(projectPath); err != nil {
		return nil, err
	}

	analyzerEngine := analyzer.New(projectPath)
	pm, err := analyzerEngine.BuildProjectMap()
	if err != nil {
		return nil, fmt.Errorf("failed to build project map: %w", err)
	}

	graph, err := analyzerEngine.BuildImportGraph(pm)
	if err != nil {
		return nil, fmt.Errorf("failed to build import graph: %w", err)
	}

	metrics, err := analyzerEngine.CollectFileMetrics(pm)
	if err != nil {
		return nil, fmt.Errorf("failed to collect file metrics: %w", err)
	}

	snapshot, snapshotOrder, omitted := te.buildCodeSnapshot(projectPath, input.IncludePaths, 100000)

	hotspots, _ := analyzerEngine.CollectGitHotspots(10)

	te.mu.RLock()
	logger := te.logger
	te.mu.RUnlock()

	if logger != nil {
		if len(input.IncludePaths) > 0 {
			logger.Printf("[snapshot] mode=include_paths count=%d", len(input.IncludePaths))
			for _, p := range input.IncludePaths {
				logger.Printf("[snapshot]   + %s", p)
			}
		} else {
			logger.Printf("[snapshot] mode=autodiscover")
		}
		logger.Printf("[snapshot] selected=%d omitted=%d", len(snapshot), len(omitted))
		for i, p := range snapshotOrder {
			logger.Printf("[snapshot]   >> [%d] %s", i+1, p)
		}
		for _, p := range omitted {
			logger.Printf("[snapshot]   -- %s (omitted)", p)
		}
	}

	llmProvider, err := te.getLLM(input.Provider, input.LLM)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM: %w", err)
	}
	if llmProvider == nil {
		return nil, fmt.Errorf("LLM provider is nil")
	}
	language := te.getLanguage(input.Language)
	prompt := llm.BuildReviewPrompt(pm, snapshot, snapshotOrder, omitted, graph, metrics, hotspots, language)

	te.mu.RLock()
	debug := te.debug
	logger = te.logger
	te.mu.RUnlock()

	if len(prompt) > warnPromptChars && logger != nil {
		logger.Printf("[WARN] prompt size %d chars may exceed model context window", len(prompt))
	}

	if debug && logger != nil {
		logger.Printf(">>> LLM Request (%s): %s", llmProvider.Name(), prompt[:min(200, len(prompt))])
	}

	if err := te.acquireLLM(ctx); err != nil {
		return nil, fmt.Errorf("failed to acquire LLM semaphore: %w", err)
	}
	defer te.releaseLLM()

	llmResponse, err := llmProvider.Complete(ctx, prompt, language)
	if err != nil {
		if debug && logger != nil {
			logger.Printf("<<< LLM Error: %v", err)
		}
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	if debug && logger != nil {
		logger.Printf("<<< LLM Response (%s): %s", llmProvider.Name(), llmResponse[:min(200, len(llmResponse))])
	}

	report := te.buildReportFromLLM(llmResponse)
	te.enrichWithLocalAnalysis(report, pm)
	te.enrichWithGraphAnalysis(report, graph, metrics)

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
	}

	if len(pm.Layers) < 2 {
		report.Issues = append(report.Issues, domain.Issue{
			Severity:   domain.SeverityMedium,
			Message:   "Limited architecture layers detected",
			Location:  pm.Root,
			Suggestion: "Consider adopting layered architecture (cmd, internal, pkg)",
		})
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
	}

	if !hasCmd {
		report.Recommendations = append(report.Recommendations, "Consider adding cmd/ for entrypoints")
	}
}

func (te *ToolExecutor) enrichWithGraphAnalysis(report *domain.AuditReport, graph *domain.ImportGraph, metrics []domain.FileMetric) {
	for _, cycle := range graph.Cycles {
		report.Issues = append(report.Issues, domain.Issue{
			Severity:   domain.SeverityCritical,
			Message:    fmt.Sprintf("Import cycle detected: %s", strings.Join(cycle, " → ")),
			Location:   strings.Join(cycle, ", "),
			Suggestion: "Break the cycle by introducing an interface or restructuring dependencies",
		})
	}

	for _, v := range graph.LayerViolations {
		report.Issues = append(report.Issues, domain.Issue{
			Severity:   domain.SeverityHigh,
			Message:    v.Message,
			Location:   v.From,
			Suggestion: fmt.Sprintf("Ensure %s does not depend on %s", v.From, v.To),
		})
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

	if err := validatePath(projectPath); err != nil {
		return nil, err
	}

	analyzerEngine := analyzer.New(projectPath)
	pm, err := analyzerEngine.BuildProjectMap()
	if err != nil {
		return nil, fmt.Errorf("failed to build project map: %w", err)
	}

	graph, _ := analyzerEngine.BuildImportGraph(pm)

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
		prompt := llm.BuildCompliancePrompt(rules, pm, graph, language)

		te.mu.RLock()
		debug := te.debug
		logger := te.logger
		te.mu.RUnlock()

		if len(prompt) > warnPromptChars && logger != nil {
			logger.Printf("[WARN] prompt size %d chars may exceed model context window", len(prompt))
		}

		if debug && logger != nil {
			logger.Printf(">>> LLM Request (%s): %s", llmProvider.Name(), prompt[:min(200, len(prompt))])
		}

		if err := te.acquireLLM(ctx); err != nil {
			return nil, fmt.Errorf("failed to acquire LLM semaphore: %w", err)
		}
		defer te.releaseLLM()

		llmResponse, err := llmProvider.Complete(ctx, prompt, language)
		if err == nil {
			if debug && logger != nil {
				logger.Printf("<<< LLM Response (%s): %s", llmProvider.Name(), llmResponse[:min(200, len(llmResponse))])
			}
			llmReport := parseLLMResponse(llmResponse)
			if llmReport.Score > 0 {
				report.Score = llmReport.Score
			}
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

	if err := validatePath(projectPath); err != nil {
		return nil, err
	}

	modulePath := input.ModulePath
	if modulePath == "" {
		modulePath = projectPath
	}

	if err := validatePath(modulePath); err != nil {
		return nil, err
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

		if len(prompt) > warnPromptChars && logger != nil {
			logger.Printf("[WARN] prompt size %d chars may exceed model context window", len(prompt))
		}

		if debug && logger != nil {
			logger.Printf(">>> LLM Request (%s): %s", llmProvider.Name(), prompt[:min(200, len(prompt))])
		}

		if err := te.acquireLLM(ctx); err != nil {
			return nil, fmt.Errorf("failed to acquire LLM semaphore: %w", err)
		}
		defer te.releaseLLM()

		llmResponse, err := llmProvider.Complete(ctx, prompt, language)
		if err == nil {
			if debug && logger != nil {
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

type fileSize struct {
	path string
	size int64
}

func (te *ToolExecutor) buildCodeSnapshot(projectPath string, includePaths []string, maxChars int) (map[string]string, []string, []string) {
	snapshot := make(map[string]string)
	var order []string
	var omitted []string

	var files []fileSize

	if len(includePaths) > 0 {
		// preserve priority order from caller — no sorting
		for _, p := range includePaths {
			absPath := p
			if !filepath.IsAbs(p) {
				absPath = filepath.Join(projectPath, p)
			}
			info, err := os.Stat(absPath)
			if err != nil || info.IsDir() {
				continue
			}
			if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
				files = append(files, fileSize{path: absPath, size: info.Size()})
			}
		}
	} else {
		filepath.Walk(projectPath, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
				files = append(files, fileSize{path: path, size: info.Size()})
			}
			return nil
		})

		sort.Slice(files, func(i, j int) bool {
			return files[i].size > files[j].size
		})
	}

	totalChars := 0
	for _, f := range files {
		relPath, _ := filepath.Rel(projectPath, f.path)
		relPath = filepath.ToSlash(relPath)

		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}

		chars := len(data)
		if totalChars+chars > maxChars {
			omitted = append(omitted, relPath)
			continue
		}

		snapshot[relPath] = string(data)
		order = append(order, relPath)
		totalChars += chars
	}

	return snapshot, order, omitted
}