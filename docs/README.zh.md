# AI Tech Lead MCP 服务器

面向AI助手的MCP服务器，提供代码架构审计和审查工具。支持 Go、TypeScript、Python、Rust 和 Java 项目。

## 功能特性

服务器提供 3 个 MCP 工具：

| 工具 | 描述 |
|------|------|
| `architecture_review` | 完整项目架构审计，包含关键文件快照 |
| `architecture_compliance_check` | 对照架构规则和文档进行合规性检查 |
| `module_audit` | 审计单个文件或模块 |

---

## 通过 .env 配置

在项目根目录创建 `.env` 文件：

```bash
cp .env.example .env
```

### .env 参数

| 变量                  | 描述                                          | 默认值       |
|-----------------------|-----------------------------------------------|--------------|
| `PROVIDER`            | LLM 提供商 (`mock`, `openai`, `anthropic`)    | `mock`       |
| `LLM`                 | 模型名称                                      | `gpt-4o`     |
| `OPENAI_API_KEY`      | OpenAI API 密钥                               | —            |
| `ANTHROPIC_API_KEY`   | Anthropic API 密钥                            | —            |
| `ENDPOINT`            | 自定义端点（OpenAI 兼容 API）                 | —            |
| `PROJECT`             | 默认项目路径                                  | 当前目录     |
| `PORT`                | HTTP 服务器端口                               | `8080`       |
| `LANGUAGE`            | 响应语言 (`ru`, `en`, `zh`)                   | `ru`         |

> **注意：** LLM 请求的 HTTP 超时时间为 10 分钟。

### .env 示例

**Mock（无 LLM，用于测试）：**
```env
PROVIDER=mock
PORT=8080
```

**OpenAI：**
```env
PROVIDER=openai
LLM=gpt-4o
OPENAI_API_KEY=sk-...
```

**Anthropic：**
```env
PROVIDER=anthropic
LLM=claude-3-5-sonnet-20241022
ANTHROPIC_API_KEY=sk-ant-...
```

### CLI 标志

所有标志覆盖 `.env` 中对应的值：

```
-stdio       以 stdio 模式运行（Claude Desktop、Cursor 等 MCP 客户端）
-provider    LLM 提供商（覆盖 PROVIDER）
-llm         模型名称（覆盖 LLM）
-endpoint    自定义端点（覆盖 ENDPOINT）
-port        端口（覆盖 PORT）
-project     项目路径（覆盖 PROJECT）
-debug-dir   调试输出目录（默认：<project>/debug）
-debug       启用详细日志
```

> **安全提示：** 将 API 密钥存储在 `.env` 文件中——该文件已加入 `.gitignore`。

**示例：**
```bash
# HTTP 模式
go run ./cmd -provider openai -llm gpt-4o -project /path/to/project

# MCP 客户端 stdio 模式
go run ./cmd -stdio -provider anthropic -llm claude-3-5-sonnet-20241022 -project /path/to/project

# 自定义调试输出目录
go run ./cmd -stdio -project /path/to/project -debug-dir /tmp/audit
```

### Debug 模式

```bash
go run ./cmd -debug -project /path/to/project
```

每次工具调用时，报告以 `.md` 和 `.json` 两种格式保存到 `<project>/debug/`。

---

## 快速开始

### 1. 运行服务器

```bash
go run ./cmd
```

默认情况下，服务器监听 `http://localhost:8080`。

### 2. 连接到 opencode

在 `~/.opencode/mcp.json` 中添加：

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

重启 opencode 后，可使用以下技能命令：

```
/architecture_review
/architecture_compliance_check
/module_audit
```

---

## 使用工具

### architecture_review

分析整个项目架构：层次结构、导入图、文件指标、git 热点。通过 `include_paths` 接受关键文件列表，使 LLM 专注于架构上最重要的文件。

**参数：**

| 参数                   | 类型     | 描述 |
|------------------------|----------|------|
| `project_path`         | string   | 项目路径（可选） |
| `provider`             | string   | LLM 提供商：`mock`, `openai`, `anthropic` |
| `llm`                  | string   | 模型名称（如 `gpt-4o`、`claude-3-5-sonnet-20241022`） |
| `language`             | string   | 响应语言：`ru`, `en`, `zh` |
| `programming_language` | string   | 项目语言：`go`, `python`, `typescript`, `rust`, `java`。不指定则自动检测。 |
| `include_paths`        | string[] | 代码快照中包含的关键文件相对路径。不指定则按文件大小自动发现。 |

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
      "language": "zh",
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

