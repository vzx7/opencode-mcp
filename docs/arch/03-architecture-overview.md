# Architecture Overview

## Стиль архитектуры

Классическая **слоистая архитектура** с жёсткими правилами направления зависимостей.  
Каждый слой зависит только от слоёв ниже. Зависимости через интерфейсы там, где это нужно для
тестируемости и расширяемости (LLMProvider, ProjectAnalyzer).

## Слои и их ответственности

```
┌─────────────────────────────────────────┐  ← уровень 4
│  cmd                                    │  точка входа, CLI-флаги, сборка графа
│  cmd/main.go                            │  зависимостей, старт сервера
└────────────────┬────────────────────────┘
                 │
┌────────────────▼────────────────────────┐  ← уровень 3
│  internal/mcp                           │  транспорт: HTTP + stdio
│  server.go                              │  JSON-RPC 2.0, маршрутизация методов,
│                                         │  сериализация запросов/ответов
└────────────────┬────────────────────────┘
                 │
┌────────────────▼────────────────────────┐  ← уровень 2
│  internal/tools                         │  оркестрация: tool execution
│  executor.go                            │  вызов analyzer + llm, семафор,
│                                         │  path validation, persistReport
└────────────┬───────────┬────────────────┘
             │           │
┌────────────▼──────────┐│  ┌────────────────────────────────────┐  ← уровень 1
│ internal/analyzer     ││  │ internal/llm                       │
│                       ││  │                                    │
│ engine.go             ││  │ provider.go   — интерфейс          │
│   Engine              ││  │               LLMProvider          │
│   BuildProjectMap     ││  │ provider_impl — OpenAI, Anthropic  │
│   BuildImportGraph    ││  │ loader.go     — промты из JSON     │
│   CollectFileMetrics  ││  │ provider_impl — BuildReviewPrompt, │
│   CollectGitHotspots  ││  │                BuildCompliance-    │
│   CheckCompliance     ││  │                Prompt,             │
│   AuditModule         ││  │                BuildModuleAudit-   │
│                       ││  │                Prompt              │
│ language.go           ││  └────────────────────────────────────┘
│   ProjectAnalyzer     ││
│   (interface)         ││
│                       ││
│ registry.go           ││
│   Detect(), ByName()  ││
│                       ││
│ golang/analyzer.go    ││
│   go/ast, go.mod      ││
│                       ││
│ python/analyzer.go    ││
│   (stub)              ││
│                       ││
│ typescript/analyzer.go││
│   (stub)              ││
└───────────────────────┘│
                          │
┌─────────────────────────▼─────────────────────────────────────┐  ← уровень 0
│  internal/domain                                               │
│  models.go                                                     │
│                                                                │
│  AuditReport, Issue, ProjectMap, Module (SourceFiles),         │
│  ImportGraph, LayerViolation, ArchitectureRules,               │
│  FileMetric, GitHotspot                                        │
└────────────────────────────────────────────────────────────────┘
```

**Дополнительный слой конфигурации** (горизонтальный, не в основной цепочке):
```
internal/config  — загрузка .env, валидация, Config struct
```
Зависит только от stdlib. Используется только из `cmd`.

## Правило зависимостей

```
cmd → mcp → tools → analyzer          → domain
                  → analyzer/golang   → domain
                  → analyzer/python   → domain
                  → analyzer/typescript → domain
                  → llm               → domain
          → config (только из cmd)
```

Ни один пакет не импортирует слой выше себя.  
`domain` не импортирует ни один внутренний пакет.  
`analyzer/golang`, `analyzer/python`, `analyzer/typescript` не импортируют родительский пакет `analyzer` (нет циклических зависимостей).

## Ключевые потоки данных

### architecture_review

