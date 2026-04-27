package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/vzx7/opencode-mcp/internal/domain"
	"github.com/vzx7/opencode-mcp/internal/llm"
	"github.com/vzx7/opencode-mcp/internal/tools"
)

type Server struct {
	executor    *tools.ToolExecutor
	defaultPath string
	debugDir    string
	llm         llm.LLMProvider
	logger      *log.Logger
	language    string
	debug      bool
}

type Config struct {
	ProjectPath string
	DebugDir    string
	Provider   string
	LLM        string
	APIKey     string
	Endpoint  string
	Port       string
	Language  string
	Debug      bool
}

func NewServer(cfg Config) *Server {
	logger := log.New(os.Stdout, "[MCP] ", log.LstdFlags)

	var llmProvider llm.LLMProvider
	var err error
	if cfg.Provider != "" {
		llmProvider, err = llm.NewProvider(cfg.Provider, cfg.APIKey, cfg.Endpoint, cfg.LLM)
		if err != nil {
			logger.Printf("Warning: failed to create LLM provider: %v, continuing without LLM", err)
			llmProvider = llm.NewMockProvider()
		}
	} else {
		llmProvider = llm.NewMockProvider()
	}

	executor := tools.NewToolExecutor(tools.ToolExecutorConfig{
		DefaultPath: cfg.ProjectPath,
		LLM:        llmProvider,
		APIKey:     cfg.APIKey,
		Endpoint:   cfg.Endpoint,
		Language:   cfg.Language,
		Debug:     cfg.Debug,
	})

	debugDir := cfg.DebugDir
	if debugDir == "" && cfg.ProjectPath != "" {
		debugDir = filepath.Join(cfg.ProjectPath, "debug")
	}
	if debugDir != "" {
		if err := os.MkdirAll(debugDir, 0755); err != nil {
			logger.Printf("Warning: failed to create debug directory: %v", err)
			debugDir = ""
		} else {
			logger.Printf("Debug dir: %s", debugDir)
		}
	}

	logger.Printf("Project: %s", cfg.ProjectPath)
	logger.Printf("LLM: %s (%s)", cfg.Provider, cfg.LLM)

	return &Server{
		executor:    executor,
		defaultPath: cfg.ProjectPath,
		debugDir:    debugDir,
		llm:        llmProvider,
		logger:     logger,
		language:   cfg.Language,
		debug:     cfg.Debug,
	}
}

func (s *Server) HandleJSONRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.sendError(w, nil, fmt.Sprintf("invalid request: %v", err))
		return
	}
	defer r.Body.Close()

	var req JSONRPCRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.sendError(w, nil, fmt.Sprintf("invalid JSON: %v", err))
		return
	}

	if s.debug {
		s.logger.Printf(">>> Request: method=%s, id=%v", req.Method, req.ID)
		if req.Method == "tools/call" {
			if name, ok := req.Params["name"].(string); ok {
				s.logger.Printf("    Tool: %s", name)
			}
		}
	}

	ctx := context.Background()
	var resp interface{}

	switch req.Method {
	case "initialize":
		resp = s.handleInitialize(req)
	case "tools/list":
		resp = s.handleToolsList(req)
	case "tools/call":
		resp = s.handleToolsCall(ctx, req)
	default:
		s.sendError(w, &req, fmt.Sprintf("method not found: %s", req.Method))
		return
	}

	jsonResp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:     req.ID,
		Result: resp,
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(jsonResp); err != nil {
		s.logger.Printf("Error encoding response: %v", err)
	}
}

func (s *Server) handleInitialize(req JSONRPCRequest) interface{} {
	return InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: Capabilities{
			Tools: struct{}{},
		},
		ServerInfo: ServerInfo{
			Name:    "code-auditor",
			Version: "1.0.0",
		},
	}
}

