# AI Tech Lead MCP Server

MCP сервер для AI-ассистентов, предоставляющий инструменты архитектурного аудита и ревью кода. Поддерживает Go, TypeScript, Python, Rust и Java проекты.

## Возможности

Сервер предоставляет 3 MCP tool:

| Tool | Описание |
|------|----------|
| `architecture_review` | Полный архитектурный аудит проекта со снапшотом ключевых файлов |
| `architecture_compliance_check` | Проверка соответствия правилам архитектуры и документации |
| `module_audit` | Аудит отдельного файла или модуля |

---

## Конфигурация через .env

Создайте файл `.env` в корне проекта:

```bash
cp .env.example .env
```

### Параметры .env

| Переменная           | Описание                                              | По умолчанию       |
|----------------------|-------------------------------------------------------|--------------------|
| `PROVIDER`           | LLM провайдер (`mock`, `openai`, `anthropic`)         | `mock`             |
| `LLM`                | Название модели                                       | `gpt-4o`           |
| `OPENAI_API_KEY`     | API ключ OpenAI                                       | —                  |
| `ANTHROPIC_API_KEY`  | API ключ Anthropic                                    | —                  |
| `ENDPOINT`           | Кастомный endpoint (для OpenAI-совместимых API)       | —                  |
| `PORT`               | Порт HTTP сервера                                     | `8080`             |
| `LANGUAGE`           | Язык ответов (`ru`, `en`, `zh`)                       | `ru`               |

> **Примечание:** Неизвестные провайдеры работают через OpenAI-совместимый API. Используйте `ENDPOINT` для сторонних провайдеров (OpenRouter, Groq, локальный Ollama).

> **Примечание:** таймаут HTTP-запросов к LLM — 10 минут.

### Примеры .env

**Mock (без LLM, для тестирования):**
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

### CLI флаги

Все флаги переопределяют соответствующие значения из `.env`:

```
-stdio       Запуск в stdio режиме для MCP клиентов (Claude Desktop, Cursor и др.)
-provider    LLM провайдер (переопределяет PROVIDER)
-llm         Модель (переопределяет LLM)
-endpoint    Кастомный endpoint (переопределяет ENDPOINT)
-port        Порт (переопределяет PORT)
-debug       Подробное логирование
```

> **Безопасность:** храните API ключи в `.env` — файл добавлен в `.gitignore`.

**Примеры:**
```bash
# HTTP режим
go run ./cmd -provider openai -llm gpt-4o

# stdio режим для MCP клиентов
go run ./cmd -stdio -provider anthropic -llm claude-3-5-sonnet-20241022
```

### Debug режим

```bash
go run ./cmd -debug
```

Промпты сохраняются в `<project_path>/debug/input/`, а отчёты — в `<project_path>/debug/` в виде `.md` и `.json` файлов при каждом вызове tool.

---

## Быстрый старт

### 1. Запуск сервера

```bash
go run ./cmd
```

По умолчанию сервер слушает на `http://localhost:8080`.

### 2. Подключение к opencode

Добавьте в `~/.opencode/mcp.json`:

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

После перезапуска opencode доступны скилл-команды:

```
/architecture_review
/architecture_compliance_check
/module_audit
```

---

## Использование tools

### architecture_review

Анализирует проект как систему: слои, граф импортов, метрики файлов, git hotspots. Принимает список ключевых файлов через `include_paths`, чтобы сфокусировать LLM на архитектурно значимых частях.

**Параметры:**

| Параметр               | Тип      | Описание |
|------------------------|----------|----------|
| `project_path`         | string   | Путь к проекту (**обязательный**) |
| `provider`             | string   | LLM провайдер: `mock`, `openai`, `anthropic` |
| `llm`                  | string   | Модель (например, `gpt-4o`, `claude-3-5-sonnet-20241022`) |
| `language`             | string   | Язык ответа: `ru`, `en`, `zh` |
| `programming_language` | string   | Язык проекта: `go`, `python`, `typescript`, `rust`, `java`. Определяется автоматически. |
| `include_paths`        | string[] | Относительные пути к ключевым файлам для снапшота. Если не указан — автодискавери по размеру. |

**Пример вызова:**
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
      "language": "ru",
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

