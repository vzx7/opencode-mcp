# AI Tech Lead MCP Server

MCP сервер для AI-ассистента, предоставляющий инструменты архитектурного аудита и ревью кода.

## Возможности

Сервер предоставляет 3 MCP tool:

| Tool | Описание |
|------|----------|
| `architecture_review` | Архитектурный аудит проекта целиком |
| `architecture_compliance_check` | Проверка соответствия заданной архитектуре |
| `module_audit` | Аудит отдельного файла или модуля |

---

## Конфигурация через .env

Создайте файл `.env` в корне проекта:

```bash
# Копируйте из .env.example
cp .env.example .env
```

### Параметры .env

| Переменная | Описание                                                              | По умолчанию       |
| ------------| -----------------------------------------------------------------------| --------------------|
| `PROVIDER` | LLM провайдер (`mock`, `openai`, или OpenAI-совместимый)              | `mock`             |
| `LLM`      | Модель                                                                | `gpt-4o`           |
| `API_KEY`  | API ключ                                                              | -                  |
| `ENDPOINT` | Кастомный endpoint                                                    | -                  |
| `PROJECT`  | Путь к проекту                                                        | текущая директория |
| `PORT`     | Порт                                                                  | `8080`             |
| `LANGUAGE` | Язык ответов (`ru`, `en`)                                             | `ru`               |

### Примеры .env

**Mock (без LLM):**
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

**Anthropic** *(не реализован — используется mock)*:
```env
PROVIDER=anthropic
LLM=claude-3-5-sonnet-20241022
API_KEY=sk-ant-...
```

### CLI флаги

Все флаги переопределяют соответствующие значения из `.env`:

```
-stdio         Запуск в stdio режиме для MCP клиентов (Claude Desktop, Cursor и др.)
-provider      LLM провайдер (переопределяет PROVIDER)
-llm           Модель (переопределяет LLM)
-api-key       API ключ (переопределяет API_KEY)
-endpoint      Кастомный endpoint (переопределяет ENDPOINT)
-port          Порт (переопределяет PORT)
-project       Путь к проекту (переопределяет PROJECT)
-debug-dir     Директория для отчётов (по умолчанию: <project>/debug)
-debug         Подробное логирование
```

**Примеры:**
```bash
# HTTP режим
go run ./cmd -provider openai -llm gpt-4o -project /path/to/project

# stdio режим для MCP клиентов
go run ./cmd -stdio -provider openai -llm gpt-4o -project /path/to/project

# Своя директория для отчётов
go run ./cmd -stdio -project /path/to/project -debug-dir /tmp/audit
```

### Debug режим

Запустите с флагом `-debug` для подробного логирования запросов к LLM, ответов и сохранения файлов:

```bash
go run ./cmd -debug -project /path/to/project
```

По умолчанию отчёты сохраняются в `<project>/debug/` в виде файлов `.md` и `.json`.

---

## Быстрый старт

### 1. Запуск сервера

```bash
go run ./cmd
```

По умолчанию сервер слушает на `http://localhost:8080`.

### 2. Подключение к opencode

Добавьте конфигурацию в `~/.opencode/mcp.json` (сервер загрузит настройки из `.env`):

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

После перезапуска opencode доступны команды:

```
/architecture-review project_path=/path/to/project
/architecture-compliance-check project_path=/path/to/project
/module-audit module_path=/path/to/module
```

---

## Использование tools

### architecture_review

Анализирует проект как систему, выявляет архитектурные проблемы.

**Параметры:**
- `project_path` (string, опционально) - путь к проекту
- `provider` (string, опционально) - LLM провайдер (`mock`, `openai`, или OpenAI-совместимый)
- `llm` (string, опционально) - модель (например `gpt-4o`)
- `language` (string, опционально) - язык ответа (`ru`, `en`)

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
      "language": "en"
    }
  }
}
```

**Из opencode:**
```
/architecture-review project_path=/path/to/project provider=openai llm=gpt-4o
```

**Ответ:** инструмент возвращает отчёт в формате Markdown в виде MCP content text. В директорию debug также сохраняются два файла:
- `architecture_review_<timestamp>.md` — читаемый отчёт
- `architecture_review_<timestamp>.json` — структурированный отчёт для AI-агентов:

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

Проверяет проект на соответствие заданной архитектуре.

**Параметры:**
- `project_path` (string, опционально) - путь к проекту
- `provider` (string, опционально) - LLM провайдер (`mock`, `openai`, или OpenAI-совместимый)
- `llm` (string, опционально) - модель
- `target_architecture` (object, опционально) - правила архитектуры
- `language` (string, опционально) - язык ответа (`ru`, `en`)

**target_architecture формат:**
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

**Из opencode:**
```
/architecture-compliance-check project_path=/path/to/project provider=anthropic llm=claude-3-5-sonnet-20241022
```

### module_audit

Аудитирует отдельный файл или модуль.

**Параметры:**
- `module_path` (string, опционально) - путь к файлу или модулю
- `project_path` (string, опционально) - путь к корню проекта
- `provider` (string, опционально) - LLM провайдер (`mock`, `openai`, или OpenAI-совместимый)
- `llm` (string, опционально) - модель
- `language` (string, опционально) - язык ответа (`ru`, `en`)

**При��ер вызова:**
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

**Из opencode:**
```
/module-audit module_path=/path/to/project/internal/service provider=openai llm=gpt-4o-mini
```

### Приоритет выбора провайдера и модели

1. **Аргументы при вызове tool** (самый высокий приоритет):
   - `provider=openai llm=gpt-4o` — динамически для каждого вызова

2. **CLI флаги при запуске сервера**:
   - `-provider=... -llm=...` — переопределяют `.env`

3. **`.env` файл**:
   - `PROVIDER=...`, `LLM=...` — default для всех вызовов

4. **Hardcoded defaults** (самый низкий приоритет):
   - Provider: `mock`, Model: `gpt-4o`

---

## Архитектура проекта

```
cmd/
  main.go              # Точка входа

internal/
  config/
    config.go        # Загрузка .env

  mcp/
    server.go       # MCP сервер (JSON-RPC)

  tools/
    executor.go    # Логика выполнения tools

  analyzer/
    engine.go      # Анализ проекта и модулей

  llm/
    provider.go      # Интерфейс LLM провайдера
    provider_impl.go  # Реализации провайдеров
    types.go         # Type aliases

  domain/
    models.go      # Data models
```

---

## Расширение функциональности

### Добавление нового tool

1. Добавьте input struct в `internal/tools/executor.go`:
   ```go
   type NewToolInput struct {
       Param1 string `json:"param1"`
   }
   ```

2. Добавьте метод в `ToolExecutor`:
   ```go
   func (te *ToolExecutor) NewTool(ctx context.Context, input NewToolInput) (*domain.AuditReport, error) {
       // логика
   }
   ```

3. Зарегистрируйте tool в `internal/mcp/server.go`:
   ```go
   {
       Name:        "new_tool",
       Description: "Description",
       InputSchema: ToolInputSchema{...},
   }
   ```

4. Добавьте обработку в `handleToolsCall`:
   ```go
   case "new_tool":
       // вызов
   ```

---

## Лицензия

MIT