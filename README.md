# AI Tech Lead MCP Server

MCP server for AI assistants, providing code architecture audit and review tools.

## Translations

- [Русский](docs/README.ru.md)
- [中文](docs/README.zh.md)

## Features

The server provides 3 MCP tools:

| Tool | Description |
|------|-------------|
| `architecture_review` | Full project architecture audit |
| `architecture_compliance_check` | Check compliance with target architecture |
| `module_audit` | Audit individual file or module |

---

## Configuration via .env

Create a `.env` file in the project root:

```bash
# Copy from .env.example
cp .env.example .env
```

### .env Parameters

| Variable | Description | Default |
|----------|-------------|---------|
| `PROVIDER` | LLM provider (`mock`, `openai`, `anthropic`, or OpenAI-compatible) | `mock` |
| `LLM` | Model | `gpt-4o` |
| `API_KEY` | API key | - |
| `ENDPOINT` | Custom endpoint | - |
| `PROJECT` | Project path | current directory |
| `PORT` | Port | `8080` |

### .env Examples

**Mock (without LLM):**
```env
PROVIDER=mock
LLM=
PORT=8080
```

**OpenAI:**
```env
PROVIDER=openai
LLM=gpt-4o
API_KEY=sk-...
```

**Anthropic:**
```env
PROVIDER=anthropic
LLM=claude-3-5-sonnet-20241022
API_KEY=sk-ant-...
```

### CLI arguments override .env

```bash
go run ./cmd --provider openai --llm gpt-4o-mini
```

---

## Quick Start

### 1. Run the server

```bash
go run ./cmd
```

By default, the server listens on `http://localhost:8080`.

### 2. Connect to opencode

Add configuration to `~/.opencode/mcp.json` (server will load settings from `.env`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "mcp_custom": {
      "type": "remote",
      "url": "localhost:8080",
      "enabled": true
    }
  }
}
```

After restarting opencode, these commands are available:

```
/architecture-review project_path=/path/to/project
/architecture-compliance-check project_path=/path/to/project
/module-audit module_path=/path/to/module
```

---

## Using Tools

### architecture_review

Analyzes the project as a system, identifies architectural issues.

**Parameters:**
- `project_path` (string, optional) - path to project
- `provider` (string, optional) - LLM provider (mock, openai, anthropic)
- `llm` (string, optional) - model (gpt-4o, claude-3-5-sonnet-20241022)

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
      "llm": "gpt-4o"
    }
  }
}
```

**From opencode:**
```
/architecture-review project_path=/path/to/project provider=openai llm=gpt-4o
```

**Response:**
```json
{
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
```

### architecture_compliance_check

Checks project compliance against target architecture.

**Parameters:**
- `project_path` (string, optional) - path to project
- `provider` (string, optional) - LLM provider
- `llm` (string, optional) - model
- `target_architecture` (object, optional) - architecture rules

**target_architecture format:**
```json
{
  "layers": [
    {
      "name": "cmd",
      "paths": ["cmd"],
      "allow": ["main"]
    },
    {
      "name": "internal",
      "paths": ["internal"],
      "allow": ["api", "domain", "service", "repository"]
    }
  ],
  "dependencies": [
    {"from": "internal", "to": "external", "violation": false}
  ],
  "constraints": []
}
```

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
      "llm": "claude-3-5-sonnet-20241022",
      "target_architecture": {
        "layers": [
          {"name": "cmd", "paths": ["cmd"], "allow": []},
          {"name": "internal", "paths": ["internal"], "allow": ["domain", "service"]}
        ]
      }
    }
  }
}
```

**From opencode:**
```
/architecture-compliance-check project_path=/path/to/project provider=anthropic llm=claude-3-5-sonnet-20241022
```

### module_audit

Audits an individual file or module.

**Parameters:**
- `module_path` (string, optional) - path to module
- `project_path` (string, optional) - path to project
- `provider` (string, optional) - LLM provider
- `llm` (string, optional) - model

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
      "provider": "openai",
      "llm": "gpt-4o-mini"
    }
  }
}
```

**From opencode:**
```
/module-audit module_path=/path/to/project/internal/service provider=openai llm=gpt-4o-mini
```

### Provider and Model Selection Priority

1. **Tool call arguments** (highest priority):
   - `provider=openai llm=gpt-4o` - dynamic for each call

2. **.env file**:
   - `PROVIDER=...`, `LLM=...` - default for all calls

3. **CLI arguments when starting server**:
   - `--provider=... --llm=...` - overrides .env

4. **Hardcoded defaults** (lowest priority):
   - Provider: `mock`, Model: `gpt-4o`

---

## Project Architecture

```
cmd/
  main.go              # Entry point

internal/
  config/
    config.go        # .env loading

  mcp/
    server.go       # MCP server (JSON-RPC)

  tools/
    executor.go    # Tool execution logic

  analyzer/
    engine.go      # Project and module analysis

  llm/
    provider.go      # LLM provider interface
    provider_impl.go  # Provider implementations
    types.go         # Type aliases

  domain/
    models.go      # Data models
```

---

## Extending Functionality

### Adding a new tool

1. Add input struct in `internal/tools/executor.go`:
   ```go
   type NewToolInput struct {
       Param1 string `json:"param1"`
   }
   ```

2. Add method to `ToolExecutor`:
   ```go
   func (te *ToolExecutor) NewTool(ctx context.Context, input NewToolInput) (*domain.AuditReport, error) {
       // logic
   }
   ```

3. Register tool in `internal/mcp/server.go`:
   ```go
   {
       Name:        "new_tool",
       Description: "Description",
       InputSchema: ToolInputSchema{...},
   }
   ```

4. Add handler in `handleToolsCall`:
   ```go
   case "new_tool":
       // call
   ```

---

## License

MIT