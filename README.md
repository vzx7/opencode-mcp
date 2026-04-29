# AI Tech Lead MCP Server

MCP server for AI assistants, providing code architecture audit and review tools. Supports Go, TypeScript, Python, Rust, and Java projects.

## Translations

- [Русский](docs/README.ru.md)
- [中文](docs/README.zh.md)

## Features

The server provides 3 MCP tools:

| Tool | Description |
|------|-------------|
| `architecture_review` | Full project architecture audit with curated file snapshot |
| `architecture_compliance_check` | Check compliance against target architecture rules and docs |
| `module_audit` | Audit an individual file or module |

---

## Configuration via .env

Create a `.env` file in the project root:

```bash
cp .env.example .env
```

### .env Parameters

| Variable             | Description                                   | Default            |
|----------------------|-----------------------------------------------|--------------------|
| `PROVIDER`           | LLM provider (`mock`, `openai`, `anthropic`)  | `mock`             |
| `LLM`                | Model name                                    | `gpt-4o`           |
| `OPENAI_API_KEY`     | OpenAI API key                                | —                  |
| `ANTHROPIC_API_KEY`  | Anthropic API key                             | —                  |
| `ENDPOINT`           | Custom endpoint (OpenAI-compatible APIs)      | —                  |
| `PROJECT`            | Default project path                          | current directory  |
| `PORT`               | HTTP server port                              | `8080`             |
| `LANGUAGE`           | Response language (`ru`, `en`, `zh`)          | `ru`               |

> **Note:** HTTP timeout for LLM requests is 10 minutes.

### .env Examples

**Mock (no LLM, for testing):**
```env
PROVIDER=mock
PORT=8080
```

**OpenAI:**
```env
PROVIDER=openai
LLM=gpt-4o
OPENAI_API_KEY=sk-...
```

**Anthropic:**
```env
PROVIDER=anthropic
LLM=claude-3-5-sonnet-20241022
ANTHROPIC_API_KEY=sk-ant-...
```

### CLI flags

All flags override the corresponding `.env` values:

```
-stdio       Run in stdio mode for MCP clients (Claude Desktop, Cursor, etc.)
-provider    LLM provider (overrides PROVIDER)
-llm         Model name (overrides LLM)
-endpoint    Custom endpoint (overrides ENDPOINT)
-port        Port (overrides PORT)
-project     Project path (overrides PROJECT)
-debug-dir   Debug output directory (default: <project>/debug)
-debug       Enable verbose logging
```

> **Security note:** Store API keys in `.env` — it is listed in `.gitignore`.

**Examples:**
```bash
# HTTP mode
go run ./cmd -provider openai -llm gpt-4o -project /path/to/project

# stdio mode for MCP clients
go run ./cmd -stdio -provider anthropic -llm claude-3-5-sonnet-20241022 -project /path/to/project

# Custom debug output directory
go run ./cmd -stdio -project /path/to/project -debug-dir /tmp/audit
```

### Debug mode

```bash
go run ./cmd -debug -project /path/to/project
```

Reports are saved to `<project>/debug/` as both `.md` and `.json` files on every tool call.

---

## Quick Start

### 1. Run the server

```bash
go run ./cmd
```

The server listens on `http://localhost:8080` by default.

### 2. Connect to opencode

Add to `~/.opencode/mcp.json`:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "mcp_project_audit": {
      "type": "remote",
      "url": "http://localhost:8080",
      "enabled": true
    }
  }
}
```

After restarting opencode, the following skill commands are available:

```
/architecture_review
/architecture_compliance_check
/module_audit
```

---

## Using Tools

### architecture_review

Analyzes the full project architecture: layers, import graph, file metrics, git hotspots. Accepts a curated file list via `include_paths` to focus the LLM on the most important files.

**Parameters:**

| Parameter              | Type     | Description |
|------------------------|----------|-------------|
| `project_path`         | string   | Path to the project (optional, defaults to server's `--project`) |
| `provider`             | string   | LLM provider: `mock`, `openai`, `anthropic` |
| `llm`                  | string   | Model name (e.g. `gpt-4o`, `claude-3-5-sonnet-20241022`) |
| `language`             | string   | Response language: `ru`, `en`, `zh` |
| `programming_language` | string   | Project language: `go`, `python`, `typescript`, `rust`, `java`. Auto-detected if omitted. |
| `include_paths`        | string[] | Relative paths to key files for the code snapshot. Auto-discovered by size if omitted. |

**Example call:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "architecture_review",
    "arguments": {
      "project_path": "/path/to/my-project",
      "provider": "openai",
      "llm": "gpt-4o",
      "language": "en",
      "programming_language": "go",
      "include_paths": [
        "cmd/main.go",
        "internal/domain/models.go",
        "internal/mcp/server.go"
      ]
    }
  }
}
```

**Response:** markdown report returned as MCP content text. Two files are also written to the debug directory:
- `architecture_review_<timestamp>.md` — human-readable report
- `architecture_review_<timestamp>.json` — structured report:

