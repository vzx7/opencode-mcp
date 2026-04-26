package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/ai-mcp/code-auditor/internal/domain"
	"github.com/ai-mcp/code-auditor/internal/llm"
	"github.com/ai-mcp/code-auditor/internal/tools"
)

type Server struct {
	executor *tools.ToolExecutor
	defaultPath string
	llm      llm.LLMProvider
	logger   *log.Logger
}

type Config struct {
	ProjectPath string
	Provider   string
	LLM        string
	APIKey     string
	Endpoint   string
	Port       string
}

func NewServer(cfg Config) *Server {
	var llmProvider llm.LLMProvider
	var err error
	if cfg.Provider != "" {
		llmProvider, err = llm.NewProvider(cfg.Provider, cfg.APIKey, cfg.Endpoint, cfg.LLM)
		if err != nil {
			log.Printf("Warning: failed to create LLM provider: %v, continuing without LLM", err)
			llmProvider = llm.NewMockProvider()
		}
	} else {
		llmProvider = llm.NewMockProvider()
	}

	executor := tools.NewToolExecutor(cfg.ProjectPath, llmProvider)

	return &Server{
		executor:   executor,
		defaultPath: cfg.ProjectPath,
		llm:        llmProvider,
		logger:     log.New(os.Stdout, "[MCP] ", log.LstdFlags),
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
					},
				},
			},
		},
	}
}

func (s *Server) handleToolsCall(ctx context.Context, req JSONRPCRequest) interface{} {
	toolName, ok := req.Params["name"].(string)
	if !ok {
		return JSONRPCError{
			Code:    -32602,
			Message: "Invalid params: missing tool name",
		}
	}

	arguments, _ := req.Params["arguments"].(map[string]interface{})

	switch toolName {
	case "architecture_review":
		input := tools.ArchitectureReviewInput{ToolInput: tools.ToolInput{ProjectPath: "."}}
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
		}
		report, err := s.executor.ArchitectureReview(ctx, input)
		if err != nil {
			return JSONRPCError{Code: -32603, Message: err.Error()}
		}
		return ToolCallResult{Content: []ContentBlock{{Type: "text", Text: s.formatReport(report)}}}

	case "architecture_compliance_check":
		input := tools.ArchitectureComplianceInput{ToolInput: tools.ToolInput{ProjectPath: "."}}
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
		}
		report, err := s.executor.ArchitectureComplianceCheck(ctx, input)
		if err != nil {
			return JSONRPCError{Code: -32603, Message: err.Error()}
		}
		return ToolCallResult{Content: []ContentBlock{{Type: "text", Text: s.formatReport(report)}}}

	case "module_audit":
		input := tools.ModuleAuditInput{ToolInput: tools.ToolInput{ProjectPath: "."}}
		if arguments != nil {
			if mp, ok := arguments["module_path"].(string); ok {
				input.ProjectPath = mp
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
		}
		report, err := s.executor.ModuleAudit(ctx, input)
		if err != nil {
			return JSONRPCError{Code: -32603, Message: err.Error()}
		}
		return ToolCallResult{Content: []ContentBlock{{Type: "text", Text: s.formatReport(report)}}}

	default:
		return JSONRPCError{Code: -32601, Message: fmt.Sprintf("Tool not found: %s", toolName)}
	}
}

func (s *Server) formatReport(report *domain.AuditReport) string {
	data, _ := json.MarshalIndent(report, "", "  ")
	return string(data)
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