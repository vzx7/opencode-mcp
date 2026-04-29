package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/vzx7/opencode-mcp/internal/llm"
)

func TestArchitectureReviewIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0644)

	te := NewToolExecutor(ToolExecutorConfig{
		LLM:     llm.NewMockProvider(),
		Language: "en",
	})

	input := ArchitectureReviewInput{
		ToolInput: ToolInput{
			ProjectPath: tmpDir,
			Provider:    "mock",
			LLM:        "mock",
			Language:    "en",
		},
	}

	report, err := te.ArchitectureReview(context.Background(), input)
	if err != nil {
		t.Fatalf("ArchitectureReview failed: %v", err)
	}

	if report == nil {
		t.Fatal("report is nil")
	}
	if report.Score < 0 || report.Score > 100 {
		t.Errorf("score %d out of range [0,100]", report.Score)
	}
	if report.Summary == "" {
		t.Error("summary is empty")
	}
}

func TestArchitectureComplianceIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0644)

	te := NewToolExecutor(ToolExecutorConfig{
		LLM:     llm.NewMockProvider(),
		Language: "en",
	})

	input := ArchitectureComplianceInput{
		ToolInput: ToolInput{
			ProjectPath: tmpDir,
			Provider:    "mock",
			LLM:        "mock",
			Language:    "en",
		},
	}

	report, err := te.ArchitectureComplianceCheck(context.Background(), input)
	if err != nil {
		t.Fatalf("ArchitectureComplianceCheck failed: %v", err)
	}

	if report == nil {
		t.Fatal("report is nil")
	}
	if report.Score < 0 || report.Score > 100 {
		t.Errorf("score %d out of range [0,100]", report.Score)
	}
	if report.Summary == "" {
		t.Error("summary is empty")
	}
}

func TestModuleAuditIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n"), 0644)

	te := NewToolExecutor(ToolExecutorConfig{
		LLM:     llm.NewMockProvider(),
		Language: "en",
	})

	input := ModuleAuditInput{
		ToolInput: ToolInput{
			ProjectPath: tmpDir,
			Provider:    "mock",
			LLM:        "mock",
			Language:    "en",
		},
	}

	report, err := te.ModuleAudit(context.Background(), input)
	if err != nil {
		t.Fatalf("ModuleAudit failed: %v", err)
	}

	if report == nil {
		t.Fatal("report is nil")
	}
	if report.Score < 0 || report.Score > 100 {
		t.Errorf("score %d out of range [0,100]", report.Score)
	}
	if report.Summary == "" {
		t.Error("summary is empty")
	}
}