**响应：** 以 MCP content text 形式返回 Markdown 格式报告。同时向 debug 目录写入两个文件：
- `architecture_review_<timestamp>.md` — 可读报告
- `architecture_review_<timestamp>.json` — 结构化报告：

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

检查项目是否符合定义的架构规则。可选择接受架构文档目录（ADR、规范）作为 LLM 评估的基准。

**参数：**

| 参数                   | 类型     | 描述 |
|------------------------|----------|------|
| `project_path`         | string   | 项目路径 |
| `provider`             | string   | LLM 提供商 |
| `llm`                  | string   | 模型名称 |
| `language`             | string   | 响应语言：`ru`, `en`, `zh` |
| `programming_language` | string   | 项目语言。不指定则自动检测。 |
| `target_architecture`  | object   | 架构规则（层次、允许的导入） |
| `docs`                 | string   | 包含 `.architecture.json` 的目录相对路径（如 `docs/arch`）。未指定时自动查找 `docs/arch`。 |
| `include_paths`        | string[] | 代码快照中包含的关键源文件相对路径。 |

**`target_architecture` 格式**（也是 `.architecture.json` 的格式）：
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
  "constraints": ["所有共享类型必须定义在 internal/domain"]
}
```

将 `.architecture.json` 放置在 `docs/arch/` 中，无需传递 `docs` 参数即可自动加载。

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

审计单个文件或目录：正确性、设计质量、耦合度、内聚性、潜在错误、复杂度。

**参数：**

| 参数                   | 类型   | 描述 |
|------------------------|--------|------|
| `module_path`          | string | 要审计的文件或目录路径 |
| `project_path`         | string | 项目根路径 |
| `provider`             | string | LLM 提供商 |
| `llm`                  | string | 模型名称 |
| `language`             | string | 响应语言：`ru`, `en`, `zh` |
| `programming_language` | string | 项目语言。不指定则自动检测。 |

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
      "project_path": "/path/to/project",
      "provider": "openai",
      "llm": "gpt-4o-mini",
      "programming_language": "go"
    }
  }
}
```

---

### 提供商和模型选择优先级

1. **工具调用参数** — `provider` / `llm`（最高优先级）
2. **启动时的 CLI 标志** — `-provider` / `-llm`
3. **`.env` 文件** — `PROVIDER` / `LLM`
4. **硬编码默认值** — 提供商：`mock`，模型：`gpt-4o`

---

## 项目架构

```
cmd/
  main.go                    # 入口点，CLI 标志

internal/
  config/
    config.go                # .env 加载和验证

  mcp/
    server.go                # MCP 服务器（JSON-RPC 2.0，HTTP + stdio）

  tools/
    executor.go              # 工具执行、提示词构建、报告持久化

  analyzer/
    engine.go                # 语言无关的分析编排器
    language.go              # ProjectAnalyzer 接口
    registry.go              # 自动检测（go.mod、tsconfig.json、pyproject.toml 等）
    golang/
      analyzer.go            # Go：通过 go/ast 构建导入图，go.mod，_test.go 检测
    python/
      analyzer.go            # Python：语言检测，空导入图（stub）
    typescript/
      analyzer.go            # TypeScript：语言检测，空导入图（stub）

  llm/
    provider.go              # LLMProvider 接口
    provider_impl.go         # OpenAI、Anthropic、Mock 实现 + 提示词构建器
    types.go                 # domain 类型别名

  domain/
    models.go                # 共享数据结构（AuditReport、Issue、ProjectMap 等）
```

---

## 扩展功能

### 添加新工具

1. 在 `internal/tools/executor.go` 中添加输入结构体。
2. 在 `ToolExecutor` 中添加方法。
3. 在 `internal/mcp/server.go` 的 `handleToolsList` 中注册工具 schema。
4. 在 `handleToolsCall` 中添加处理分支。

### 添加新语言

1. 创建 `internal/analyzer/<lang>/analyzer.go`，实现 `ProjectAnalyzer` 接口。
2. 在 `internal/analyzer/registry.go` 的注册切片中添加一行。

---

## 许可证

MIT