```json
{
  "tool": "architecture_review",
  "timestamp": "2026-04-28T12:00:00+03:00",
  "project": "/path/to/project",
  "report": {
    "score": 85,
    "summary": "...",
    "issues": [
      {
        "severity": "medium",
        "message": "Limited architecture layers detected",
        "location": "/path/to/project",
        "suggestion": "Consider adopting layered architecture"
      }
    ],
    "recommendations": ["Add cmd/ for entrypoints"]
  }
}
```

---

### architecture_compliance_check

Checks the project against defined architecture rules. Optionally accepts architecture documentation (ADRs, specs) as the ground truth for LLM evaluation.

**Parameters:**

| Parameter              | Type     | Description |
|------------------------|----------|-------------|
| `project_path`         | string   | Path to the project |
| `provider`             | string   | LLM provider |
| `llm`                  | string   | Model name |
| `language`             | string   | Response language: `ru`, `en`, `zh` |
| `programming_language` | string   | Project language. Auto-detected if omitted. |
| `target_architecture`  | object   | Architecture rules (layers, allowed imports) |
| `docs`                 | string   | Relative path to directory containing `.architecture.json` (e.g. `docs/arch`). If omitted, `docs/arch` is tried automatically. |
| `include_paths`        | string[] | Relative paths to key source files for the code snapshot. |

**`target_architecture` format** (also the format of `.architecture.json`):
```json
{
  "layers": [
    {
      "name": "cmd",
      "patterns": ["cmd"],
      "allow_imports_from": ["domain", "mcp", "config"]
    },
    {
      "name": "domain",
      "patterns": ["internal/domain"],
      "allow_imports_from": []
    }
  ],
  "forbidden_dependencies": [
    {"from": "domain", "to": "cmd", "reason": "no upward deps"}
  ],
  "constraints": ["All shared types must be in internal/domain"]
}
```

Place `.architecture.json` in `docs/arch/` — it will be loaded automatically without passing `docs`.

**Example call:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "architecture_compliance_check",
    "arguments": {
      "project_path": "/path/to/project",
      "provider": "anthropic",
      "llm": "claude-sonnet-4-6",
      "include_paths": [
        "internal/domain/models.go",
        "internal/mcp/server.go"
      ]
    }
  }
}
```

---

### module_audit

Audits an individual file or directory: correctness, design quality, coupling, cohesion, potential bugs, complexity.

**Parameters:**

| Parameter              | Type   | Description |
|------------------------|--------|-------------|
| `module_path`          | string | Path to the file or directory to audit |
| `project_path`         | string | Project root path |
| `provider`             | string | LLM provider |
| `llm`                  | string | Model name |
| `language`             | string | Response language: `ru`, `en`, `zh` |
| `programming_language` | string | Project language. Auto-detected if omitted. |

**Example call:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "module_audit",
    "arguments": {
      "module_path": "/path/to/project/internal/service",
      "project_path": "/path/to/project",
      "provider": "openai",
      "llm": "gpt-4o-mini",
      "programming_language": "go"
    }
  }
}
```

---

### Provider and Model Selection Priority

1. **Tool call arguments** — `provider` / `llm` per call (highest priority)
2. **CLI flags at startup** — `-provider` / `-llm`
3. **`.env` file** — `PROVIDER` / `LLM`
4. **Hardcoded defaults** — provider: `mock`, model: `gpt-4o`

---

## Project Architecture

```
cmd/
  main.go                    # Entry point, CLI flags

internal/
  config/
    config.go                # .env loading and validation

  mcp/
    server.go                # MCP server (JSON-RPC 2.0, HTTP + stdio)

  tools/
    executor.go              # Tool execution, prompt building, report persistence

  analyzer/
    engine.go                # Language-agnostic analysis orchestrator
    language.go              # ProjectAnalyzer interface
    registry.go              # Auto-detection (go.mod, tsconfig.json, pyproject.toml, ...)
    golang/
      analyzer.go            # Go: go/ast import graph, go.mod, _test.go detection
    python/
      analyzer.go            # Python stub: detection, empty import graph
    typescript/
      analyzer.go            # TypeScript stub: detection, empty import graph

  llm/
    provider.go              # LLMProvider interface
    provider_impl.go         # OpenAI, Anthropic, Mock implementations + prompt builders
    types.go                 # Type aliases for domain types

  domain/
    models.go                # Shared data structs (AuditReport, Issue, ProjectMap, ...)
```

---

## Extending Functionality

### Adding a new tool

1. Add input struct in `internal/tools/executor.go`.
2. Add method to `ToolExecutor`.
3. Register tool schema in `internal/mcp/server.go` (`handleToolsList`).
4. Add handler case in `handleToolsCall`.

### Adding a new language

1. Create `internal/analyzer/<lang>/analyzer.go` implementing `ProjectAnalyzer`.
2. Add one line to the registry slice in `internal/analyzer/registry.go`.

---

## License

MIT
