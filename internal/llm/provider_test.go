package llm

import (
	"context"
	"strings"
	"testing"

	"github.com/vzx7/opencode-mcp/internal/domain"
)

func TestGetDefaultEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		want     string
	}{
		{"empty defaults to openai", "", "https://api.openai.com/v1/chat/completions"},
		{"openai", "openai", "https://api.openai.com/v1/chat/completions"},
		{"anthropic", "anthropic", "https://api.anthropic.com/v1/messages"},
		{"unknown defaults to openai", "unknown", "https://api.openai.com/v1/chat/completions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getDefaultEndpoint(tt.provider)
			if got != tt.want {
				t.Errorf("getDefaultEndpoint(%q) = %q, want %q", tt.provider, got, tt.want)
			}
		})
	}
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name        string
		provider   string
		apiKey     string
		wantName   string
		wantError bool
	}{
		{"mock no apiKey", "mock", "", "mock", false},
		{"mock empty no apiKey", "", "", "mock", false},
		{"openai with key", "openai", "sk-test", "openai", false},
		{"openai no key fails", "openai", "", "", true},
		{"anthropic with key", "anthropic", "sk-ant", "anthropic", false},
		{"anthropic no key fails", "anthropic", "", "", true},
		{"unknown provider fails", "unknown", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NewProvider(tt.provider, tt.apiKey, "", "")
			if tt.wantError {
				if err == nil {
					t.Error("NewProvider expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("NewProvider unexpected error: %v", err)
				return
			}
			if got.Name() != tt.wantName {
				t.Errorf("NewProvider.Name() = %q, want %q", got.Name(), tt.wantName)
			}
		})
	}
}