**Ответ:** отчёт в формате Markdown в виде MCP content text. В директорию debug сохраняются два файла:
- `architecture_review_<timestamp>.md` — читаемый отчёт
- `architecture_review_<timestamp>.json` — структурированный отчёт:

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

Проверяет проект на соответствие заданным правилам архитектуры. Опционально принимает директорию с архитектурной документацией (ADR, спецификации) как эталон для LLM.

**Параметры:**

| Параметр               | Тип      | Описание |
|------------------------|----------|----------|
| `project_path`         | string   | Путь к проекту (**обязательный**) |
| `provider`             | string   | LLM провайдер |
| `llm`                  | string   | Модель |
| `language`             | string   | Язык ответа: `ru`, `en`, `zh` |
| `programming_language` | string   | Язык проекта. Определяется автоматически. |
| `target_architecture`  | object   | Правила архитектуры (слои, допустимые импорты) |
| `docs`                 | string   | Относительный путь к директории с `.architecture.json` (например, `docs/arch`). Если не передан, ищется автоматически в `docs/arch`. |
| `include_paths`        | string[] | Относительные пути к ключевым файлам для снапшота. |

**Формат `target_architecture`** (он же формат файла `.architecture.json`):
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
  "constraints": ["Все общие типы должны быть в internal/domain"]
}
```

Поместите `.architecture.json` в `docs/arch/` — он будет загружен автоматически без передачи параметра `docs`.

**Пример вызова:**
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

Аудитирует отдельный файл или директорию: корректность, качество дизайна, coupling, cohesion, потенциальные баги, сложность.

**Параметры:**

| Параметр               | Тип    | Описание |
|------------------------|--------|----------|
| `module_path`          | string | Путь к файлу или директории |
| `project_path`         | string | Путь к корню проекта (**обязательный**) |
| `provider`             | string | LLM провайдер |
| `llm`                  | string | Модель |
| `language`             | string | Язык ответа: `ru`, `en`, `zh` |
| `programming_language` | string | Язык проекта. Определяется автоматически. |

**Пример вызова:**
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

### Приоритет выбора провайдера и модели

1. **Аргументы при вызове tool** — `provider` / `llm` (наивысший приоритет)
2. **CLI флаги при запуске** — `-provider` / `-llm`
3. **`.env` файл** — `PROVIDER` / `LLM`
4. **Hardcoded defaults** — провайдер: `mock`, модель: `gpt-4o`

---

## Архитектура проекта

```
cmd/
  main.go                    # Точка входа, CLI флаги

internal/
  config/
    config.go                # Загрузка и валидация .env

  mcp/
    server.go                # MCP сервер (JSON-RPC 2.0, HTTP + stdio)

  tools/
    executor.go              # Выполнение tools, построение промптов, сохранение отчётов

  analyzer/
    engine.go                # Языконезависимый оркестратор анализа
    language.go              # Интерфейс ProjectAnalyzer
    registry.go              # Автодетекция (go.mod, tsconfig.json, pyproject.toml, ...)
    golang/
      analyzer.go            # Go: граф импортов через go/ast, go.mod, _test.go
    python/
      analyzer.go            # Python: детекция языка, пустой граф (stub)
    typescript/
      analyzer.go            # TypeScript: детекция языка, пустой граф (stub)

  llm/
    provider.go              # Интерфейс LLMProvider
    provider_impl.go         # Реализации OpenAI, Anthropic, Mock + построители промптов
    types.go                 # Type aliases для domain типов

  domain/
    models.go                # Общие структуры (AuditReport, Issue, ProjectMap, ...)
```

---

## Расширение функциональности

### Добавление нового tool

1. Добавьте input struct в `internal/tools/executor.go`.
2. Добавьте метод в `ToolExecutor`.
3. Зарегистрируйте схему tool в `internal/mcp/server.go` (`handleToolsList`).
4. Добавьте обработку в `handleToolsCall`.

### Добавление нового языка

1. Создайте `internal/analyzer/<lang>/analyzer.go`, реализующий `ProjectAnalyzer`.
2. Добавьте одну строку в слайс реестра в `internal/analyzer/registry.go`.

---

## Лицензия

MIT
