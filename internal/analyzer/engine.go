package analyzer

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

func (a *Analyzer) BuildImportGraph(pm *domain.ProjectMap) (*domain.ImportGraph, error) {
	graph := &domain.ImportGraph{
		Edges:           make(map[string][]string),
		Cycles:          [][]string{},
		LayerViolations: []domain.LayerViolation{},
	}

	modulePrefix := a.getModulePrefix()

	fset := token.NewFileSet()

	err := filepath.Walk(a.rootPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		f, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
		if err != nil {
			return nil
		}

		pkgPath, _ := filepath.Rel(a.rootPath, filepath.Dir(path))
		pkgPath = filepath.ToSlash(pkgPath)

		for _, imp := range f.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			if strings.HasPrefix(importPath, modulePrefix) {
				importPath = strings.TrimPrefix(importPath, modulePrefix)
				importPath = strings.TrimPrefix(importPath, "/")
				graph.Edges[pkgPath] = append(graph.Edges[pkgPath], importPath)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to build import graph: %w", err)
	}

	graph.Cycles = findCycles(graph.Edges)
	graph.LayerViolations = findLayerViolations(graph.Edges)

	return graph, nil
}

func (a *Analyzer) getModulePrefix() string {
	goModPath := filepath.Join(a.rootPath, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.Fields(line)[1]
		}
	}
	return ""
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
// Level N = packages whose longest path from a leaf is N hops.
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
			return 0 // cycle — break recursion
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
			// Violation: a lower-level package imports a higher-level one.
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

func (a *Analyzer) CollectGitHotspots(topN int) ([]domain.GitHotspot, error) {
	cmd := exec.Command("git", "-C", a.rootPath, "log", "--name-only", "--pretty=format:")
	out, err := cmd.Output()
	if err != nil {
		return nil, nil // not a git repo or git unavailable
	}

	counts := make(map[string]int)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && strings.HasSuffix(line, ".go") && !strings.HasSuffix(line, "_test.go") {
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

func (a *Analyzer) CollectFileMetrics(pm *domain.ProjectMap) ([]domain.FileMetric, error) {
	metrics := []domain.FileMetric{}

	err := filepath.Walk(a.rootPath, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if strings.HasPrefix(info.Name(), ".") || info.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}

		relPath, _ := filepath.Rel(a.rootPath, path)
		relPath = filepath.ToSlash(relPath)

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Count(string(src), "\n") + 1
		exportedFuncs, exportedTypes := countExportedSymbols(src)

		hasTests := hasTestFile(path)

		metrics = append(metrics, domain.FileMetric{
			Path:           relPath,
			Lines:          lines,
			ExportedFuncs:  exportedFuncs,
			ExportedTypes:  exportedTypes,
			HasTests:       hasTests,
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to collect file metrics: %w", err)
	}

	return metrics, nil
}

func countExportedSymbols(src []byte) (funcs int, types int) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
	if err != nil {
		return 0, 0
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Name != nil && d.Name.IsExported() {
				funcs++
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if s.Name != nil && s.Name.IsExported() {
						types++
					}
				}
			}
		}
	}

	return
}

func hasTestFile(path string) bool {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := base[:len(base)-len(ext)]
	testName := name + "_test.go"
	testPath := filepath.Join(dir, testName)

	_, err := os.Stat(testPath)
	return err == nil
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