func TestExtractToolName(t *testing.T) {
	tests := []struct {
		name    string
		prompt string
		want   string
	}{
		// Marker-based detection
		{"review marker", "[TOOL: architecture_review]", "architecture_review"},
		{"compliance marker", "[TOOL: architecture_compliance]", "architecture_compliance"},
		{"audit marker", "[TOOL: module_audit]", "module_audit"},
		// English fallback
		{"review en", "You are performing an architecture review", "architecture_review"},
		{"compliance en", "You are performing an architecture compliance check", "architecture_compliance"},
		{"audit en", "You are performing a module audit", "module_audit"},
		// Russian fallback
		{"review ru", "Вы выполняете обзор архитектуры", "architecture_review"},
		{"compliance ru", "Вы выполняете проверку соответствия", "architecture_compliance"},
		{"audit ru", "Вы выполняете аудит модуля", "module_audit"},
		// Chinese fallback
		{"review zh", "您正在执行架构审查", "architecture_review"},
		{"compliance zh", "您正在执行架构合规性检查", "architecture_compliance"},
		{"audit zh", "您正在执行模块审计", "module_audit"},
		// Case insensitive
		{"review mixed case", "You are performing an ARCHITECTURE Review", "architecture_review"},
		// Empty returns default
		{"empty prompt", "", "architecture_review"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractToolName(tt.prompt)
			if got != tt.want {
				t.Errorf("extractToolName(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}

func TestMockProviderComplete(t *testing.T) {
	ctx := context.Background()

	t.Run("empty prompt", func(t *testing.T) {
		m := NewMockProvider()
		_, err := m.Complete(ctx, "", "en")
		if err == nil {
			t.Error("expected error for empty prompt")
		}
	})

	t.Run("valid prompt en", func(t *testing.T) {
		m := NewMockProvider()
		resp, err := m.Complete(ctx, "[TOOL: architecture_review]", "en")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if resp == "" {
			t.Error("expected non-empty response")
		}
	})

	t.Run("russian language", func(t *testing.T) {
		m := NewMockProvider()
		resp, err := m.Complete(ctx, "[TOOL: architecture_review]", "ru")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !contains(resp, "85") {
			t.Errorf("expected Russian response with score 85, got: %s", resp)
		}
	})

	t.Run("chinese language", func(t *testing.T) {
		m := NewMockProvider()
		resp, err := m.Complete(ctx, "[TOOL: architecture_review]", "zh")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !contains(resp, "架构") {
			t.Errorf("expected Chinese response, got: %s", resp)
		}
	})

	t.Run("fallback to en", func(t *testing.T) {
		m := NewMockProvider()
		resp, err := m.Complete(ctx, "[TOOL: architecture_review]", "fr")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !contains(resp, "Architecture") {
			t.Errorf("expected English fallback, got: %s", resp)
		}
	})

	t.Run("tool name", func(t *testing.T) {
		m := NewMockProvider()
		if m.Name() != "mock" {
			t.Errorf("Name() = %q, want %q", m.Name(), "mock")
		}
	})
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || (len(s) >= len(substr) && strings.Contains(s, substr)))
}

func TestGetPrompts(t *testing.T) {
	t.Run("returns valid prompts for english", func(t *testing.T) {
		p := GetPrompts("en")
		if p == nil {
			t.Fatal("GetPrompts returned nil for en")
		}
		if p.SystemRole == "" {
			t.Error("expected non-empty system role")
		}
	})

	t.Run("returns valid prompts for russian", func(t *testing.T) {
		p := GetPrompts("ru")
		if p == nil {
			t.Fatal("GetPrompts returned nil for ru")
		}
	})

	t.Run("falls back to english for unknown language", func(t *testing.T) {
		p := GetPrompts("xx")
		en := GetPrompts("en")
		if p.SystemRole != en.SystemRole {
			t.Error("expected fallback to english")
		}
	})
}

func TestGetJSONSchema(t *testing.T) {
	t.Run("returns schema in english", func(t *testing.T) {
		schema := GetJSONSchema("en")
		if !contains(schema, "score") {
			t.Error("expected 'score' in schema")
		}
	})

	t.Run("returns schema in russian", func(t *testing.T) {
		schema := GetJSONSchema("ru")
		if !contains(schema, "score") {
			t.Error("expected 'score' in schema")
		}
	})
}

func TestGetLangLabel(t *testing.T) {
	tests := []struct {
		lang   string
		want   string
	}{
		{"en", "English"},
		{"ru", "Russian"},
		{"zh", "Chinese"},
		{"", "English"},
		{"unknown", "English"},
	}

	for _, tt := range tests {
		t.Run(tt.lang, func(t *testing.T) {
			got := GetLangLabel(tt.lang)
			if got != tt.want {
				t.Errorf("GetLangLabel(%q) = %q, want %q", tt.lang, got, tt.want)
			}
		})
	}
}

func TestBuildPrompts(t *testing.T) {
	t.Run("BuildReviewPrompt contains marker", func(t *testing.T) {
		pm := &domain.ProjectMap{
			Root:    "/test",
			Modules: []domain.Module{{Name: "test", Path: "test.go"}},
			Layers:  []domain.ArchitectureLayer{{Name: "cmd", Paths: []string{"cmd"}}},
		}
		prompt := BuildReviewPrompt(pm, "en")
		if !contains(prompt, ToolMarkerReview) {
			t.Error("expected tool marker in review prompt")
		}
	})

	t.Run("BuildReviewPrompt in russian", func(t *testing.T) {
		pm := &domain.ProjectMap{
			Root:    "/test",
			Modules: []domain.Module{{Name: "test", Path: "test.go"}},
			Layers:  []domain.ArchitectureLayer{{Name: "cmd", Paths: []string{"cmd"}}},
		}
		prompt := BuildReviewPrompt(pm, "ru")
		if !contains(prompt, "Модули:") {
			t.Error("expected russian labels")
		}
	})

	t.Run("BuildCompliancePrompt contains marker", func(t *testing.T) {
		prompt := BuildCompliancePrompt(&domain.ArchitectureRules{
			Layers: []domain.LayerRule{{Name: "cmd", Paths: []string{"cmd"}, Allow: []string{"main"}}},
		}, &domain.ProjectMap{Root: "/test", Layers: []domain.ArchitectureLayer{}}, "en")
		if !contains(prompt, ToolMarkerCompliance) {
			t.Error("expected tool marker in compliance prompt")
		}
	})

	t.Run("BuildModuleAuditPrompt contains marker", func(t *testing.T) {
		files := map[string]string{"test.go": "package main"}
		prompt := BuildModuleAuditPrompt("test.go", files, "en")
		if !contains(prompt, ToolMarkerModuleAudit) {
			t.Error("expected tool marker in audit prompt")
		}
		if !contains(prompt, "test.go") {
			t.Error("expected file path in prompt")
		}
	})
}