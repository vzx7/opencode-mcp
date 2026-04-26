package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ai-mcp/code-auditor/internal/config"
	"github.com/ai-mcp/code-auditor/internal/mcp"
)

func main() {
	provider := flag.String("provider", "", "LLM provider (overrides ENV)")
	llm := flag.String("llm", "", "Model name (overrides ENV)")
	apiKey := flag.String("api-key", "", "API key (overrides ENV)")
	endpoint := flag.String("endpoint", "", "Endpoint (overrides ENV)")
	port := flag.String("port", "", "Port (overrides ENV)")
	project := flag.String("project", "", "Project path (overrides ENV)")

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
	if flag.Lookup("api-key").Value.String() != "" {
		cfg.APIKey = *apiKey
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

	mcpCfg := mcp.Config{
		Provider:   cfg.Provider,
		LLM:        cfg.LLM,
		APIKey:     cfg.APIKey,
		Endpoint:   cfg.Endpoint,
		ProjectPath: cfg.Project,
		Port:       cfg.Port,
	}

	server := mcp.NewServer(mcpCfg)

	fmt.Printf("AI Tech Lead MCP Server\n")
	fmt.Printf("Project: %s\n", cfg.Project)
	fmt.Printf("Provider: %s (%s)\n", cfg.Provider, cfg.LLM)
	fmt.Printf("Port: %s\n\n", cfg.Port)

	server.Start(cfg.Port)
}