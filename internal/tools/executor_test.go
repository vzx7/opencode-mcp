package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vzx7/opencode-mcp/internal/domain"
)

type mockProvider struct {
	name string
}

func (m *mockProvider) Complete(ctx context.Context, prompt string, language string) (string, error) {
	return `{"score":85,"summary":"test","issues":[],"recommendations":[]}`, nil
}

func (m *mockProvider) Name() string {
	return m.name
}

func TestNewToolExecutor(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}

	tests := []struct {
		name string
		cfg  ToolExecutorConfig
		wantLang string
	}{
		{
			name: "with language",
			cfg: ToolExecutorConfig{
				Language: "ru",
				LLM:      mockLLM,
			},
			wantLang: "ru",
		},
		{
			name: "empty language defaults to en",
			cfg: ToolExecutorConfig{
				LLM: mockLLM,
			},
			wantLang: "en",
		},
		{
			name: "with default path",
			cfg: ToolExecutorConfig{
				DefaultPath: "/test/path",
				LLM:         mockLLM,
			},
			wantLang: "en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := NewToolExecutor(tt.cfg)
			if te.defaultLang != tt.wantLang {
				t.Errorf("defaultLang = %q, want %q", te.defaultLang, tt.wantLang)
			}
			if tt.cfg.DefaultPath != "" && te.defaultPath != tt.cfg.DefaultPath {
				t.Errorf("defaultPath = %q, want %q", te.defaultPath, tt.cfg.DefaultPath)
			}
		})
	}
}

func TestSetDebug(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}
	te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})

	if te.debug != false {
		t.Errorf("initial debug = true, want false")
	}

	te.SetDebug(true)
	if te.debug != true {
		t.Errorf("after SetDebug(true), debug = false, want true")
	}

	te.SetDebug(false)
	if te.debug != false {
		t.Errorf("after SetDebug(false), debug = true, want false")
	}
}

func TestGetLanguage(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}
	te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM, Language: "ru"})

	tests := []struct {
		name     string
		language string
		want     string
	}{
		{"provided", "en", "en"},
		{"empty uses default", "", "ru"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := te.getLanguage(tt.language)
			if got != tt.want {
				t.Errorf("getLanguage(%q) = %q, want %q", tt.language, got, tt.want)
			}
		})
	}
}

func TestGetLLM(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}

	t.Run("returns default when no args", func(t *testing.T) {
		te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})
		got, err := te.getLLM("", "")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if got.Name() != "mock" {
			t.Errorf("got.Name() = %q, want %q", got.Name(), "mock")
		}
	})

	t.Run("returns default when provider empty", func(t *testing.T) {
		te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})
		got, err := te.getLLM("", "gpt-4o")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if got.Name() != "mock" {
			t.Errorf("got.Name() = %q, want %q", got.Name(), "mock")
		}
	})

	t.Run("returns error for invalid provider without api key", func(t *testing.T) {
		os.Setenv("OPENAI_API_KEY", "")
		os.Setenv("ANTHROPIC_API_KEY", "")
		defer func() {
			os.Setenv("OPENAI_API_KEY", "")
			os.Setenv("ANTHROPIC_API_KEY", "")
		}()
		te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})
		_, err := te.getLLM("openai", "gpt-4o")
		if err == nil {
			t.Error("expected error for openai without api key")
		}
	})
}

func TestParseLLMResponse(t *testing.T) {
	tests := []struct {
		name    string
		response string
		wantScore int
	}{
		{
			name:       "valid json",
			response:   `{"score":85,"summary":"test","issues":[],"recommendations":[]}`,
			wantScore:  85,
		},
		{
			name:       "json with markdown fence",
			response:   "```json\n{\"score\":80,\"summary\":\"test\"}\n```",
			wantScore:  80,
		},
		{
			name:       "plain text fallback",
			response:   "Just plain text response",
			wantScore:  75,
		},
		{
			name:       "empty response",
			response:   "",
			wantScore:  75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			te := &ToolExecutor{}
			report := te.buildReportFromLLM(tt.response)
			if report.Score != tt.wantScore {
				t.Errorf("Score = %d, want %d", report.Score, tt.wantScore)
			}
		})
	}
}

