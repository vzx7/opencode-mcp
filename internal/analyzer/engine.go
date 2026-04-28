package analyzer

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vzx7/opencode-mcp/internal/domain"
)

const (
	scorePenaltyLayerMissing    = 10
	scorePenaltyComponentMissing = 5
)

var defaultLayerPatterns = []struct {
	name   string
	prefix string
}{
	{"cmd", "cmd"},
	{"internal", "internal"},
	{"pkg", "pkg"},
	{"api", "internal/api"},
	{"domain", "internal/domain"},
	{"service", "internal/service"},
	{"repository", "internal/repository"},
}

// Engine is the language-agnostic analysis orchestrator.
// Language-specific operations are delegated to the embedded ProjectAnalyzer.
type Engine struct {
	rootPath string
	lang     ProjectAnalyzer
}

// New auto-detects the project language from rootPath.
func New(rootPath string) *Engine {
	return &Engine{rootPath: rootPath, lang: Detect(rootPath)}
}

// NewWithLang uses the named language analyzer. Falls back to auto-detection if unknown.
func NewWithLang(rootPath, langName string) *Engine {
	if langName != "" {
		if lang, ok := ByName(langName); ok {
			return &Engine{rootPath: rootPath, lang: lang}
		}
	}
	return &Engine{rootPath: rootPath, lang: Detect(rootPath)}
}

// SnippetLang returns the markdown code block language tag for this project.
func (e *Engine) SnippetLang() string { return e.lang.SnippetLang() }

// LangName returns the detected/selected language name.
func (e *Engine) LangName() string { return e.lang.Name() }

// IsSourceFile returns true if path is a non-test source file for this language.
func (e *Engine) IsSourceFile(path string) bool {
	ext := filepath.Ext(filepath.Base(path))
	for _, se := range e.lang.SourceExtensions() {
		if ext == se && !e.lang.IsTestFile(path) {
			return true
		}
	}
	return false
}

func (e *Engine) BuildProjectMap() (*domain.ProjectMap, error) {
	pm := &domain.ProjectMap{
		Root:         e.rootPath,
		Modules:      []domain.Module{},
		Entrypoints:  []string{},
		Dependencies: make(map[string][]string),
	}

	err := filepath.Walk(e.rootPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			if info.Name() == "internal" || info.Name() == "pkg" || info.Name() == "cmd" {
				moduleType := info.Name()
				mod := domain.Module{
					Name:  moduleType,
					Path:  path,
					Type:  moduleType,
					Files: []string{},
				}
				e.scanSourceFiles(path, &mod)
				if mod.SourceFiles > 0 {
					pm.Modules = append(pm.Modules, mod)
				}
			}
			if info.Name() == "cmd" {
				e.findEntrypoints(path, pm)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	e.addRootModule(pm)
	e.detectLayers(pm)

	sort.Slice(pm.Modules, func(i, j int) bool {
		return pm.Modules[i].Name < pm.Modules[j].Name
	})
	sort.Slice(pm.Layers, func(i, j int) bool {
		return pm.Layers[i].Name < pm.Layers[j].Name
	})

	return pm, nil
}

func (e *Engine) scanSourceFiles(dir string, mod *domain.Module) {
	err := filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if e.IsSourceFile(path) {
			relPath, err := filepath.Rel(e.rootPath, path)
			if err != nil {
				log.Printf("Failed to get relative path for %s: %v", path, err)
				return nil
			}
			mod.Files = append(mod.Files, relPath)
			mod.SourceFiles++
		}
		return nil
	})
	if err != nil {
		log.Printf("Error walking directory %s: %v", dir, err)
	}
}

func (e *Engine) findEntrypoints(dir string, pm *domain.ProjectMap) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	hasSourceFiles := false
	for _, entry := range entries {
		if entry.IsDir() {
			pm.Entrypoints = append(pm.Entrypoints, filepath.Join(dir, entry.Name()))
		} else if e.IsSourceFile(filepath.Join(dir, entry.Name())) {
			hasSourceFiles = true
		}
	}
	if hasSourceFiles {
		pm.Entrypoints = append(pm.Entrypoints, dir)
	}
}

func (e *Engine) addRootModule(pm *domain.ProjectMap) {
	entries, err := os.ReadDir(e.rootPath)
	if err != nil {
		return
	}
	mod := domain.Module{Name: "root", Path: e.rootPath, Type: "main", Files: []string{}}
	for _, entry := range entries {
		if !entry.IsDir() {
			path := filepath.Join(e.rootPath, entry.Name())
			if e.IsSourceFile(path) {
				mod.Files = append(mod.Files, entry.Name())
				mod.SourceFiles++
			}
		}
	}
	if mod.SourceFiles > 0 {
		pm.Modules = append(pm.Modules, mod)
	}
}

