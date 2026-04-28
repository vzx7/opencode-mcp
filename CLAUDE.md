# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What This Project Is

An MCP (Model Context Protocol) server that provides code architecture audit and review tools for AI assistants (Claude Desktop, Cursor, etc.). It exposes 3 MCP tools: `architecture_review`, `architecture_compliance_check`, and `module_audit`, each backed by a configurable LLM provider (OpenAI, Anthropic, or mock).

## Commands

```bash
# Build
go build ./...
go build -o audit ./cmd

# Run (HTTP mode, port 8080)
go run ./cmd

# Run (stdio mode for MCP clients)
go run ./cmd -stdio -provider openai -llm gpt-4o

# Run with debug output
go run ./cmd -debug -project /path/to/project -debug-dir /tmp/audit

# Tests
go test ./...
go test -v ./...
go test -v ./internal/mcp/
```

## Architecture

```
MCP Client (JSON-RPC 2.0)
    ↓
internal/mcp/server.go       — HTTP or stdio transport, request routing
    ↓
internal/tools/executor.go   — tool dispatch, LLM calls, report persistence
    ├── internal/analyzer/engine.go   — static project analysis, structure mapping
    └── internal/llm/provider_impl.go — OpenAI / Anthropic / Mock provider
    ↓
debug/ directory             — .md + .json report pairs per invocation
```

**Module responsibilities:**
- `cmd/main.go` — CLI flags, config loading, server startup
- `internal/config/config.go` — `.env` loading and validation
- `internal/domain/models.go` — shared data structs (`Issue`, `AuditReport`, `ProjectMap`)
- `internal/llm/` — `Provider` interface + implementations; `types.go` has type aliases for domain types
- `internal/analyzer/engine.go` — walks project files, detects languages, maps dependencies
- `internal/tools/executor.go` — thread-safe (RWMutex) executor; builds LLM prompts, parses responses, saves reports

## Configuration

Priority (highest → lowest): CLI flags → `.env` file → hardcoded defaults.

Defaults: `PROVIDER=mock`, `LLM=gpt-4o`, `PORT=8080`, `LANGUAGE=ru`, HTTP timeout: 10 minutes.

Copy `.env.example` to `.env` and set `API_KEY` for non-mock providers. Each tool call can also override provider/model via its own arguments.

## Adding a New Tool

1. Define the tool's `tools/list` entry in `internal/mcp/server.go` (name, description, inputSchema).
2. Add a handler case in the `tools/call` dispatch in `server.go`.
3. Implement execution logic in `internal/tools/executor.go`.
4. If new domain types are needed, add them to `internal/domain/models.go`.

## Debug Output

When `-debug` flag is set, reports are written to `<project>/debug/` (or `-debug-dir`) as both `<tool>_<timestamp>_<timestamp_nanos>.md` and `.json`. The `debug/` directory is git-ignored.