func (s *Server) handleToolsList(req JSONRPCRequest) interface{} {
	return ToolsListResult{
		Tools: []Tool{
			{
				Name:        "architecture_review",
				Description: "Анализирует проект как систему целиком, выявляет архитектурные проблемы, оценивает качество слоёв и границ, масштабируемость и maintainability",
				InputSchema: ToolInputSchema{
					Type: "object",
					Properties: map[string]ToolSchemaProperty{
						"project_path": {
							Type:        "string",
							Description: "Путь к анализируемому проекту",
						},
						"provider": {
							Type:        "string",
							Description: "LLM провайдер (mock, openai, anthropic)",
						},
						"llm": {
							Type:        "string",
							Description: "Модель (например, gpt-4o, claude-3-5-sonnet-20241022)",
						},
						"language": {
							Type:        "string",
							Description: "Язык ответа (ru, en)",
						},
					},
				},
			},
			{
				Name:        "architecture_compliance_check",
				Description: "Анализирует текущую структуру репозитория на соответствие заданной архитектуре, находит нарушения архитектурных правил",
				InputSchema: ToolInputSchema{
					Type: "object",
					Properties: map[string]ToolSchemaProperty{
						"project_path": {
							Type:        "string",
							Description: "Путь к анализируемому проекту",
						},
						"provider": {
							Type:        "string",
							Description: "LLM провайдер (mock, openai, anthropic)",
						},
						"llm": {
							Type:        "string",
							Description: "Модель (например, gpt-4o, claude-3-5-sonnet-20241022)",
						},
						"target_architecture": {
							Type:        "object",
							Description: "Описание целевой архитектуры (rules / constraints)",
						},
						"language": {
							Type:        "string",
							Description: "Язык ответа (ru, en)",
						},
					},
				},
			},
			{
				Name:        "module_audit",
				Description: "Аудит отдельного файла или модуля: correctness, design quality, coupling / cohesion, potential bugs",
				InputSchema: ToolInputSchema{
					Type: "object",
					Properties: map[string]ToolSchemaProperty{
						"module_path": {
							Type:        "string",
							Description: "Путь к файлу или модулю для аудита",
						},
						"project_path": {
							Type:        "string",
							Description: "Путь к проекту (если module_path относится к другому проекту)",
						},
						"provider": {
							Type:        "string",
							Description: "LLM провайдер (mock, openai, anthropic)",
						},
						"llm": {
							Type:        "string",
							Description: "Модель (например, gpt-4o, claude-3-5-sonnet-20241022)",
						},
						"language": {
							Type:        "string",
							Description: "Язык ответа (ru, en)",
						},
					},
				},
			},
		},
	}
}

// FIXED VERSION (key fixes: correct content saving, better logging, safer debug handling)

func (s *Server) handleToolsCall(ctx context.Context, req JSONRPCRequest) interface{} {
	toolName, ok := req.Params["name"].(string)
	if !ok {
		return JSONRPCError{
			Code:    -32602,
			Message: "Invalid params: missing tool name",
		}
	}

	arguments, _ := req.Params["arguments"].(map[string]interface{})

	if s.debug {
		s.logger.Printf(">>> Tool call: %s", toolName)
		s.logger.Printf("    ProjectPath: %s", s.defaultPath)
		if arguments != nil {
			s.logger.Printf("    Arguments: %s", formatArgs(arguments))
		}
	}

	var result interface{}
	var execErr error

	switch toolName {
	case "architecture_review":
		input := tools.ArchitectureReviewInput{ToolInput: tools.ToolInput{ProjectPath: s.defaultPath}}
		input.Language = s.language

		if arguments != nil {
			if pp, ok := arguments["project_path"].(string); ok {
				input.ProjectPath = pp
			}
			if p, ok := arguments["provider"].(string); ok {
				input.Provider = p
			}
			if m, ok := arguments["llm"].(string); ok {
				input.LLM = m
			}
			if l, ok := arguments["language"].(string); ok {
				input.Language = l
			}
		}

		report, err := s.executor.ArchitectureReview(ctx, input)
		if err != nil {
			return JSONRPCError{Code: -32603, Message: err.Error()}
		}
		return ToolCallResult{Content: []ContentBlock{{Type: "text", Text: s.persistReport(toolName, report, input.ProjectPath, "")}}}

	case "architecture_compliance_check":
		input := tools.ArchitectureComplianceInput{ToolInput: tools.ToolInput{ProjectPath: s.defaultPath}}
		input.Language = s.language

		if arguments != nil {
			if pp, ok := arguments["project_path"].(string); ok {
				input.ProjectPath = pp
			}
			if p, ok := arguments["provider"].(string); ok {
				input.Provider = p
			}
			if m, ok := arguments["llm"].(string); ok {
				input.LLM = m
			}
			if ta, ok := arguments["target_architecture"].(map[string]interface{}); ok {
				taJSON, _ := json.Marshal(ta)
				json.Unmarshal(taJSON, &input.TargetArchitecture)
			}
			if l, ok := arguments["language"].(string); ok {
				input.Language = l
			}
		}

		report, err := s.executor.ArchitectureComplianceCheck(ctx, input)
		if err != nil {
			return JSONRPCError{Code: -32603, Message: err.Error()}
		}
		return ToolCallResult{Content: []ContentBlock{{Type: "text", Text: s.persistReport(toolName, report, input.ProjectPath, "")}}}

	case "module_audit":
		input := tools.ModuleAuditInput{ToolInput: tools.ToolInput{ProjectPath: s.defaultPath}}
		input.Language = s.language

		if arguments != nil {
			if mp, ok := arguments["module_path"].(string); ok {
				input.ModulePath = mp
			}
			if pp, ok := arguments["project_path"].(string); ok {
				input.ProjectPath = pp
			}
			if p, ok := arguments["provider"].(string); ok {
				input.Provider = p
			}
			if m, ok := arguments["llm"].(string); ok {
				input.LLM = m
			}
			if l, ok := arguments["language"].(string); ok {
				input.Language = l
			}
		}

		report, err := s.executor.ModuleAudit(ctx, input)
		if err != nil {
			return JSONRPCError{Code: -32603, Message: err.Error()}
		}
		return ToolCallResult{Content: []ContentBlock{{Type: "text", Text: s.persistReport(toolName, report, input.ProjectPath, input.ModulePath)}}}

	default:
		execErr = fmt.Errorf("tool not found: %s", toolName)
	}

	if execErr != nil {
		return JSONRPCError{Code: -32601, Message: execErr.Error()}
	}

	return result
}

