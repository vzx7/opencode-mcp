package analyzer

import (
	"bufio"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vzx7/opencode-mcp/internal/domain"
)

const (
	scorePenaltyLayerMissing = 10
	scorePenaltyComponentMissing = 5
)

var defaultLayerPatterns = []struct {
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

	a.addRootModule(pm)
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
		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			relPath, err := filepath.Rel(a.rootPath, path)
			if err != nil {
				log.Printf("Failed to get relative path for %s: %v", path, err)
				return nil
			}
			mod.Files = append(mod.Files, relPath)
			mod.GoFiles++
		}
		return nil
})
	if err != nil {
		log.Printf("Error walking directory %s: %v", dir, err)
	}
}

func (a *Analyzer) findEntrypoints(dir string, pm *domain.ProjectMap) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	hasGoFiles := false
	for _, e := range entries {
		if e.IsDir() {
			pm.Entrypoints = append(pm.Entrypoints, filepath.Join(dir, e.Name()))
		} else if strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			hasGoFiles = true
		}
	}
	// cmd/main.go directly inside cmd/ (no subdirectory per binary)
	if hasGoFiles {
		pm.Entrypoints = append(pm.Entrypoints, dir)
	}
}

func (a *Analyzer) addRootModule(pm *domain.ProjectMap) {
	entries, err := os.ReadDir(a.rootPath)
	if err != nil {
		return
	}
	mod := domain.Module{Name: "root", Path: a.rootPath, Type: "main", Files: []string{}}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			mod.Files = append(mod.Files, e.Name())
			mod.GoFiles++
		}
	}
	if mod.GoFiles > 0 {
		pm.Modules = append(pm.Modules, mod)
	}
}

func (a *Analyzer) detectLayers(pm *domain.ProjectMap) {
	existingPaths := make(map[string]bool)
	for _, l := range pm.Layers {
		for _, p := range l.Paths {
			existingPaths[p] = true
		}
	}

	for _, m := range pm.Modules {
		for _, pattern := range defaultLayerPatterns {
			fullPath := filepath.Join(a.rootPath, pattern.prefix)
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

func (a *Analyzer) analyzeGoMod(pm *domain.ProjectMap) {
	goModPath := filepath.Join(a.rootPath, "go.mod")
	file, err := os.Open(goModPath)
	if err != nil {
		return
	}
	defer file.Close()

	inRequireBlock := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		if strings.HasPrefix(line, "require (") {
			inRequireBlock = true
			continue
		}
		if line == ")" {
			inRequireBlock = false
			continue
		}
		
		var dep string
		switch {
		case inRequireBlock:
			// "github.com/foo/bar v1.2.3 // indirect" → parts[0] is the package
			parts := strings.Fields(line)
			if len(parts) >= 1 && !strings.HasPrefix(parts[0], "//") {
				dep = parts[0]
			}
		case strings.HasPrefix(line, "require "):
			// "require github.com/foo/bar v1.2.3" → parts[1] is the package
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				dep = parts[1]
			}
		}
		if dep != "" && !strings.HasPrefix(dep, "github.com/vzx7/") {
			pm.Dependencies["github.com"] = append(pm.Dependencies["github.com"], dep)
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
				issue := domain.Issue{
					Severity:  domain.SeverityLow,
					Message:  fmt.Sprintf("Expected component '%s' in layer '%s'", allowed, rule.Name),
					Location: rule.Name,
					Suggestion: fmt.Sprintf("Consider adding %s", allowed),
				}
				report.Issues = append(report.Issues, issue)
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

func (a *Analyzer) AuditModule(modulePath, projectRoot string) (*domain.AuditReport, error) {
	report := &domain.AuditReport{
		Score:          100,
		Summary:        "Module audit complete",
		Issues:         []domain.Issue{},
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

	var goFiles []string

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
			if strings.HasSuffix(fi.Name(), ".go") && !strings.HasSuffix(fi.Name(), "_test.go") {
				rel, _ := filepath.Rel(absModulePath, p)
				goFiles = append(goFiles, rel)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("failed to walk module: %w", err)
		}
	} else {
		if strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go") {
			goFiles = append(goFiles, info.Name())
		}
	}

	if len(goFiles) == 0 {
		report.Issues = append(report.Issues, domain.Issue{
			Severity:   domain.SeverityHigh,
			Message:    "Module has no Go files",
			Location:  modulePath,
			Suggestion: "Add Go source files to the module",
		})
		report.Score = 0
		report.Summary = "Module empty or missing source files"
		return report, nil
	}

	relPath, _ := filepath.Rel(projectRoot, absModulePath)
	report.Recommendations = append(report.Recommendations, fmt.Sprintf("Review: %s", relPath))
	report.Recommendations = append(report.Recommendations, fmt.Sprintf("Found %d Go files", len(goFiles)))

	return report, nil
}