package analyzer

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ai-mcp/code-auditor/internal/domain"
)

type Analyzer struct {
	rootPath string
}

func New(rootPath string) *Analyzer {
	return &Analyzer{rootPath: rootPath}
}

func (a *Analyzer) BuildProjectMap() (*domain.ProjectMap, error) {
	pm := &domain.ProjectMap{
		Root:        a.rootPath,
		Modules:     []domain.Module{},
		Entrypoints: []string{},
		Dependencies: make(map[string][]string),
	}

	err := filepath.Walk(a.rootPath, func(path string, info fs.FileInfo, err error) error {
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
				a.scanGoFiles(path, &mod)
				if mod.GoFiles > 0 {
					pm.Modules = append(pm.Modules, mod)
				}
			}
			if info.Name() == "cmd" {
				a.findEntrypoints(path, pm)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	a.detectLayers(pm)
	a.analyzeGoMod(pm)

	sort.Slice(pm.Modules, func(i, j int) bool {
		return pm.Modules[i].Name < pm.Modules[j].Name
	})
	sort.Slice(pm.Layers, func(i, j int) bool {
		return pm.Layers[i].Name < pm.Layers[j].Name
	})

	return pm, nil
}

func (a *Analyzer) scanGoFiles(dir string, mod *domain.Module) {
	filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
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
			relPath, _ := filepath.Rel(a.rootPath, path)
			mod.Files = append(mod.Files, relPath)
			mod.GoFiles++
		}
		return nil
	})
}

func (a *Analyzer) findEntrypoints(dir string, pm *domain.ProjectMap) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			pm.Entrypoints = append(pm.Entrypoints, filepath.Join(dir, e.Name()))
		}
	}
}

func (a *Analyzer) detectLayers(pm *domain.ProjectMap) {
	layerPatterns := []struct {
		name    string
		prefix  string
	}{
		{"cmd", "cmd"},
		{"internal", "internal"},
		{"pkg", "pkg"},
		{"api", "internal/api"},
		{"domain", "internal/domain"},
		{"service", "internal/service"},
		{"repository", "internal/repository"},
	}

	hasLayer := func(path string) bool {
		for _, l := range pm.Layers {
			for _, p := range l.Paths {
				if strings.HasPrefix(path, p) {
					return true
				}
			}
		}
		return false
	}

	for _, m := range pm.Modules {
		for _, pattern := range layerPatterns {
			if strings.HasPrefix(m.Path, filepath.Join(a.rootPath, pattern.prefix)) && !hasLayer(m.Path) {
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
			}
		}
	}
}

func (a *Analyzer) analyzeGoMod(pm *domain.ProjectMap) {
	goModPath := filepath.Join(a.rootPath, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "require (") || strings.HasPrefix(line, "\trequire") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				dep := parts[1]
				if !strings.HasPrefix(dep, "github.com/ai-mcp/") {
					pm.Dependencies["github.com"] = append(pm.Dependencies["github.com"], dep)
				}
			}
		}
	}
}

func (a *Analyzer) CheckCompliance(rules *domain.ArchitectureRules, pm *domain.ProjectMap) *domain.AuditReport {
	report := &domain.AuditReport{
		Score:          100,
		Summary:        "Architecture compliance check complete",
		Issues:         []domain.Issue{},
		Recommendations: []string{},
	}

	layerPaths := make(map[string][]string)
	for _, layer := range pm.Layers {
		layerPaths[layer.Name] = layer.Paths
	}

	for _, rule := range rules.Layers {
		currentPaths, ok := layerPaths[rule.Name]
		if !ok {
			issue := domain.Issue{
				Severity:  domain.SeverityMedium,
				Message:   fmt.Sprintf("Expected layer '%s' not found", rule.Name),
				Location:  "project structure",
				Suggestion: fmt.Sprintf("Add layer for %s components", rule.Name),
			}
			report.Issues = append(report.Issues, issue)
			report.Score -= 10
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
				issue := domain.Issue{
					Severity:  domain.SeverityLow,
					Message:  fmt.Sprintf("Expected component '%s' in layer '%s'", allowed, rule.Name),
					Location: rule.Name,
					Suggestion: fmt.Sprintf("Consider adding %s", allowed),
				}
				report.Issues = append(report.Issues, issue)
				report.Score -= 5
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

func (a *Analyzer) AuditModule(modulePath string) (*domain.AuditReport, error) {
	report := &domain.AuditReport{
		Score:          80,
		Summary:        "Module audit complete",
		Issues:         []domain.Issue{},
		Recommendations: []string{},
	}

	files, err := os.ReadDir(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read module path: %w", err)
	}

	goFiles := 0
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".go") && !strings.HasSuffix(f.Name(), "_test.go") {
			goFiles++
		}
	}

	if goFiles == 0 {
		report.Issues = append(report.Issues, domain.Issue{
			Severity:  domain.SeverityHigh,
			Message:  "Module has no Go files",
			Location: modulePath,
			Suggestion: "Add Go source files to the module",
		})
		report.Score = 0
		report.Summary = "Module empty or missing source files"
		return report, nil
	}

	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.HasSuffix(f.Name(), ".go") {
			relPath, _ := filepath.Rel(a.rootPath, modulePath)
			report.Recommendations = append(report.Recommendations, fmt.Sprintf("Review: %s", relPath))
		}
	}

	data, _ := json.MarshalIndent(report, "", "  ")
	report.Summary = string(data)

	return report, nil
}