func (s *Server) resolveDebugDir(projectPath string) string {
	if s.debugDir != "" {
		return s.debugDir
	}
	if projectPath != "" {
		return filepath.Join(projectPath, "debug")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(cwd, "debug")
}

type reportEnvelope struct {
	Tool       string              `json:"tool"`
	Timestamp  string              `json:"timestamp"`
	Project    string              `json:"project"`
	ModulePath string              `json:"module_path,omitempty"`
	Report     *domain.AuditReport `json:"report"`
}

// persistReport saves the report as both .md and .json, returns the markdown string.
func (s *Server) persistReport(toolName string, report *domain.AuditReport, projectPath, modulePath string) string {
	mdContent := s.formatReport(report)

	dir := s.resolveDebugDir(projectPath)
	if dir == "" {
		s.logger.Printf("[WARN] cannot resolve debug dir, skipping file save")
		return mdContent
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		s.logger.Printf("[ERROR] failed to create debug dir %s: %v", dir, err)
		return mdContent
	}

	now := time.Now()
	base := fmt.Sprintf("%s_%s_%d", toolName, now.Format("20060102_150405"), now.UnixNano()%10000)

	mdHeader := fmt.Sprintf("# %s\n\n**Time:** %s\n\n**Project:** %s\n\n",
		strings.ToUpper(toolName),
		now.Format("2006-01-02 15:04:05"),
		projectPath,
	)
	if modulePath != "" {
		mdHeader += fmt.Sprintf("**Module:** %s\n\n", modulePath)
	}
	mdHeader += "---\n\n"

	if err := os.WriteFile(filepath.Join(dir, base+".md"), []byte(mdHeader+mdContent), 0644); err != nil {
		s.logger.Printf("[ERROR] failed to write md: %v", err)
	} else {
		s.logger.Printf("[OK] md saved: %s", filepath.Join(dir, base+".md"))
	}

	envelope := reportEnvelope{
		Tool:       toolName,
		Timestamp:  now.Format(time.RFC3339),
		Project:    projectPath,
		ModulePath: modulePath,
		Report:     report,
	}
	jsonData, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		s.logger.Printf("[ERROR] failed to marshal json: %v", err)
	} else if err := os.WriteFile(filepath.Join(dir, base+".json"), jsonData, 0644); err != nil {
		s.logger.Printf("[ERROR] failed to write json: %v", err)
	} else {
		s.logger.Printf("[OK] json saved: %s", filepath.Join(dir, base+".json"))
	}

	return mdContent
}


