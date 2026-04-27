package analyzer

import (
	"os"
	"path/filepath"
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
	tmpDir := t.TempDir()

	os.MkdirAll(filepath.Join(tmpDir, "cmd", "main"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "cmd", "main", "main.go"), []byte("package main"), 0644)

	os.MkdirAll(filepath.Join(tmpDir, "internal", "api"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "internal", "api", "handler.go"), []byte("package api"), 0644)

	a := New(tmpDir)
	pm, _ := a.BuildProjectMap()

	rules := &domain.ArchitectureRules{
		Layers: []domain.LayerRule{
			{Name: "cmd", Paths: []string{"cmd"}, Allow: []string{"main"}},
			{Name: "internal", Paths: []string{"internal"}, Allow: []string{"api"}},
		},
	}

	report := a.CheckCompliance(rules, pm)

	if report == nil {
		t.Fatal("CheckCompliance returned nil")
	}

	if report.Score < 0 || report.Score > 100 {
		t.Errorf("Score = %d, expected 0-100", report.Score)
	}
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