package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTimeout = 10 * time.Minute
	MaxRetries   = 3
	InitialDelay = 1 * time.Second
)

type OpenAIProvider struct {
	apiKey   string
	endpoint string
	model   string
	client  *http.Client
}

func NewOpenAIProvider(apiKey string, endpoint string, model string) *OpenAIProvider {
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	if model == "" {
		model = "gpt-4o"
	}
	return &OpenAIProvider{
		apiKey:   apiKey,
		endpoint: endpoint,
		model:   model,
		client:  &http.Client{Timeout: DefaultTimeout},
	}
}

type openAIRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	MaxTokens int      `json:"max_tokens,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message message `json:"message"`
}

func (p *OpenAIProvider) Complete(ctx context.Context, prompt string, language string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("API key required for OpenAI provider")
	}

	reqBody := openAIRequest{
		Model: p.model,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 4000,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	delay := InitialDelay

	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = nil
		var result string
		result, lastErr = p.doRequest(ctx, body)
		if lastErr == nil {
			return result, nil
		}

		if rErr, ok := lastErr.(*retryableError); ok && rErr.retryAfter > 0 {
			delay = rErr.retryAfter
		} else if !isRetryableError(lastErr) {
			return "", lastErr
		} else if delay < InitialDelay*4 {
			delay *= 2
		}
	}

	return "", fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (p *OpenAIProvider) doRequest(ctx context.Context, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode >= 500 {
		return "", &retryableError{statusCode: resp.StatusCode, retryAfter: parseRetryAfter(resp.Header)}
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error: %s", string(respBody))
	}

	var result openAIResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	return result.Choices[0].Message.Content, nil
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*retryableError); ok {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "request failed") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "i/o timeout")
}

type retryableError struct {
	statusCode int
	retryAfter time.Duration
}

func (e *retryableError) Error() string {
	if e.retryAfter > 0 {
		return fmt.Sprintf("retryable error: status %d, retry after %v", e.statusCode, e.retryAfter)
	}
	return fmt.Sprintf("retryable error: status %d", e.statusCode)
}

func parseRetryAfter(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if d, err := strconv.Atoi(v); err == nil {
			return time.Duration(d) * time.Second
		}
	}
	return 0
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

type AnthropicProvider struct {
	apiKey   string
	endpoint string
	model    string
	client   *http.Client
}

func NewAnthropicProvider(apiKey string, endpoint string, model string) *AnthropicProvider {
	if endpoint == "" {
		endpoint = "https://api.anthropic.com/v1/messages"
	}
	if model == "" {
		model = "claude-3-5-sonnet-20241022"
	}
	return &AnthropicProvider{
		apiKey:   apiKey,
		endpoint: endpoint,
		model:    model,
		client:   &http.Client{Timeout: DefaultTimeout},
	}
}

type anthropicRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
	MaxTokens int     `json:"max_tokens,omitempty"`
}

type anthropicResponse struct {
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text"`
}

func (p *AnthropicProvider) Complete(ctx context.Context, prompt string, language string) (string, error) {
	if p.apiKey == "" {
		return "", fmt.Errorf("API key required for Anthropic provider")
	}

	reqBody := anthropicRequest{
		Model: p.model,
		Messages: []message{
			{Role: "user", Content: prompt},
		},
		MaxTokens: 4096,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	var lastErr error
	delay := InitialDelay

	for attempt := 0; attempt < MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(delay):
			}
		}

		lastErr = nil
		var result string
		result, lastErr = p.doRequest(ctx, body)
		if lastErr == nil {
			return result, nil
		}

		if rErr, ok := lastErr.(*retryableError); ok && rErr.retryAfter > 0 {
			delay = rErr.retryAfter
		} else if !isRetryableError(lastErr) {
			return "", lastErr
		} else if delay < InitialDelay*4 {
			delay *= 2
		}
	}

	return "", fmt.Errorf("max retries exceeded: %w", lastErr)
}

func (p *AnthropicProvider) doRequest(ctx context.Context, body []byte) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", p.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode >= 500 {
		return "", &retryableError{statusCode: resp.StatusCode, retryAfter: parseRetryAfter(resp.Header)}
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API error: %s", string(respBody))
	}

	var result anthropicResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(result.Content) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in response")
}

func (p *AnthropicProvider) Name() string {
	return "anthropic"
}

type notImplementedError struct {
	msg string
}

func (e *notImplementedError) Error() string {
	return e.msg
}

// relPath returns path relative to root. Falls back to filepath.Base(path) if
// the path cannot be made relative (e.g. different drives on Windows).
func relPath(root, path string) string {
	if root == "" || path == "" {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.Base(path)
	}
	return filepath.ToSlash(rel)
}

