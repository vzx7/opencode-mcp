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

defaults: `PROVIDER=mock`, `LLM=gpt-4o`, `PORT=8080`, `LANGUAGE=ru`

## Build, Test, Run

```bash
go build ./...     # Build
go test ./...      # Test (only internal/mcp has tests)
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
3. Register tool in `internal/mcp/server.go` (add to `tools` slice)
4. Add case in `handleToolsCall`

## Debug Mode

```bash
go run ./cmd -debug -project /path/to/project
```

Reports saved to `<project>/debug/` as `.md` and `.json`.