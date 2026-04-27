# AI Tech Lead MCP 服务器

面向AI助手的MCP服务器，提供代码架构审计和审查工具。

## 功能特性

服务器提供3个MCP工具：

| 工具 | 描述 |
|------|-------------|
| `architecture_review` | 完整项目架构审计 |
| `architecture_compliance_check` | 检查是否符合目标架构 |
| `module_audit` | 审计单个文件或模块 |

---

## 通过 .env 配置

在项目根目录创建 `.env` 文件：

```bash
# 从 .env.example 复制
cp .env.example .env
```

### .env 参数

| 变量　　　 | 描述　　　　　　　　　　　　　　　　　　　　　 | 默认值　 |
| ------------| ------------------------------------------------| ----------|
| `PROVIDER` | LLM提供商 (`mock`, `openai`, 或任何OpenAI兼容) | `mock`　 |
| `LLM`　　　| 模型　　　　　　　　　　　　　　　　　　　　　 | `gpt-4o` |
| `API_KEY`　| API密钥　　　　　　　　　　　　　　　　　　　　| -　　　　|
| `ENDPOINT` | 自定义端点　　　　　　　　　　　　　　　　　　 | -　　　　|
| `PROJECT`　| 项目路径　　　　　　　　　　　　　　　　　　　 | 当前目录 |
| `PORT`　　 | 端口　　　　　　　　　　　　　　　　　　　　　 | `8080`　 |
| `LANGUAGE` | 响应语言 (`ru`, `en`)　　　　　　　　　　　　　| `ru`　　 |

### .env 示例

**Mock (无LLM)：**
```env
PROVIDER=mock
LLM=
PORT=8080
```

**OpenAI：**
```env
PROVIDER=openai
LLM=gpt-4o
API_KEY=sk-...
```

**Anthropic** *（尚未实现，回退到mock）*：
```env
PROVIDER=anthropic
LLM=claude-3-5-sonnet-20241022
API_KEY=sk-ant-...
```

### CLI标志

所有标志覆盖 `.env` 中对应的值：

```
-stdio         为MCP客户端以stdio模式运行（Claude Desktop、Cursor等）
-provider      LLM提供商（覆盖PROVIDER）
-llm           模型名称（覆盖LLM）
-api-key       API密钥（覆盖API_KEY）
-endpoint      自定义端点（覆盖ENDPOINT）
-port          端口（覆盖PORT）
-project       项目路径（覆盖PROJECT）
-debug-dir     调试输出目录（默认：<project>/debug）
-debug         启用详细日志
```

**示例：**
```bash
# HTTP模式
go run ./cmd -provider openai -llm gpt-4o -project /path/to/project

# MCP客户端的stdio模式
go run ./cmd -stdio -provider openai -llm gpt-4o -project /path/to/project

# 自定义调试输出目录
go run ./cmd -stdio -project /path/to/project -debug-dir /tmp/audit
```

### Debug模式

使用 `-debug` 运行以获取LLM请求、响应和文件保存的详细日志：

```bash
go run ./cmd -debug -project /path/to/project
```

默认情况下，报告以 `.md` 和 `.json` 两种格式保存到 `<project>/debug/`。

---

## 快速开始

### 1. 运行服务器

```bash
go run ./cmd
```

默认情况下，服务器监听 `http://localhost:8080`。

### 2. 连接到 opencode

在 `~/.opencode/mcp.json` 中添加配置（服务器将从 `.env` 加载设置）：

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

重启 opencode 后，可以使用以下命令：

```
/architecture-review project_path=/path/to/project
/architecture-compliance-check project_path=/path/to/project
/module-audit module_path=/path/to/module
```

---

## 使用工具

### architecture_review

分析项目作为系统，识别架构问题。

**参数：**
- `project_path` (string, 可选) - 项目路径
- `provider` (string, 可选) - LLM提供商 (`mock`, `openai`, 或任何OpenAI兼容)
- `llm` (string, 可选) - 模型 (例如 `gpt-4o`)
- `language` (string, 可选) - 响应语言 (`ru`, `en`)

**调用示例：**
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
      "language": "en"
    }
  }
}
```

**从 opencode 调用：**
```
/architecture-review project_path=/path/to/project provider=openai llm=gpt-4o
```

**响应：** 工具以MCP content text形式返回Markdown格式报告。同时向debug目录保存两个文件：
- `architecture_review_<timestamp>.md` — 可读报告
- `architecture_review_<timestamp>.json` — 供AI代理使用的结构化报告：

```json
{
  "tool": "architecture_review",
  "timestamp": "2026-04-26T22:42:57+03:00",
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

### architecture_compliance_check

检查项目是否符合目标架构。

**参数：**
- `project_path` (string, 可选) - 项目路径
- `provider` (string, 可选) - LLM提供商 (`mock`, `openai`, 或任何OpenAI兼容)
- `llm` (string, 可选) - 模型
- `target_architecture` (object, 可选) - 架构规则
- `language` (string, 可选) - 响应语言 (`ru`, `en`)

**target_architecture 格式：**
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

**调用示例：**
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

**从 opencode 调用：**
```
/architecture-compliance-check project_path=/path/to/project provider=anthropic llm=claude-3-5-sonnet-20241022
```

### module_audit

审计单个文件或模块。

**参数：**
- `module_path` (string, 可选) - 文件或模块路径
- `project_path` (string, 可选) - 项目根路径
- `provider` (string, 可选) - LLM提供商 (`mock`, `openai`, 或任何OpenAI兼容)
- `llm` (string, 可选) - 模型
- `language` (string, 可选) - 响应语言 (`ru`, `en`)

**调用示例：**
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

**从 opencode 调用：**
```
/module-audit module_path=/path/to/project/internal/service provider=openai llm=gpt-4o-mini
```

### 提供商和模型选择优先级

1. **工具调用参数**（最高优先级）：
   - `provider=openai llm=gpt-4o` — 每次动态选择

2. **启动服务器时的CLI标志**：
   - `-provider=... -llm=...` — 覆盖 `.env`

3. **`.env` 文件**：
   - `PROVIDER=...`, `LLM=...` — 所有调用的默认值

4. **硬编码默认值**（最低优先级）：
   - 提供商: `mock`, 模型: `gpt-4o`

---

## 项目架构

```
cmd/
  main.go              # 入口点

internal/
  config/
    config.go        # .env 加载

  mcp/
    server.go       # MCP服务器 (JSON-RPC)

  tools/
    executor.go    # 工具执行逻辑

  analyzer/
    engine.go      # 项目和模块分析

  llm/
    provider.go      # LLM提供商接口
    provider_impl.go  # 提供商实现
    types.go         # 类型别名

  domain/
    models.go      # 数据模型
```

---

## 扩展功能

### 添加新工具

1. 在 `internal/tools/executor.go` 中添加输入结构：
   ```go
   type NewToolInput struct {
       Param1 string `json:"param1"`
   }
   ```

2. 在 `ToolExecutor` 中添加方法：
   ```go
   func (te *ToolExecutor) NewTool(ctx context.Context, input NewToolInput) (*domain.AuditReport, error) {
       // 逻辑
   }
   ```

3. 在 `internal/mcp/server.go` 中注册工具：
   ```go
   {
       Name:        "new_tool",
       Description: "Description",
       InputSchema: ToolInputSchema{...},
   }
   ```

4. 在 `handleToolsCall` 中添加处理：
   ```go
   case "new_tool":
       // 调用
   ```

---

## 许可证

MIT