func BuildReviewPrompt(projectMap *ProjectMap, snapshot map[string]string, snapshotOrder []string, omitted []string, graph *ImportGraph, metrics []FileMetric, hotspots []GitHotspot, snippetLang string, language string) string {
	var sb strings.Builder

	p := GetPrompts(language)
	if p == nil {
		p = &Prompts{
			SystemRole: "You are a software architect reviewing code.",
			Labels: map[string]string{
				"root":              "Root",
				"modules":           "Modules",
				"layers":            "Layers",
				"snapshot_section":   "Snapshot",
				"snapshot_order_note": "Files are ordered by size (largest first).",
				"omitted_note":       "%d files omitted (over %d chars): %s",
				"import_graph_section": "Import Graph",
				"metrics_section":      "File Metrics",
				"hotspots_section":     "Git Hotspots",
			},
		}
	}

	sb.WriteString(ToolMarkerReview)
	sb.WriteString("\n")
	sb.WriteString(p.SystemRole)
	sb.WriteString("\n\n## " + p.Labels["root"] + " ")
	sb.WriteString(filepath.Base(projectMap.Root))
	sb.WriteString("\n\n")
	sb.WriteString(p.Labels["modules"])
	sb.WriteString("\n")
	for _, m := range projectMap.Modules {
		sb.WriteString("- ")
		sb.WriteString(m.Name)
		sb.WriteString(": ")
		sb.WriteString(relPath(projectMap.Root, m.Path))
		sb.WriteString("\n")
	}
	sb.WriteString("\n" + p.Labels["layers"] + "\n")
	for _, l := range projectMap.Layers {
		sb.WriteString("- ")
		sb.WriteString(l.Name)
		sb.WriteString(": ")
		relPaths := make([]string, len(l.Paths))
		for i, p := range l.Paths {
			relPaths[i] = relPath(projectMap.Root, p)
		}
		sb.WriteString(strings.Join(relPaths, ", "))
		sb.WriteString("\n")
	}

	sb.WriteString("\n" + p.Labels["snapshot_section"] + "\n")
	sb.WriteString(p.Labels["snapshot_order_note"] + "\n\n")
	for _, path := range snapshotOrder {
		content := snapshot[path]
		sb.WriteString("### ")
		sb.WriteString(path)
		sb.WriteString("\n```")
		sb.WriteString(snippetLang)
		sb.WriteString("\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n\n")
	}
	if len(omitted) > 0 {
		sb.WriteString(fmt.Sprintf("> "+p.Labels["omitted_note"]+"\n\n",
			len(omitted), strings.Join(omitted, ", ")))
	}

	sb.WriteString("\n" + p.Labels["import_graph_section"] + "\n")
	for pkg, imports := range graph.Edges {
		sb.WriteString("- ")
		sb.WriteString(pkg)
		sb.WriteString(" → ")
		sb.WriteString(strings.Join(imports, ", "))
		sb.WriteString("\n")
	}

	if len(graph.Cycles) > 0 {
		sb.WriteString("\n" + p.Labels["cycles_detected"] + "\n")
		for _, cycle := range graph.Cycles {
			sb.WriteString("- ")
			sb.WriteString(strings.Join(cycle, " → "))
			sb.WriteString("\n")
		}
	}

	if len(graph.LayerViolations) > 0 {
		sb.WriteString("\n" + p.Labels["layer_violations"] + "\n")
		for _, v := range graph.LayerViolations {
			sb.WriteString("- ")
			sb.WriteString(v.From)
			sb.WriteString(" → ")
			sb.WriteString(v.To)
			sb.WriteString(": ")
			sb.WriteString(v.Message)
			sb.WriteString("\n")
		}
	}

	sb.WriteString("\n" + p.Labels["file_metrics_section"] + "\n")
	sb.WriteString(p.Labels["metrics_table_header"] + "\n")
	sb.WriteString("|------|-------|----------------|----------------|----------|\n")
	for _, m := range metrics {
		if m.Lines > 500 {
			sb.WriteString("| **")
			sb.WriteString(m.Path)
			sb.WriteString("** " + p.Labels["large_file_marker"] + " | ")
		} else {
			sb.WriteString("| ")
			sb.WriteString(m.Path)
			sb.WriteString(" | ")
		}
		sb.WriteString(fmt.Sprintf("%d | %d | %d | %v |\n", m.Lines, m.ExportedFuncs, m.ExportedTypes, m.HasTests))
	}

	if len(hotspots) > 0 {
		sb.WriteString("\n" + p.Labels["hotspots_section"] + "\n")
		sb.WriteString(p.Labels["hotspots_note"] + "\n\n")
		for _, h := range hotspots {
			sb.WriteString(fmt.Sprintf("- %s (%d commits)\n", h.Path, h.Commits))
		}
	}

	sb.WriteString("\n" + p.Labels["review_intro"] + "\n")
	for i, focus := range p.ReviewFocus {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, focus))
	}
	sb.WriteString("\n")
	if rubric := p.Labels["scoring_rubric"]; rubric != "" {
		sb.WriteString(rubric)
		sb.WriteString("\n\n")
	}
	sb.WriteString(p.JSONSchemaInstruction)
	sb.WriteString("\n")
	sb.WriteString(GetJSONSchema(language))
	return sb.String()
}

