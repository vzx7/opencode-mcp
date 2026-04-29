package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vzx7/opencode-mcp/internal/domain"
)

func TestNew(t *testing.T) {
	a := New("/test/path")
	if a.rootPath != "/test/path" {
		t.Errorf("rootPath = %q, want %q", a.rootPath, "/test/path")
	}
}

func TestBuildProjectMap(t *testing.T) {
	tmpDir := t.TempDir()

	os.MkdirAll(filepath.Join(tmpDir, "cmd", "main"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "cmd", "main", "main.go"), []byte("package main\nfunc main(){}"), 0644)

	os.MkdirAll(filepath.Join(tmpDir, "internal", "api"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "internal", "api", "handler.go"), []byte("package api\nfunc Handler(){}"), 0644)

	os.MkdirAll(filepath.Join(tmpDir, "internal", "service"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "internal", "service", "service.go"), []byte("package service\nfunc Service(){}"), 0644)

	a := New(tmpDir)
	pm, err := a.BuildProjectMap()
	if err != nil {
		t.Fatalf("BuildProjectMap() error = %v", err)
	}

	if pm.Root != tmpDir {
		t.Errorf("Root = %q, want %q", pm.Root, tmpDir)
	}

	if len(pm.Modules) < 2 {
		t.Errorf("Modules len = %d, want >= 2", len(pm.Modules))
	}

	foundCmd := false
	foundInternal := false
	for _, m := range pm.Modules {
		if m.Name == "cmd" {
			foundCmd = true
		}
		if m.Name == "internal" {
			foundInternal = true
		}
	}
	if !foundCmd {
		t.Error("expected cmd module")
	}
	if !foundInternal {
		t.Error("expected internal module")
	}
}

func TestCheckCompliance(t *testing.T) {
	t.Run("all layers present, no violations", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, "cmd"), 0755)
		os.MkdirAll(filepath.Join(tmpDir, "internal", "domain"), 0755)
		os.MkdirAll(filepath.Join(tmpDir, "internal", "service"), 0755)

		a := New(tmpDir)
		pm, _ := a.BuildProjectMap()
		pm.Dependencies = map[string][]string{
			"cmd": {"internal/service"},
		}

		rules := &domain.ArchitectureRules{
			Layers: []domain.LayerRule{
				{Name: "cmd", Paths: []string{"cmd"}, Allow: []string{"service"}},
				{Name: "domain", Paths: []string{"internal/domain"}, Allow: []string{}},
				{Name: "service", Paths: []string{"internal/service"}, Allow: []string{"domain"}},
			},
		}

		report := a.CheckCompliance(rules, pm)
		if report == nil {
			t.Fatal("nil report")
		}
		if len(report.Issues) != 0 {
			t.Errorf("expected 0 issues, got %d: %v", len(report.Issues), report.Issues)
		}
		if report.Score != 100 {
			t.Errorf("Score = %d, want 100", report.Score)
		}
	})

	t.Run("missing layer detected", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, "cmd"), 0755)

		a := New(tmpDir)
		pm, _ := a.BuildProjectMap()

		rules := &domain.ArchitectureRules{
			Layers: []domain.LayerRule{
				{Name: "cmd", Paths: []string{"cmd"}, Allow: []string{}},
				{Name: "domain", Paths: []string{"internal/domain"}, Allow: []string{}},
			},
		}

		report := a.CheckCompliance(rules, pm)
		foundMissing := false
		for _, issue := range report.Issues {
			if strings.Contains(issue.Message, "domain") {
				foundMissing = true
			}
		}
		if !foundMissing {
			t.Error("expected missing layer issue for 'domain'")
		}
	})

	t.Run("forbidden dependency detected", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, "internal", "domain"), 0755)
		os.MkdirAll(filepath.Join(tmpDir, "internal", "service"), 0755)

		a := New(tmpDir)
		pm, _ := a.BuildProjectMap()
		pm.Dependencies = map[string][]string{
			"internal/domain": {"internal/service"},
		}

		rules := &domain.ArchitectureRules{
			Layers: []domain.LayerRule{
				{Name: "domain", Paths: []string{"internal/domain"}, Allow: []string{}},
				{Name: "service", Paths: []string{"internal/service"}, Allow: []string{"domain"}},
			},
			Dependencies: []domain.DependencyRule{
				{From: "domain", To: "service", Reason: "domain must have no internal imports"},
			},
		}

		report := a.CheckCompliance(rules, pm)
		foundForbidden := false
		for _, issue := range report.Issues {
			if issue.Severity == domain.SeverityCritical && strings.Contains(issue.Message, "Forbidden") {
				foundForbidden = true
			}
		}
		if !foundForbidden {
			t.Error("expected critical forbidden dependency issue")
		}
	})

	t.Run("allow_imports_from violation detected", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, "internal", "domain"), 0755)
		os.MkdirAll(filepath.Join(tmpDir, "internal", "llm"), 0755)
		os.MkdirAll(filepath.Join(tmpDir, "internal", "tools"), 0755)

		a := New(tmpDir)
		pm, _ := a.BuildProjectMap()
		// llm imports tools — not allowed
		pm.Dependencies = map[string][]string{
			"internal/llm": {"internal/tools"},
		}

		rules := &domain.ArchitectureRules{
			Layers: []domain.LayerRule{
				{Name: "domain", Paths: []string{"internal/domain"}, Allow: []string{}},
				{Name: "llm", Paths: []string{"internal/llm"}, Allow: []string{"domain"}},
				{Name: "tools", Paths: []string{"internal/tools"}, Allow: []string{"domain", "llm"}},
			},
		}

		report := a.CheckCompliance(rules, pm)
		foundViolation := false
		for _, issue := range report.Issues {
			if issue.Severity == domain.SeverityHigh && strings.Contains(issue.Message, "llm") {
				foundViolation = true
			}
		}
		if !foundViolation {
			t.Error("expected high severity allow_imports_from violation for llm→tools")
		}
	})
}

