package tools

import (
	"context"
	"os"
	"path/filepath"
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
		te := NewToolExecutor(ToolExecutorConfig{LLM: mockLLM, APIKey: ""})
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