func appendJSONSchema(sb *strings.Builder, language string) {
	p := GetPrompts(language)
	sb.WriteString("\n\n" + p.JSONSchemaInstruction)
	sb.WriteString("\n")
	sb.WriteString(GetJSONSchema(language))
}

func BuildCompliancePrompt(
	rules *ArchitectureRules,
	projectMap *ProjectMap,
	graph *ImportGraph,
	docsContent map[string]string,
	docsOrder []string,
	snapshot map[string]string,
	snapshotOrder []string,
	snapshotOmitted []string,
	snippetLang string,
	language string,
) string {
	var sb strings.Builder

	p := GetPrompts(language)
	if p == nil {
		p = &Prompts{
			SystemRole: "You are a software architect reviewing architecture compliance.",
			Labels: map[string]string{
				"compliance_rules_section":    "Compliance Rules",
				"compliance_layer_label":       "Layer",
				"paths":                       "Paths",
				"allowed":                     "Allowed",
				"compliance_structure_section": "Project Structure",
				"root":                        "Root",
				"layers":                      "Layers",
				"import_graph_section":        "Import Graph",
				"docs_section":                "## Architecture Documentation",
				"docs_note":                   "The following documentation describes the intended architecture.",
				"compliance_snapshot_section": "## Key Source Files",
				"compliance_snapshot_note":    "Architecturally significant files selected for compliance review:",
				"compliance_omitted_note":     "%d file(s) omitted from snapshot: %s",
			},
		}
	}
	c := p.Compliance
	if c.SystemRole == "" {
		c.SystemRole = "You are a software architect reviewing architecture compliance."
	}

	sb.WriteString(ToolMarkerCompliance)
	sb.WriteString("\n")
	sb.WriteString(c.SystemRole)

	// Documentation section — ground truth for compliance evaluation
	if len(docsOrder) > 0 {
		sb.WriteString("\n\n" + p.Labels["docs_section"] + "\n")
		sb.WriteString(p.Labels["docs_note"] + "\n\n")
		for _, path := range docsOrder {
			content, ok := docsContent[path]
			if !ok {
				continue
			}
			sb.WriteString("### ")
			sb.WriteString(path)
			sb.WriteString("\n\n")
			sb.WriteString(content)
			sb.WriteString("\n\n")
		}
	}

	// Rules section — full content from .architecture.json
	sb.WriteString("\n" + p.Labels["compliance_rules_section"] + "\n")
	if rules.Description != "" {
		sb.WriteString(rules.Description + "\n")
	}
	sb.WriteString("\n### Layers\n")
	for _, layer := range rules.Layers {
		sb.WriteString(p.Labels["compliance_layer_label"] + " **")
		sb.WriteString(layer.Name)
		sb.WriteString("**")
		if layer.Description != "" {
			sb.WriteString(" — ")
			sb.WriteString(layer.Description)
		}
		sb.WriteString("\n  " + p.Labels["paths"] + " ")
		sb.WriteString(strings.Join(layer.Paths, ", "))
		if len(layer.Allow) > 0 {
			sb.WriteString("\n  " + p.Labels["allowed"] + " ")
			sb.WriteString(strings.Join(layer.Allow, ", "))
		} else {
			sb.WriteString("\n  " + p.Labels["allowed"] + " (none)")
		}
		sb.WriteString("\n")
	}
	if len(rules.Dependencies) > 0 {
		sb.WriteString("\n### Forbidden Dependencies\n")
		for _, dep := range rules.Dependencies {
			sb.WriteString("- ")
			sb.WriteString(dep.From)
			sb.WriteString(" → ")
			sb.WriteString(dep.To)
			if dep.Reason != "" {
				sb.WriteString(": ")
				sb.WriteString(dep.Reason)
			}
			sb.WriteString("\n")
		}
	}
	if len(rules.Constraints) > 0 {
		sb.WriteString("\n### Architecture Constraints\n")
		for _, c := range rules.Constraints {
			sb.WriteString("- ")
			sb.WriteString(c)
			sb.WriteString("\n")
		}
	}

	// Project structure
	sb.WriteString("\n" + p.Labels["compliance_structure_section"] + "\n")
	sb.WriteString(p.Labels["root"] + " ")
	sb.WriteString(filepath.Base(projectMap.Root))
	sb.WriteString("\n\n" + p.Labels["layers"] + "\n")
	for _, l := range projectMap.Layers {
		sb.WriteString("- ")
		sb.WriteString(l.Name)
		sb.WriteString(": ")
		layerRelPaths := make([]string, len(l.Paths))
		for i, lp := range l.Paths {
			layerRelPaths[i] = relPath(projectMap.Root, lp)
		}
		sb.WriteString(strings.Join(layerRelPaths, ", "))
		sb.WriteString("\n")
	}

	// Code snapshot — key source files
	if len(snapshotOrder) > 0 {
		sb.WriteString("\n" + p.Labels["compliance_snapshot_section"] + "\n")
		sb.WriteString(p.Labels["compliance_snapshot_note"] + "\n\n")
		for _, path := range snapshotOrder {
			content := snapshot[path]
			sb.WriteString("### ")
			sb.WriteString(path)
			sb.WriteString("\n```")
			sb.WriteString(snippetLang)
			sb.WriteString("\n")
			sb.WriteString(content)
			sb.WriteString("\n```\n\n")
		}
		if len(snapshotOmitted) > 0 {
			sb.WriteString(fmt.Sprintf("> "+p.Labels["compliance_omitted_note"]+"\n\n",
				len(snapshotOmitted), strings.Join(snapshotOmitted, ", ")))
		}
	}

	// Import graph
	if graph != nil {
		sb.WriteString("\n" + p.Labels["import_graph_section"] + "\n")
		for pkg, imports := range graph.Edges {
			sb.WriteString("- ")
			sb.WriteString(pkg)
			sb.WriteString(" → ")
			sb.WriteString(strings.Join(imports, ", "))
			sb.WriteString("\n")
		}
		if len(graph.Cycles) > 0 {
			sb.WriteString("\n" + p.Labels["cycles_detected"] + "\n")
			for _, cycle := range graph.Cycles {
				sb.WriteString("- ")
				sb.WriteString(strings.Join(cycle, " → "))
				sb.WriteString("\n")
			}
		}
		if len(graph.LayerViolations) > 0 {
			sb.WriteString("\n" + p.Labels["layer_violations"] + "\n")
			for _, v := range graph.LayerViolations {
				sb.WriteString("- ")
				sb.WriteString(v.From)
				sb.WriteString(" → ")
				sb.WriteString(v.To)
				sb.WriteString(": ")
				sb.WriteString(v.Message)
				sb.WriteString("\n")
			}
		}
	}

	sb.WriteString("\n" + c.SeverityGuide)
	sb.WriteString("\n\n" + c.IdentifyViolations)
	sb.WriteString("\n" + c.ReturnFormat)
	sb.WriteString("\n")
	sb.WriteString(p.JSONSchemaInstruction)
	sb.WriteString("\n")
	sb.WriteString(GetJSONSchema(language))
	return sb.String()
}

