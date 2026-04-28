# Architecture Compliance Specification

Этот документ определяет:
1. JSON-схему для описания архитектурных правил (`target_architecture`)
2. Правила этого проекта как эталонный пример

---

## Схема `target_architecture`

Файл `.architecture.json` в корне проекта (или объект в вызове инструмента).

```jsonc
{
  // Версия схемы. Текущая: "1.0"
  "version": "1.0",

  // Описание архитектурного стиля для контекста LLM
  "description": "string (optional)",

  // Слои — основа compliance check
  "layers": [
    {
      // Уникальное имя слоя
      "name": "string",

      // Человекочитаемое описание назначения слоя
      "description": "string (optional)",

      // Go package path patterns — подстроки пути пакета.
      // Пакет принадлежит слою, если его путь содержит хотя бы один pattern.
      // Пример: "internal/domain" матчит "github.com/org/repo/internal/domain"
      "patterns": ["string"],

      // Имена слоёв, из которых этому слою разрешено импортировать.
      // Пустой список [] означает: не импортирует ни один внутренний пакет.
      // Проверяется по реальному графу импортов.
      "allow_imports_from": ["string"]
    }
  ],

  // Явные запреты — дополняют правила слоёв
  // Нарушение любого из них: severity CRITICAL
  "forbidden_dependencies": [
    {
      "from": "string",   // имя слоя или pattern пакета
      "to": "string",     // имя слоя или pattern пакета
      "reason": "string"  // пояснение для LLM и разработчика
    }
  ],

  // Текстовые инварианты для LLM-анализа.
  // Движок не проверяет их статически — только передаёт в промт.
  // Используются для паттернов, которые нельзя выразить через граф импортов.
  "constraints": [
    "string"
  ]
}
```

### Правила сопоставления пакетов со слоями

1. Пакет сопоставляется с первым слоем, чей `patterns` содержит подстроку пути пакета.
2. Если пакет не сопоставлен ни с одним слоем — он игнорируется при проверке.
3. `allow_imports_from` проверяется по реальному графу импортов: для каждого импорта из пакета A
   проверяется, что целевой пакет B принадлежит слою из списка `allow_imports_from[A.layer]`.

### Severity нарушений

| Тип нарушения | Severity |
|---|---|
| Слой объявлен в правилах, но не найден в проекте | HIGH |
| Пакет импортирует пакет из запрещённого слоя | CRITICAL |
| Нарушение `forbidden_dependencies` | CRITICAL |
| Пакет не сопоставлен ни с одним слоем | LOW (информационное) |

---

## Правила этого проекта

Файл `.architecture.json` для `github.com/vzx7/opencode-mcp`:

```json
{
  "version": "1.0",
  "description": "Layered Go MCP server. Dependency direction: cmd → mcp → tools → {analyzer,llm} → domain. No upward dependencies.",

  "layers": [
    {
      "name": "domain",
      "description": "Pure data types. No internal imports allowed.",
      "patterns": ["internal/domain"],
      "allow_imports_from": []
    },
    {
      "name": "analyzer",
      "description": "Static Go project analysis. Depends only on domain.",
      "patterns": ["internal/analyzer"],
      "allow_imports_from": ["domain"]
    },
    {
      "name": "llm",
      "description": "LLM provider abstraction and prompt building. Depends only on domain.",
      "patterns": ["internal/llm"],
      "allow_imports_from": ["domain"]
    },
    {
      "name": "config",
      "description": "Configuration loading. No internal deps.",
      "patterns": ["internal/config"],
      "allow_imports_from": []
    },
    {
      "name": "tools",
      "description": "Tool execution orchestration. Depends on analyzer, llm, domain.",
      "patterns": ["internal/tools"],
      "allow_imports_from": ["domain", "analyzer", "llm"]
    },
    {
      "name": "mcp",
      "description": "MCP transport layer. Depends on tools, llm, domain.",
      "patterns": ["internal/mcp"],
      "allow_imports_from": ["domain", "llm", "tools"]
    },
    {
      "name": "cmd",
      "description": "Entry point. May depend on all internal packages.",
      "patterns": ["cmd"],
      "allow_imports_from": ["domain", "config", "llm", "analyzer", "tools", "mcp"]
    }
  ],

  "forbidden_dependencies": [
    {
      "from": "domain",
      "to": "analyzer",
      "reason": "domain is the lowest layer — must have zero internal imports"
    },
    {
      "from": "domain",
      "to": "llm",
      "reason": "domain is the lowest layer — must have zero internal imports"
    },
    {
      "from": "domain",
      "to": "tools",
      "reason": "domain is the lowest layer — must have zero internal imports"
    },
    {
      "from": "domain",
      "to": "mcp",
      "reason": "domain is the lowest layer — must have zero internal imports"
    },
    {
      "from": "analyzer",
      "to": "llm",
      "reason": "analyzer and llm are sibling layers — cross-dependency would create coupling"
    },
    {
      "from": "llm",
      "to": "analyzer",
      "reason": "analyzer and llm are sibling layers — cross-dependency would create coupling"
    },
    {
      "from": "llm",
      "to": "tools",
      "reason": "llm must not know about orchestration logic"
    },
    {
      "from": "analyzer",
      "to": "tools",
      "reason": "analyzer must not know about orchestration logic"
    },
    {
      "from": "tools",
      "to": "mcp",
      "reason": "tools must not depend on transport layer"
    }
  ],

  "constraints": [
    "All exported types shared between packages must be defined in internal/domain",
    "LLMProvider interface must remain the only interface in the system",
    "No package-level mutable state (global vars) except promptsByLang in internal/llm (loaded once at init)",
    "No init() side effects except prompt file loading in internal/llm/loader.go",
    "All LLM calls must go through ToolExecutor.acquireLLM semaphore",
    "Score in AuditReport must be set only by LLM response — engine enrichment must not modify Score"
  ]
}
```

---

## Использование

### Через MCP-инструмент

```json
{
  "name": "architecture_compliance_check",
  "arguments": {
    "project_path": "/path/to/project",
    "provider": "anthropic",
    "llm": "claude-opus-4-7",
    "language": "ru"
  }
}
```

*(если `.architecture.json` лежит в корне проекта — правила читаются автоматически)*

### С явной передачей правил

```json
{
  "name": "architecture_compliance_check",
  "arguments": {
    "project_path": "/path/to/project",
    "target_architecture": {
      "version": "1.0",
      "layers": [...]
    }
  }
}
```

### Через CI/CD (HTTP)

```bash
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "id": 1,
    "params": {
      "name": "architecture_compliance_check",
      "arguments": {
        "project_path": "/workspace",
        "language": "en"
      }
    }
  }'
```