func TestFindEntrypoints(t *testing.T) {
	tmpDir := t.TempDir()

	os.MkdirAll(filepath.Join(tmpDir, "cmd", "app1"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "cmd", "app1", "main.go"), []byte("package main"), 0644)

	os.MkdirAll(filepath.Join(tmpDir, "cmd", "app2"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "cmd", "app2", "main.go"), []byte("package main"), 0644)

	pm := &domain.ProjectMap{
		Root:         tmpDir,
		Modules:      []domain.Module{},
		Entrypoints:  []string{},
		Dependencies: map[string][]string{},
	}

	a := New(tmpDir)
	a.findEntrypoints(filepath.Join(tmpDir, "cmd"), pm)

	if len(pm.Entrypoints) != 2 {
		t.Errorf("Entrypoints len = %d, want 2", len(pm.Entrypoints))
	}
}

func TestBuildImportGraph(t *testing.T) {
	tmpDir := t.TempDir()

	writeFile(t, tmpDir, "go.mod", "module github.com/test/project\n")
	writeFile(t, tmpDir, "cmd/main/main.go", "package main\nimport \"github.com/test/project/internal/service\"\nfunc main() {}\n")
	writeFile(t, tmpDir, "internal/service/service.go", "package service\nimport \"github.com/test/project/internal/api\"\nfunc Service() {}\n")
	writeFile(t, tmpDir, "internal/api/api.go", "package api\nfunc Api() {}\n")

	a := New(tmpDir)
	graph, err := a.BuildImportGraph()
	if err != nil {
		t.Fatalf("BuildImportGraph() error = %v", err)
	}

	if len(graph.Edges) == 0 {
		t.Error("expected import graph edges")
	}

	if graph.Edges["cmd/main"] == nil {
		t.Error("expected cmd/main in edges")
	}
}

func TestBuildImportGraphCycles(t *testing.T) {
	tmpDir := t.TempDir()

	writeFile(t, tmpDir, "go.mod", "module github.com/test/project\n")
	writeFile(t, tmpDir, "pkg/a/a.go", "package a\nimport \"github.com/test/project/pkg/b\"\nfunc A() {}\n")
	writeFile(t, tmpDir, "pkg/b/b.go", "package b\nimport \"github.com/test/project/pkg/a\"\nfunc B() {}\n")

	a := New(tmpDir)
	graph, err := a.BuildImportGraph()
	if err != nil {
		t.Fatalf("BuildImportGraph() error = %v", err)
	}

	if len(graph.Cycles) == 0 {
		t.Error("expected cycle detection")
	}
}

func TestBuildImportGraphLayerViolations(t *testing.T) {
	tmpDir := t.TempDir()

	writeFile(t, tmpDir, "go.mod", "module github.com/test/project\n")
	writeFile(t, tmpDir, "internal/domain/model.go", "package domain\nfunc Model() {}\n")
	writeFile(t, tmpDir, "internal/tools/tool.go", "package tools\nimport \"github.com/test/project/internal/domain\"\nfunc Tool() {}\n")

	a := New(tmpDir)
	graph, err := a.BuildImportGraph()
	if err != nil {
		t.Fatalf("BuildImportGraph() error = %v", err)
	}

	if len(graph.LayerViolations) > 0 {
		t.Error("expected no layer violations (domain → tools is allowed)")
	}
}

func TestCollectFileMetrics(t *testing.T) {
	tmpDir := t.TempDir()

	writeFile(t, tmpDir, "go.mod", "module github.com/test/project\n")
	content := "package api\nfunc ExportedFunc() {}\ntype ExportedType struct{}\nfunc unexported() {}\n"
	writeFile(t, tmpDir, "internal/api/api.go", content)
	writeFile(t, tmpDir, "internal/api/api_test.go", "package api\nfunc TestApi(t *testing.T) {}\n")

	a := New(tmpDir)
	pm, _ := a.BuildProjectMap()
	metrics, err := a.CollectFileMetrics(pm)
	if err != nil {
		t.Fatalf("CollectFileMetrics() error = %v", err)
	}

	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}

	if metrics[0].ExportedFuncs != 1 {
		t.Errorf("ExportedFuncs = %d, want 1", metrics[0].ExportedFuncs)
	}

	if metrics[0].ExportedTypes != 1 {
		t.Errorf("ExportedTypes = %d, want 1", metrics[0].ExportedTypes)
	}

	if !metrics[0].HasTests {
		t.Error("expected HasTests = true")
	}
}

func writeFile(t *testing.T, dir, path, content string) {
	t.Helper()
	fullPath := filepath.Join(dir, path)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, []byte(content), 0644)
}