func (e *Engine) detectLayers(pm *domain.ProjectMap) {
	existingPaths := make(map[string]bool)
	for _, l := range pm.Layers {
		for _, p := range l.Paths {
			existingPaths[p] = true
		}
	}

	for _, m := range pm.Modules {
		for _, pattern := range defaultLayerPatterns {
			fullPath := filepath.Join(e.rootPath, pattern.prefix)
			if strings.HasPrefix(m.Path, fullPath) && !existingPaths[m.Path] {
				found := false
				for i := range pm.Layers {
					if pm.Layers[i].Name == pattern.name {
						pm.Layers[i].Paths = append(pm.Layers[i].Paths, m.Path)
						found = true
						break
					}
				}
				if !found {
					pm.Layers = append(pm.Layers, domain.ArchitectureLayer{
						Name:  pattern.name,
						Paths: []string{m.Path},
					})
				}
				existingPaths[m.Path] = true
			}
		}
	}
}

// BuildImportGraph delegates edge construction to the language analyzer,
// then computes cycles and layer violations (language-agnostic).
func (e *Engine) BuildImportGraph() (*domain.ImportGraph, error) {
	graph, err := e.lang.BuildImportGraph(e.rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build import graph: %w", err)
	}
	graph.Cycles = findCycles(graph.Edges)
	graph.LayerViolations = findLayerViolations(graph.Edges)
	return graph, nil
}

func findCycles(edges map[string][]string) [][]string {
	cycles := [][]string{}
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	path := []string{}

	var dfs func(node string)
	dfs = func(node string) {
		visited[node] = true
		inStack[node] = true
		path = append(path, node)

		for _, dep := range edges[node] {
			if !visited[dep] {
				dfs(dep)
			} else if inStack[dep] {
				if idx := indexOf(path, dep); idx != -1 {
					cycle := make([]string, len(path)-idx)
					copy(cycle, path[idx:])
					cycles = append(cycles, cycle)
				}
			}
		}

		path = path[:len(path)-1]
		inStack[node] = false
	}

	for node := range edges {
		if !visited[node] {
			dfs(node)
		}
	}

	return cycles
}

func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}

// computePackageLevels assigns a dependency level to each package.
// Level 0 = packages that import no other internal packages (domain-like leaves).
// Packages at a higher level should only import packages at a lower or equal level.
func computePackageLevels(edges map[string][]string) map[string]int {
	allNodes := make(map[string]bool)
	for from, deps := range edges {
		allNodes[from] = true
		for _, to := range deps {
			allNodes[to] = true
		}
	}

	level := make(map[string]int)
	visited := make(map[string]bool)
	inStack := make(map[string]bool)

	var computeLevel func(node string) int
	computeLevel = func(node string) int {
		if inStack[node] {
			return 0
		}
		if visited[node] {
			return level[node]
		}
		visited[node] = true
		inStack[node] = true

		maxDepLevel := -1
		for _, dep := range edges[node] {
			if dl := computeLevel(dep); dl > maxDepLevel {
				maxDepLevel = dl
			}
		}
		level[node] = maxDepLevel + 1
		inStack[node] = false
		return level[node]
	}

	for node := range allNodes {
		if !visited[node] {
			computeLevel(node)
		}
	}

	return level
}

func findLayerViolations(edges map[string][]string) []domain.LayerViolation {
	levels := computePackageLevels(edges)
	violations := []domain.LayerViolation{}

	for from, deps := range edges {
		fromLevel, fromKnown := levels[from]
		if !fromKnown {
			continue
		}
		for _, to := range deps {
			toLevel, toKnown := levels[to]
			if !toKnown {
				continue
			}
			if fromLevel < toLevel {
				violations = append(violations, domain.LayerViolation{
					From:    from,
					To:      to,
					Message: fmt.Sprintf("Layer violation: %s (level %d) imports %s (level %d)", from, fromLevel, to, toLevel),
				})
			}
		}
	}

	return violations
}

func (e *Engine) CollectGitHotspots(topN int) ([]domain.GitHotspot, error) {
	cmd := exec.Command("git", "-C", e.rootPath, "log", "--name-only", "--pretty=format:")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}

	counts := make(map[string]int)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && e.IsSourceFile(line) {
			counts[line]++
		}
	}

	type pair struct {
		path    string
		commits int
	}
	pairs := make([]pair, 0, len(counts))
	for path, cnt := range counts {
		pairs = append(pairs, pair{path, cnt})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].commits > pairs[j].commits
	})

	n := topN
	if n > len(pairs) {
		n = len(pairs)
	}
	hotspots := make([]domain.GitHotspot, n)
	for i := 0; i < n; i++ {
		hotspots[i] = domain.GitHotspot{Path: pairs[i].path, Commits: pairs[i].commits}
	}
	return hotspots, nil
}