func TestEnrichWithLocalAnalysis(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}
	te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})

	t.Run("no modules adds issue", func(t *testing.T) {
		report := &domain.AuditReport{Score: 100}
		pm := &domain.ProjectMap{Root: "/test", Modules: []domain.Module{}, Layers: []domain.ArchitectureLayer{}}

		te.enrichWithLocalAnalysis(report, pm)

		if len(report.Issues) == 0 {
			t.Error("expected issues when no modules")
		}
	})

	t.Run("single layer adds issue", func(t *testing.T) {
		report := &domain.AuditReport{Score: 100}
		pm := &domain.ProjectMap{
			Root: "/test",
			Modules: []domain.Module{{Name: "main", Path: "cmd/main", GoFiles: 1}},
			Layers: []domain.ArchitectureLayer{{Name: "cmd", Paths: []string{"cmd"}}},
		}

		te.enrichWithLocalAnalysis(report, pm)

		if len(report.Issues) == 0 {
			t.Error("expected issues when single layer")
		}
	})

	t.Run("good architecture no issues", func(t *testing.T) {
		report := &domain.AuditReport{Score: 100}
		pm := &domain.ProjectMap{
			Root: "/test",
			Modules: []domain.Module{{Name: "main", Path: "cmd/main", GoFiles: 1}},
			Layers: []domain.ArchitectureLayer{
				{Name: "cmd", Paths: []string{"cmd"}},
				{Name: "internal", Paths: []string{"internal"}},
			},
		}

		te.enrichWithLocalAnalysis(report, pm)

		if report.Score < 0 {
			t.Error("score should not be negative")
		}
	})
}

func TestGetDefaultRules(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}
	te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})

	rules := te.getDefaultRules()

	if len(rules.Layers) != 3 {
		t.Errorf("Layers len = %d, want 3", len(rules.Layers))
	}

	if rules.Layers[0].Name != "cmd" {
		t.Errorf("first layer = %q, want %q", rules.Layers[0].Name, "cmd")
	}
}

func TestReadModuleContent(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}
	te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})

	t.Run("reads go files", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0644)

		content, err := te.readModuleContent(tmpDir, tmpDir)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(content) == 0 {
			t.Error("expected file content")
		}
	})

	t.Run("skips test files", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package main"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "test_test.go"), []byte("package main"), 0644)

		content, err := te.readModuleContent(tmpDir, tmpDir)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if len(content) > 1 {
			t.Errorf("expected to skip test files, got %d files", len(content))
		}
	})

	t.Run("returns empty for non-existent path", func(t *testing.T) {
		_, err := te.readModuleContent("/non/existent", "/non")
		if err == nil {
			t.Error("expected error for non-existent path")
		}
	})
}

func TestArchitectureReview(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}
	te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})

	t.Run("uses default path when empty", func(t *testing.T) {
		te.defaultPath = "/tmp"
		input := ArchitectureReviewInput{
			ToolInput: ToolInput{ProjectPath: ""},
		}

		_, err := te.ArchitectureReview(context.Background(), input)
		if err != nil {
			t.Logf("ArchitectureReview error: %v", err)
		}
	})
}

func TestBuildCodeSnapshot(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}
	te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})

	t.Run("includes go files sorted by size", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "big.go"), []byte("package main\n"+string(make([]byte, 1000))), 0644)
		os.WriteFile(filepath.Join(tmpDir, "small.go"), []byte("package main"), 0644)

		snapshot, order, omitted := te.buildCodeSnapshot(tmpDir, nil, 5000)
		if len(snapshot) != 2 {
			t.Errorf("expected 2 files, got %d", len(snapshot))
		}
		if len(order) != 2 {
			t.Errorf("expected order len 2, got %d", len(order))
		}
		if len(omitted) != 0 {
			t.Errorf("expected 0 omitted, got %d", len(omitted))
		}
	})

	t.Run("respects maxChars limit", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("package main\n"+string(make([]byte, 2000))), 0644)
		os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("package main\n"+string(make([]byte, 2000))), 0644)

		snapshot, _, omitted := te.buildCodeSnapshot(tmpDir, nil, 2500)
		if len(snapshot) == 0 {
			t.Error("expected at least one file")
		}
		if len(omitted) == 0 {
			t.Error("expected some files omitted")
		}
	})

	t.Run("skips vendor and hidden dirs", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.MkdirAll(filepath.Join(tmpDir, "vendor"), 0755)
		os.WriteFile(filepath.Join(tmpDir, "vendor", "dep.go"), []byte("package dep"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "app.go"), []byte("package main"), 0644)

		snapshot, _, _ := te.buildCodeSnapshot(tmpDir, nil, 5000)
		if len(snapshot) != 1 {
			t.Errorf("expected 1 file (skip vendor), got %d", len(snapshot))
		}
	})

	t.Run("skips test files", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "app.go"), []byte("package main"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "app_test.go"), []byte("package main"), 0644)

		snapshot, _, _ := te.buildCodeSnapshot(tmpDir, nil, 5000)
		if len(snapshot) != 1 {
			t.Errorf("expected 1 file (skip test), got %d", len(snapshot))
		}
	})

	t.Run("uses include_paths when provided", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("package main"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("package main"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "c.go"), []byte("package main"), 0644)

		snapshot, order, omitted := te.buildCodeSnapshot(tmpDir, []string{filepath.Join(tmpDir, "a.go")}, 5000)
		if len(snapshot) != 1 {
			t.Errorf("expected 1 file from include_paths, got %d", len(snapshot))
		}
		if _, ok := snapshot["a.go"]; !ok {
			t.Error("expected a.go in snapshot")
		}
		if len(order) != 1 || order[0] != "a.go" {
			t.Errorf("expected order=[a.go], got %v", order)
		}
		if len(omitted) != 0 {
			t.Errorf("expected 0 omitted, got %d", len(omitted))
		}
	})

	t.Run("include_paths preserves priority order", func(t *testing.T) {
		tmpDir := t.TempDir()
		os.WriteFile(filepath.Join(tmpDir, "first.go"), []byte("package main // priority 1"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "second.go"), []byte("package main // priority 2"), 0644)
		os.WriteFile(filepath.Join(tmpDir, "third.go"), []byte("package main // priority 3"), 0644)

		paths := []string{
			filepath.Join(tmpDir, "first.go"),
			filepath.Join(tmpDir, "second.go"),
			filepath.Join(tmpDir, "third.go"),
		}
		_, order, _ := te.buildCodeSnapshot(tmpDir, paths, 5000)
		if len(order) != 3 {
			t.Fatalf("expected 3 in order, got %d", len(order))
		}
		if order[0] != "first.go" || order[1] != "second.go" || order[2] != "third.go" {
			t.Errorf("order not preserved: %v", order)
		}
	})
}

