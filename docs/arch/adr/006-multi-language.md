# ADR-006: Плагин-архитектура для поддержки нескольких языков программирования

**Статус:** принято  
**Дата:** 2025-04

## Контекст

Изначально MCP-сервер анализировал только Go-проекты. Всё языкоспецифичное поведение
(расширения файлов, фильтрация тестов, граф импортов через `go/ast`, счёт символов, парсинг `go.mod`)
было захардкожено в `engine.go` и `executor.go`.

Задача: сделать сервер универсальным аудитором, поддерживающим несколько языков программирования,
так чтобы добавление нового языка не требовало изменений в оркестрирующем коде.

**Ограничения:**
- Нет новых внешних зависимостей
- Оркестрирующий код (`executor.go`, `mcp/server.go`, `llm/`) должен меняться минимально
- Python и TypeScript могут быть stub-реализациями на первом шаге

## Решение

### Интерфейс `ProjectAnalyzer`

Единственная точка расширения для нового языка — реализовать интерфейс в `internal/analyzer/language.go`:

```go
type ProjectAnalyzer interface {
    Name() string
    Detect(rootPath string) bool
    SourceExtensions() []string
    IsTestFile(path string) bool
    HasTestCounterpart(sourcePath string) bool
    BuildImportGraph(rootPath string) (*domain.ImportGraph, error)
    CountSymbols(src []byte) (functions, types int)
    SnippetLang() string
    ModuleName(rootPath string) string
}
```

`BuildImportGraph` возвращает только рёбра (`graph.Edges`). Циклы и layer violations
добавляет `Engine` с помощью языконезависимых алгоритмов (`findCycles`, `findLayerViolations`).

### Реестр анализаторов (`internal/analyzer/registry.go`)

```go
var registered = []ProjectAnalyzer{
    &golang.Analyzer{},
    &typescript.Analyzer{},
    &python.Analyzer{},
}

func Detect(rootPath string) ProjectAnalyzer { ... }  // первый совпавший, фолбэк → Go
func ByName(name string) (ProjectAnalyzer, bool) { ... }
```

### Структура файлов

```
internal/analyzer/
  language.go           — интерфейс ProjectAnalyzer
  registry.go           — Detect(), ByName(), список анализаторов
  engine.go             — языконезависимый Engine; граф-алгоритмы
  golang/analyzer.go    — полная реализация: go/ast, go.mod, _test.go
  python/analyzer.go    — stub: определение языка, пустой граф
  typescript/analyzer.go — stub: определение языка, пустой граф
```

`golang/`, `python/`, `typescript/` — отдельные Go-пакеты. Они не импортируют родительский
пакет `analyzer`, поэтому нет циклических зависимостей.

### Конструкторы `Engine`

```go
func New(rootPath string) *Engine                    // авто-определение
func NewWithLang(rootPath, langName string) *Engine  // явное указание, фолбэк на авто
```

### Параметр `programming_language`

Добавлен в `ToolInput` и в JSON-схему всех трёх инструментов:
```json
"programming_language": {
  "type": "string",
  "description": "go, python, typescript. Авто-определение если не указан."
}
```

Приоритет: явный аргумент → `Detect(rootPath)` → фолбэк на Go.

### Языконезависимость промтов

`Engine.SnippetLang()` → `lang.SnippetLang()` возвращает тег языка для markdown-блоков.
`BuildReviewPrompt` и `BuildModuleAuditPrompt` принимают `snippetLang string` вместо
захардкоженного `` ```go ``. Это единственное изменение в `llm/` пакете.

## Обоснование

**Почему Strategy/Plugin pattern, а не switch по имени языка:**
- Switch в оркестрирующем коде нарушает Open/Closed: каждый новый язык требует правки `executor.go`
- Интерфейс изолирует знание о языке в одном месте; `executor.go` не знает о языках вообще

**Почему отдельные Go-пакеты (`golang/`, `python/`), а не методы Engine:**
- Нет циклических импортов: `golang/` не знает про `analyzer`, только про `domain`
- Каждый анализатор компилируется отдельно; его зависимости изолированы (go/ast только в `golang/`)
- Чёткая граница: всё, что специфично для языка — в своём пакете

**Почему порядок Detect: Go → TypeScript → Python:**
- TypeScript-проекты часто имеют `tsconfig.json` рядом с `package.json`, но без маркеров Python
- Python-маркеры (`requirements.txt`) могут встречаться в mixed-репозиториях с JS-инфраструктурой
- Go-проекты всегда имеют `go.mod` — надёжный и однозначный маркер, поэтому первый

**Почему фолбэк на Go:**
- Исторически сервер был Go-only; все тесты и интеграции ожидают Go-поведение
- Go — наиболее полная реализация; лучше вернуть частичный анализ, чем полностью пустой

**Почему языконезависимые алгоритмы остаются в `engine.go`:**
- `findCycles` и `findLayerViolations` работают на `map[string][]string` — им неважно, как построен граф
- Дублировать их в каждом языковом анализаторе избыточно

## Рассмотренные альтернативы

**Switch по имени языка в `executor.go`:** требует правок оркестрирующего кода при добавлении языка.  
**Методы Engine с if/switch внутри:** скрывает сложность, но не убирает её; Engine разрастается.  
**Отдельные MCP-инструменты для каждого языка:** взрывной рост числа инструментов, дублирование схем.  
**Runtime-плагины (`.so`):** избыточная инфраструктура; усложняет сборку и деплой.

## Последствия

- Новый язык добавляется в 2 шага: реализовать `ProjectAnalyzer` + добавить строку в `registry.go`
- Python и TypeScript в текущей реализации — stubs: `BuildImportGraph` возвращает пустой граф;
  статические нарушения не обнаруживаются, но LLM видит исходный код
- `domain.Module.GoFiles` переименовано в `SourceFiles` (JSON: `"source_files"`)
- `engine.go` больше не импортирует `go/ast`, `go/parser`, `go/token`
- Тесты в `executor_test.go` и `provider_test.go` обновлены: передают engine в snapshot-методы
  и `snippetLang` в функции построения промтов
