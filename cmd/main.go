package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/vzx7/opencode-mcp/internal/config"
	"github.com/vzx7/opencode-mcp/internal/mcp"
)

var stdioMode bool
var debugMode bool

func init() {
	flag.BoolVar(&stdioMode, "stdio", false, "Run in stdio mode for MCP clients")
	flag.BoolVar(&debugMode, "debug", false, "Enable debug logging")
}

func main() {
	provider := flag.String("provider", "", "LLM provider (overrides ENV)")
	llm := flag.String("llm", "", "Model name (overrides ENV)")
	endpoint := flag.String("endpoint", "", "Endpoint (overrides ENV)")
	port := flag.String("port", "", "Port (overrides ENV)")
	project := flag.String("project", "", "Project path (overrides ENV)")
	debugDir := flag.String("debug-dir", "", "Debug output directory (default: <project>/debug)")

	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// CLI args override .env
	if flag.Lookup("provider").Value.String() != "" {
		cfg.Provider = *provider
	}
	if flag.Lookup("llm").Value.String() != "" {
		cfg.LLM = *llm
	}
	if flag.Lookup("endpoint").Value.String() != "" {
		cfg.Endpoint = *endpoint
	}
	if flag.Lookup("port").Value.String() != "" {
		cfg.Port = *port
	}
	if flag.Lookup("project").Value.String() != "" {
		cfg.Project = *project
	}

	// Validate after CLI overrides
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	dbgDir := *debugDir

	mcpCfg := mcp.Config{
		ProjectPath: cfg.Project,
		DebugDir:    dbgDir,
		Provider:  cfg.Provider,
		LLM:      cfg.LLM,
		OpenAIKey:  cfg.OpenAIKey,
		AnthropicKey: cfg.AnthropicKey,
		Endpoint: cfg.Endpoint,
		Port:     cfg.Port,
		Language: cfg.Language,
		Debug:    debugMode,
	}

	server := mcp.NewServer(mcpCfg)

	if stdioMode {
		fmt.Fprintf(os.Stderr, "AI Tech Lead MCP Server (stdio mode)\n")
		fmt.Fprintf(os.Stderr, "Provider: %s (%s)\n", cfg.Provider, cfg.LLM)
		if debugMode {
			fmt.Fprintf(os.Stderr, "Debug: enabled\n")
		}
		fmt.Fprintf(os.Stderr, "\n")
		server.StartStdio()
	} else {
		fmt.Printf("AI Tech Lead MCP Server\n")
		fmt.Printf("Project: %s\n", cfg.Project)
		fmt.Printf("Provider: %s (%s)\n", cfg.Provider, cfg.LLM)
		fmt.Printf("Port: %s\n", cfg.Port)
		if debugMode {
			fmt.Printf("Debug: enabled\n")
		}
		fmt.Printf("\n")
		server.Start(cfg.Port)
	}
}