func BuildModuleAuditPrompt(modulePath string, files map[string]string, snippetLang string, language string) string {
	var sb strings.Builder

	p := GetPrompts(language)
	if p == nil {
		p = &Prompts{
			SystemRole: "You are a software architect auditing a module.",
			Labels: map[string]string{
				"module_path_label":   "Module Path",
				"module_source_label": "Module Source",
			},
		}
	}
	ma := p.ModuleAudit
	if ma.SystemRole == "" {
		ma.SystemRole = "You are a software architect auditing a module."
	}

	sb.WriteString(ToolMarkerModuleAudit)
	sb.WriteString("\n")
	sb.WriteString(ma.SystemRole)
	sb.WriteString("\n\n" + p.Labels["module_path_label"] + " ")
	sb.WriteString(modulePath)
	sb.WriteString("\n\n" + p.Labels["module_source_label"] + "\n")
	for path, content := range files {
		sb.WriteString("### ")
		sb.WriteString(path)
		sb.WriteString("\n```")
		sb.WriteString(snippetLang)
		sb.WriteString("\n")
		sb.WriteString(content)
		sb.WriteString("\n```\n\n")
	}
	if len(ma.Focus) > 0 {
		sb.WriteString(p.Labels["review_intro"] + "\n")
		for i, focus := range ma.Focus {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, focus))
		}
		sb.WriteString("\n")
	}
	if ma.ScoringRubric != "" {
		sb.WriteString(ma.ScoringRubric)
		sb.WriteString("\n\n")
	}
	sb.WriteString(ma.ReturnFormat)
	sb.WriteString("\n")
	sb.WriteString(p.JSONSchemaInstruction)
	sb.WriteString("\n")
	sb.WriteString(GetJSONSchema(language))
	return sb.String()
}