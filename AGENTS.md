# AGENTS.md

## Quick Start

```bash
# Run HTTP server (port 8080)
go run ./cmd

# Run in stdio mode (for Claude Desktop, Cursor, opencode)
go run ./cmd -stdio -provider openai -llm gpt-4o
```

## Configuration Priority

CLI flags > `.env` > hardcoded defaults.

defaults: `PROVIDER=mock`, `LLM=gpt-4o`, `PORT=8080`, `LANGUAGE=ru`, HTTP timeout: 10 minutes

`.env` keys: `PROVIDER`, `LLM`, `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `ENDPOINT`, `PORT`, `LANGUAGE`

Unknown providers fall back to OpenAI-compatible API. Use `ENDPOINT` for third-party providers (OpenRouter, Groq, local Ollama, etc.).

## Build, Test, Run

```bash
go build ./...     # Build
go test ./...      # Test
```

## Project Structure

```
cmd/main.go           # Entry point
internal/config/      # .env loading
internal/mcp/        # MCP server (JSON-RPC)
internal/tools/      # Tool execution
internal/analyzer/  # Analysis logic
internal/llm/       # LLM provider interface
internal/domain/    # Data models
```

## Adding a New Tool

1. Add input struct in `internal/tools/executor.go`
2. Add method to `ToolExecutor`
3. Register tool in `internal/mcp/server.go` (add to `tools` slice in `handleToolsList`)
4. Add case in `handleToolsCall` in `internal/mcp/server.go`

## Debug Mode

```bash
go run ./cmd -debug
```

Prompts saved to `<project_path>/debug/input/`. Reports saved to `<project_path>/debug/` as `.md` and `.json`.