func (s *Server) formatReport(report *domain.AuditReport) string {
	var b strings.Builder

	scoreLabel := "LOW"
	switch {
	case report.Score >= 80:
		scoreLabel = "HIGH"
	case report.Score >= 60:
		scoreLabel = "MEDIUM"
	}
	b.WriteString(fmt.Sprintf("## Score: %d / 100  [%s]\n\n", report.Score, scoreLabel))

	if report.Summary != "" {
		b.WriteString("## Summary\n\n")
		b.WriteString(report.Summary)
		b.WriteString("\n\n")
	}

	if len(report.Issues) > 0 {
		bySeverity := map[domain.Severity][]domain.Issue{}
		order := []domain.Severity{
			domain.SeverityCritical,
			domain.SeverityHigh,
			domain.SeverityMedium,
			domain.SeverityLow,
		}
		for _, issue := range report.Issues {
			bySeverity[issue.Severity] = append(bySeverity[issue.Severity], issue)
		}

		b.WriteString(fmt.Sprintf("## Issues (%d)\n\n", len(report.Issues)))
		for _, sev := range order {
			issues, ok := bySeverity[sev]
			if !ok {
				continue
			}
			b.WriteString(fmt.Sprintf("### %s\n\n", strings.ToUpper(string(sev))))
			for _, issue := range issues {
				b.WriteString(fmt.Sprintf("- **%s**", issue.Message))
				if issue.Location != "" {
					b.WriteString(fmt.Sprintf("  \n  Location: `%s`", issue.Location))
				}
				if issue.Suggestion != "" {
					b.WriteString(fmt.Sprintf("  \n  Suggestion: %s", issue.Suggestion))
				}
				b.WriteString("\n\n")
			}
		}
	}

	if len(report.Recommendations) > 0 {
		b.WriteString("## Recommendations\n\n")
		for _, r := range report.Recommendations {
			b.WriteString(fmt.Sprintf("- %s\n", r))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func (s *Server) saveDebugResponse(toolName string, content string) {
	if s.debug {
		s.logger.Printf("saveDebugResponse: debugDir=%q, content_len=%d", s.debugDir, len(content))
	}

	if s.debugDir == "" {
		if s.debug {
			s.logger.Printf("saveDebugResponse: skipped - no debugDir")
		}
		return
	}

	if content == "" {
		if s.debug {
			s.logger.Printf("saveDebugResponse: skipped - empty content")
		}
		return
	}

	filename := fmt.Sprintf("%s_%s_%04d.md", toolName, time.Now().Format("20060102_150405"), rand.Intn(9999))
	path := filepath.Join(s.debugDir, filename)

	header := fmt.Sprintf("# %s\n\n", strings.ToUpper(toolName))
	header += fmt.Sprintf("**Time:** %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	header += fmt.Sprintf("**Project:** %s\n\n", s.defaultPath)
	header += "---\n\n"

	output := header + content

	if s.debug {
		s.logger.Printf("saveDebugResponse: writing %d bytes to %s", len(output), path)
	}

	if err := os.WriteFile(path, []byte(output), 0644); err != nil {
		s.logger.Printf("Failed to write debug file: %v", err)
	} else {
		if s.debug {
			s.logger.Printf("Saved response to: %s", path)
		}
	}
}

func (s *Server) sendError(w http.ResponseWriter, req *JSONRPCRequest, message string) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	var id interface{}
	if req != nil {
		id = req.ID
	}
	enc.Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:     id,
		Error: &JSONRPCError{
			Code:    -32603,
			Message: message,
		},
	})
}

func (s *Server) Start(port string) {
	s.logger.Printf("Starting MCP server on port %s", port)
	s.logger.Printf("LLM Provider: %s", s.llm.Name())

	http.HandleFunc("/", s.HandleJSONRPC)

	go func() {
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			s.logger.Fatalf("Server error: %v", err)
		}
	}()

	s.logger.Println("Server ready")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	s.logger.Println("Shutting down...")
}

func (s *Server) StartStdio() {
	decoder := json.NewDecoder(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	if s.debug {
		s.logger.Println("Stdio mode started")
	}

	for {
		var req JSONRPCRequest
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				return
			}
			s.logger.Printf("Read error: %v", err)
			return
		}

		if s.debug {
			s.logger.Printf(">>> Request: method=%s, id=%v", req.Method, req.ID)
			if req.Method == "tools/call" {
				if name, ok := req.Params["name"].(string); ok {
					s.logger.Printf("    Tool: %s", name)
				}
			}
		}

		ctx := context.Background()
		var resp interface{}

		switch req.Method {
		case "initialize":
			resp = s.handleInitialize(req)
		case "tools/list":
			resp = s.handleToolsList(req)
		case "tools/call":
			resp = s.handleToolsCall(ctx, req)
		default:
			s.sendErrorStdio(encoder, &req, fmt.Sprintf("method not found: %s", req.Method))
			continue
		}

		jsonResp := JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  resp,
		}
		encoder.Encode(jsonResp)
	}
}

func (s *Server) sendErrorStdio(encoder *json.Encoder, req *JSONRPCRequest, message string) {
	var id interface{}
	if req != nil {
		id = req.ID
	}
	encoder.Encode(JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    -32603,
			Message: message,
		},
	})
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method string          `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
	ID     interface{}    `json:"id,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID     interface{} `json:"id,omitempty"`
	Result interface{} `json:"result,omitempty"`
	Error  *JSONRPCError `json:"error,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type InitializeResult struct {
	ProtocolVersion string      `json:"protocolVersion"`
	Capabilities   Capabilities `json:"capabilities"`
	ServerInfo     ServerInfo  `json:"serverInfo"`
}

type Capabilities struct {
	Tools struct{} `json:"tools"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

type Tool struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	InputSchema ToolInputSchema `json:"inputSchema"`
}

type ToolInputSchema struct {
	Type       string `json:"type"`
	Properties map[string]ToolSchemaProperty `json:"properties,omitempty"`
}

type ToolSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

type ToolCallResult struct {
	Content []ContentBlock `json:"content"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func formatArgs(args map[string]interface{}) string {
	var b strings.Builder
	first := true
	for k, v := range args {
		if !first {
			b.WriteString(", ")
		}
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(fmt.Sprintf("%v", v))
		first = false
	}
	return b.String()
}