```
MCP Client
  → [JSON-RPC] server.go: handleToolsCall
  → executor.go: ArchitectureReview
      → analyzer.NewWithLang(path, programmingLanguage)
          → registry.Detect(path) если язык не задан явно
      → engine.BuildProjectMap
      → engine.BuildImportGraph
          → lang.BuildImportGraph   (язык-специфичная часть: только рёбра)
          → findCycles              (языконезависимый DFS)
          → findLayerViolations     (языконезависимый топосорт)
      → engine.CollectFileMetrics
          → lang.CountSymbols, lang.HasTestCounterpart
      → engine.CollectGitHotspots  (git log, фильтр через IsSourceFile)
      → executor.buildCodeSnapshot (100k char, engine.IsSourceFile)
      → llm.BuildReviewPrompt      (snippetLang из lang.SnippetLang())
      → llmProvider.Complete       (с retry + semaphore)
      → parseLLMResponse
      → enrichWithLocalAnalysis    (структурные issues, без score)
      → enrichWithGraphAnalysis    (cycles + violations, без score)
  → server.persistReport           (.md + .json)
  → [JSON-RPC response] MCP Client
```

### architecture_compliance_check

```
MCP Client
  → server.go: handleToolsCall
  → executor.go: ArchitectureComplianceCheck
      → analyzer.NewWithLang(path, programmingLanguage)
      → engine.BuildProjectMap
      → engine.BuildImportGraph
      → rules = input.TargetArchitecture
             ?? load from docs/arch/.architecture.json (auto-discovery)
             ?? getDefaultRules()
      → engine.CheckCompliance(rules, pm)
          — проверяет наличие слоёв
          — проверяет Allow-правила по реальным импортам
      → llm.BuildCompliancePrompt(rules, pm, graph)
      → llmProvider.Complete
      → merge: LLM score + issues поверх статических findings
  → server.persistReport
  → [JSON-RPC response]
```

### module_audit

```
MCP Client
  → server.go: handleToolsCall
  → executor.go: ModuleAudit
      → analyzer.NewWithLang(path, programmingLanguage)
      → engine.AuditModule         (структурная проверка через IsSourceFile)
      → executor.readModuleContent (читает файлы через engine.IsSourceFile)
      → llm.BuildModuleAuditPrompt (snippetLang из lang.SnippetLang())
      → llmProvider.Complete
      → LLM score + issues заменяют статический отчёт
  → server.persistReport
  → [JSON-RPC response]
```

## Интерфейсы между компонентами

### LLMProvider (internal/llm/provider.go)

```go
type LLMProvider interface {
    Complete(ctx context.Context, prompt string, language string) (string, error)
    Name() string
}
```

Позволяет подменять LLM-реализацию (OpenAI / Anthropic / Mock) без изменения вызывающего кода.

### ProjectAnalyzer (internal/analyzer/language.go)

```go
type ProjectAnalyzer interface {
    Name() string                                                  // "go", "python", "typescript"
    Detect(rootPath string) bool                                   // есть ли go.mod / pyproject.toml / tsconfig.json
    SourceExtensions() []string                                    // [".go"], [".py"], [".ts", ".tsx"]
    IsTestFile(path string) bool                                   // _test.go, test_*.py, *.test.ts
    HasTestCounterpart(sourcePath string) bool                     // существует ли соседний тест-файл
    BuildImportGraph(rootPath string) (*domain.ImportGraph, error) // только рёбра; циклы добавляет Engine
    CountSymbols(src []byte) (functions, types int)                // экспортируемые символы
    SnippetLang() string                                           // тег для markdown-блоков ("go", "python")
    ModuleName(rootPath string) string                             // имя модуля из манифеста
}
```

Единственная точка расширения для нового языка: реализовать интерфейс и добавить в `registry.go`.

### Обнаружение языка (internal/analyzer/registry.go)

```go
Detect(rootPath string) ProjectAnalyzer  // первый совпавший анализатор, фолбэк → Go
ByName(name string) (ProjectAnalyzer, bool)
```

Порядок проверки: Go → TypeScript → Python. Фолбэк: Go.

## Конфигурация

Приоритет: `CLI flags` > `.env file` > `hardcoded defaults`

| Параметр | Дефолт | Описание |
|---|---|---|
| PROVIDER | mock | LLM-провайдер |
| LLM | gpt-4o | Модель |
| PORT | 8080 | HTTP-порт |
| LANGUAGE | ru | Язык ответа |
| PROJECT | — | Дефолтный путь к проекту |
| OPENAI_API_KEY | — | Ключ OpenAI |
| ANTHROPIC_API_KEY | — | Ключ Anthropic |

Язык программирования (`programming_language`) передаётся per-call через аргументы инструмента,
не через глобальный конфиг.
