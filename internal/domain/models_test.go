package domain

import (
	"encoding/json"
	"testing"
)

func TestSeverityConstants(t *testing.T) {
	tests := []struct {
		severity Severity
		want     string
	}{
		{SeverityCritical, "critical"},
		{SeverityHigh, "high"},
		{SeverityMedium, "medium"},
		{SeverityLow, "low"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if string(tt.severity) != tt.want {
				t.Errorf("Severity = %q, want %q", tt.severity, tt.want)
			}
		})
	}
}

func TestIssueJSON(t *testing.T) {
	issue := Issue{
		Severity:   SeverityHigh,
		Message:    "Test issue",
		Location:   "test.go",
		Suggestion: "Fix it",
	}

	data, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed Issue
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Severity != issue.Severity {
		t.Errorf("Severity = %q, want %q", parsed.Severity, issue.Severity)
	}
	if parsed.Message != issue.Message {
		t.Errorf("Message = %q, want %q", parsed.Message, issue.Message)
	}
}

func TestAuditReportJSON(t *testing.T) {
	report := AuditReport{
		Score:   85,
		Summary: "Good architecture",
		Issues: []Issue{
			{Severity: SeverityLow, Message: "Minor issue", Location: "test.go", Suggestion: "Fix"},
		},
		Recommendations: []string{"Add tests"},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed AuditReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Score != report.Score {
		t.Errorf("Score = %d, want %d", parsed.Score, report.Score)
	}
	if len(parsed.Issues) != 1 {
		t.Errorf("Issues len = %d, want %d", len(parsed.Issues), 1)
	}
	if len(parsed.Recommendations) != 1 {
		t.Errorf("Recommendations len = %d, want %d", len(parsed.Recommendations), 1)
	}
}

func TestProjectMap(t *testing.T) {
	pm := ProjectMap{
		Root: "/test",
		Modules: []Module{
			{Name: "main", Path: "./cmd/main.go", Type: "cmd", GoFiles: 1},
		},
		Entrypoints: []string{"cmd/main.go"},
		Dependencies: map[string][]string{
			"main": {"internal/api"},
		},
		Layers: []ArchitectureLayer{
			{Name: "cmd", Paths: []string{"cmd"}},
			{Name: "internal", Paths: []string{"internal"}},
		},
	}

	data, err := json.Marshal(pm)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed ProjectMap
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if parsed.Root != pm.Root {
		t.Errorf("Root = %q, want %q", parsed.Root, pm.Root)
	}
	if len(parsed.Modules) != 1 {
		t.Errorf("Modules len = %d, want %d", len(parsed.Modules), 1)
	}
	if len(parsed.Layers) != 2 {
		t.Errorf("Layers len = %d, want %d", len(parsed.Layers), 2)
	}
}

func TestArchitectureRules(t *testing.T) {
	rules := ArchitectureRules{
		Layers: []LayerRule{
			{Name: "cmd", Paths: []string{"cmd"}, Allow: []string{"main"}},
			{Name: "internal", Paths: []string{"internal"}, Allow: []string{"api", "service"}},
		},
		Dependencies: []DependencyRule{
			{From: "cmd", To: "internal", Violation: false},
			{From: "internal", To: "cmd", Violation: true},
		},
		Constraints: []string{"no cyclic dependencies"},
	}

	data, err := json.Marshal(rules)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed ArchitectureRules
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(parsed.Layers) != 2 {
		t.Errorf("Layers len = %d, want %d", len(parsed.Layers), 2)
	}
	if len(parsed.Dependencies) != 2 {
		t.Errorf("Dependencies len = %d, want %d", len(parsed.Dependencies), 2)
	}
	if parsed.Dependencies[1].Violation != true {
		t.Errorf("Dependencies[1].Violation = %v, want true", parsed.Dependencies[1].Violation)
	}
}