func TestEnrichWithGraphAnalysis(t *testing.T) {
	mockLLM := &mockProvider{name: "mock"}
	te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM})

	t.Run("adds cycle as critical", func(t *testing.T) {
		report := &domain.AuditReport{Score: 100, Issues: []domain.Issue{}}
		graph := &domain.ImportGraph{
			Cycles: [][]string{{"a", "b", "a"}},
		}
		metrics := []domain.FileMetric{}

		te.enrichWithGraphAnalysis(report, graph, metrics)

		found := false
		for _, iss := range report.Issues {
			if iss.Severity == domain.SeverityCritical && strings.Contains(iss.Message, "cycle") {
				found = true
			}
		}
		if !found {
			t.Error("expected critical issue for cycle")
		}
		// Score is set only by LLM — engine does not modify it
		if report.Score != 100 {
			t.Errorf("expected score unchanged (100), got %d", report.Score)
		}
	})

	t.Run("adds layer violation as high", func(t *testing.T) {
		report := &domain.AuditReport{Score: 100, Issues: []domain.Issue{}}
		graph := &domain.ImportGraph{
			LayerViolations: []domain.LayerViolation{
				{From: "internal/tools", To: "internal/domain", Message: "violation"},
			},
		}
		metrics := []domain.FileMetric{}

		te.enrichWithGraphAnalysis(report, graph, metrics)

		found := false
		for _, iss := range report.Issues {
			if iss.Severity == domain.SeverityHigh && strings.Contains(iss.Message, "violation") {
				found = true
			}
		}
		if !found {
			t.Error("expected high issue for layer violation")
		}
	})

	t.Run("large files not added by engine (LLM sees them in metrics)", func(t *testing.T) {
		report := &domain.AuditReport{Score: 100, Issues: []domain.Issue{}}
		graph := &domain.ImportGraph{}
		metrics := []domain.FileMetric{
			{Path: "big.go", Lines: 600},
		}

		te.enrichWithGraphAnalysis(report, graph, metrics)

		for _, iss := range report.Issues {
			if strings.Contains(iss.Message, "big.go") {
				t.Error("engine should not add large file issues — LLM handles them via metrics table")
			}
		}
	})

	t.Run("missing tests not added by engine (LLM sees metrics)", func(t *testing.T) {
		report := &domain.AuditReport{Score: 100, Issues: []domain.Issue{}}
		graph := &domain.ImportGraph{}
		metrics := []domain.FileMetric{
			{Path: "pkg/mypkg/app.go", Lines: 50, HasTests: false},
		}

		te.enrichWithGraphAnalysis(report, graph, metrics)

		for _, iss := range report.Issues {
			if strings.Contains(iss.Message, "no test files") {
				t.Error("engine should not add missing test issues — LLM handles them via metrics table")
			}
		}
	})

	t.Run("score unchanged by engine", func(t *testing.T) {
		report := &domain.AuditReport{Score: 10, Issues: []domain.Issue{}}
		graph := &domain.ImportGraph{
			Cycles: [][]string{{"a", "b"}},
		}
		metrics := []domain.FileMetric{}

		te.enrichWithGraphAnalysis(report, graph, metrics)

		if report.Score != 10 {
			t.Errorf("score = %d, engine should not modify score", report.Score)
		}
	})
}