func (e *Engine) CollectFileMetrics(pm *domain.ProjectMap) ([]domain.FileMetric, error) {
	metrics := []domain.FileMetric{}

	err := filepath.Walk(e.rootPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !e.IsSourceFile(path) {
			return nil
		}

		relPath, _ := filepath.Rel(e.rootPath, path)
		relPath = filepath.ToSlash(relPath)

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Count(string(src), "\n") + 1
		exportedFuncs, exportedTypes := e.lang.CountSymbols(src)
		hasTests := e.lang.HasTestCounterpart(path)

		metrics = append(metrics, domain.FileMetric{
			Path:          relPath,
			Lines:         lines,
			ExportedFuncs: exportedFuncs,
			ExportedTypes: exportedTypes,
			HasTests:      hasTests,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to collect file metrics: %w", err)
	}

	return metrics, nil
}

func (e *Engine) CheckCompliance(rules *domain.ArchitectureRules, pm *domain.ProjectMap) *domain.AuditReport {
	report := &domain.AuditReport{
		Score:           100,
		Summary:         "Architecture compliance check complete",
		Issues:          []domain.Issue{},
		Recommendations: []string{},
	}

	layerPaths := make(map[string][]string)
	for _, layer := range pm.Layers {
		layerPaths[layer.Name] = layer.Paths
	}

	for _, rule := range rules.Layers {
		currentPaths, ok := layerPaths[rule.Name]
		if !ok {
			report.Issues = append(report.Issues, domain.Issue{
				Severity:   domain.SeverityMedium,
				Message:    fmt.Sprintf("Expected layer '%s' not found", rule.Name),
				Location:   "project structure",
				Suggestion: fmt.Sprintf("Add layer for %s components", rule.Name),
			})
			report.Score -= scorePenaltyLayerMissing
			continue
		}

		for _, allowed := range rule.Allow {
			found := false
			for _, path := range currentPaths {
				if strings.Contains(path, allowed) {
					found = true
					break
				}
			}
			if !found {
				report.Issues = append(report.Issues, domain.Issue{
					Severity:   domain.SeverityLow,
					Message:    fmt.Sprintf("Expected component '%s' in layer '%s'", allowed, rule.Name),
					Location:   rule.Name,
					Suggestion: fmt.Sprintf("Consider adding %s", allowed),
				})
				report.Score -= scorePenaltyComponentMissing
			}
		}
	}

	if len(report.Issues) == 0 {
		report.Summary = "Architecture compliance check passed. All layers properly defined."
	} else {
		report.Summary = fmt.Sprintf("Found %d architecture violations", len(report.Issues))
	}

	if report.Score < 0 {
		report.Score = 0
	}

	return report
}

func (e *Engine) AuditModule(modulePath, projectRoot string) (*domain.AuditReport, error) {
	report := &domain.AuditReport{
		Score:           100,
		Summary:         "Module audit complete",
		Issues:          []domain.Issue{},
		Recommendations: []string{},
	}

	absModulePath := modulePath
	if !filepath.IsAbs(modulePath) {
		absModulePath = filepath.Join(projectRoot, modulePath)
	}
	absModulePath, _ = filepath.Abs(absModulePath)

	info, err := os.Stat(absModulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read module path: %w", err)
	}

	var sourceFiles []string

	if info.IsDir() {
		err := filepath.Walk(absModulePath, func(p string, fi fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				if strings.HasPrefix(fi.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if e.IsSourceFile(p) {
				rel, _ := filepath.Rel(absModulePath, p)
				sourceFiles = append(sourceFiles, rel)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk module: %w", err)
		}
	} else {
		if e.IsSourceFile(absModulePath) {
			sourceFiles = append(sourceFiles, info.Name())
		}
	}

	if len(sourceFiles) == 0 {
		report.Issues = append(report.Issues, domain.Issue{
			Severity:   domain.SeverityHigh,
			Message:    "Module has no source files",
			Location:   modulePath,
			Suggestion: "Add source files to the module",
		})
		report.Score = 0
		report.Summary = "Module empty or missing source files"
		return report, nil
	}

	relPath, _ := filepath.Rel(projectRoot, absModulePath)
	report.Recommendations = append(report.Recommendations, fmt.Sprintf("Review: %s", relPath))
	report.Recommendations = append(report.Recommendations, fmt.Sprintf("Found %d source files", len(sourceFiles)))

	return report, nil
}
