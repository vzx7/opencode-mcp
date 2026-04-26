# Аудит проекта: AI-MCP code-auditor

**Дата:** 2026-04-26  
**Роль:** Tech Lead  
**Статус сборки:** ✅ собирается  
**Тесты:** только `internal/mcp` — 5 тестов, 0.002s  

---

## Общая оценка: 58 / 100

Проект представляет собой MCP-сервер (JSON-RPC 2.0) с двумя режимами работы — HTTP и stdio — для LLM-based code review. Идея хорошая, структура пакетов разумная. Но есть критические дыры, которые блокируют production-использование.

---

## Архитектура

```
cmd/main.go                  — точка входа, парсинг флагов
internal/
  config/config.go           — загрузка конфига (.env + валидация)
  domain/models.go           — доменные типы (AuditReport, Issue, ...)
  analyzer/engine.go         — статический анализ структуры проекта
  llm/
    provider.go              — интерфейс LLMProvider + Mock + фабрика
    provider_impl.go         — OpenAI + Anthropic реализации + prompt builders
    types.go                 — type aliases (ProjectMap, ArchitectureRules)
  mcp/
    server.go                — JSON-RPC dispatcher (HTTP + stdio)
    server_test.go           — тесты saveDebugResponse
  tools/
    executor.go              — вызов инструментов (ArchitectureReview, Compliance, ModuleAudit)
```

**Хорошо:** четкое разделение domain / analyzer / llm / mcp / tools. Код читается.  
**Плохо:** prompt builders живут в пакете `llm`, хотя это прикладная логика — должно быть в `tools` или отдельном `prompt` пакете.

---

## Критические проблемы (блокеры)

### CRIT-1: AnthropicProvider не реализован
**Файл:** `internal/llm/provider_impl.go:133`  
```go
func (p *AnthropicProvider) Complete(ctx context.Context, prompt string, language string) (string, error) {
    return "", &notImplementedError{"anthropic provider needs implementation"}
}
```
Выбор `PROVIDER=anthropic` всегда вернёт ошибку. Молчаливый фоллбэк в `NewServer` скроет это от пользователя (он получит Mock-ответы, думая что работает с Claude).

### CRIT-2: HTTP-клиент без таймаута
**Файл:** `internal/llm/provider_impl.go:31`  
```go
client: &http.Client{Timeout: 0},
```
Commit `4a6d45e` явно убрал таймаут. Зависший LLM-запрос заблокирует горутину навсегда. В production это гарантированный goroutine leak.

### CRIT-3: Глобальный HTTP mux
**Файл:** `internal/mcp/server.go:493`  
```go
http.HandleFunc("/", s.HandleJSONRPC)
```
Используется `DefaultServeMux`. При двух вызовах `Start()` будет паника (handler already registered). Надо `http.NewServeMux()`.

### CRIT-4: API-ключ в аргументах CLI
**Файл:** `cmd/main.go:22`  
```go
apiKey := flag.String("api-key", "", "API key (overrides ENV)")
```
Ключ виден в `ps aux` для всех пользователей системы. API-ключи должны передаваться только через переменные окружения или secrets manager.

---

## Высокие проблемы

### HIGH-1: Валидация конфига не работает после CLI override
**Файл:** `cmd/main.go:38-56`  
`cfg.Validate()` вызывается только внутри `config.Load()`. Если пользователь передаёт `--provider openai` без `--api-key`, новое значение не валидируется — сервер стартует и падает при первом запросе.  
**Фикс:** вынести `Validate()` в отдельный вызов после override или повторно вызвать его.

### HIGH-2: Нет лимита на размер тела запроса
**Файл:** `internal/mcp/server.go:96`  
```go
body, err := io.ReadAll(r.Body)
```
Без `http.MaxBytesReader`. Злоумышленник может отправить гигабайтный payload и исчерпать память.

### HIGH-3: `analyzeGoMod` не парсит зависимости
**Файл:** `internal/analyzer/engine.go:163`  
```go
if strings.HasPrefix(line, "require (") || strings.HasPrefix(line, "\trequire") {
```
Строки внутри блока `require ( ... )` начинаются с `\t` + имя пакета (например `\tgithub.com/joho/godotenv v1.5.1`), а не с `\trequire`. Условие никогда не выполняется — `pm.Dependencies` всегда пуст. Зависимости в отчёте никогда не отражаются.

### HIGH-4: `AuditModule` кладёт JSON в Summary
**Файл:** `internal/analyzer/engine.go:288-289`  
```go
data, _ := json.MarshalIndent(report, "", "  ")
report.Summary = string(data)
```
Отчёт сериализует сам себя в JSON и кладёт это в поле `Summary`. Затем `executor.go` перезаписывает `Summary` LLM-ответом. Это артефакт отладки, оставленный в коде. Запутывает и замедляет — лишний marshal.

---

## Средние проблемы

### MED-1: Дублирование кода: `persistReport` vs `saveDebugResponse`
**Файл:** `internal/mcp/server.go:383-470`  
Два метода делают одно и то же. `persistReport` используется в продакшн-хэндлерах, `saveDebugResponse` — только в тестах. Надо оставить один, обновить тесты.

### MED-2: `formatReport` возвращает сырой JSON как текст
**Файл:** `internal/mcp/server.go:425-428`  
```go
func (s *Server) formatReport(report *domain.AuditReport) string {
    data, _ := json.MarshalIndent(report, "", "  ")
    return string(data)
}
```
MCP-клиент (Claude, Cursor) получает машинный JSON вместо читаемого markdown-отчёта. Нужно форматировать `AuditReport` в человекочитаемый текст.

### MED-3: `extractToolName` — хрупкое сопоставление по строкам
**Файл:** `internal/llm/provider.go:73-89`  
Mock-провайдер определяет тип инструмента по подстрокам в промпте. Изменение формулировки в prompt — и mock начнёт возвращать дефолтный `architecture_review` для всех запросов. Лучше передавать toolName явным параметром.

### MED-4: `logger` nil при `debug=false`
**Файл:** `internal/tools/executor.go:42-45`  
`te.logger` инициализируется только в `SetDebug(true)`. Все вызовы `te.logger.Printf` обёрнуты в `if te.debug`, поэтому паники нет сейчас — но это хрупкая конструкция. Любой новый лог вне `if te.debug` вызовет nil pointer dereference.

### MED-5: Случайность в имени файла не гарантирует уникальность
**Файл:** `internal/mcp/server.go:398`  
```go
rand.Intn(9999)
```
При параллельных запросах в одну секунду `timestamp + rand` может совпасть. Надо использовать `time.Now().UnixNano()` или `uuid`.

### MED-6: Коментарии-артефакты в коде
**Файл:** `internal/mcp/server.go:250, 301, 382`  
```go
// FIXED VERSION (key fixes: correct content saving, better logging, safer debug handling)
// ✅ FIX: always save FULL report, not Summary
// ✅ NEW: centralized persistence
```
Это задачи, которые должны быть в git commit message или issue tracker — не в production коде.

---

## Низкие проблемы

### LOW-1: Нет retry/backoff для LLM-вызовов
Одна попытка. LLM API дают rate limit 429 и временные 5xx — нужен хотя бы простой exponential backoff.

### LOW-2: Отсутствуют MCP методы `ping` и `notifications/list`
Некоторые MCP-клиенты ожидают эти методы при инициализации. Их отсутствие приводит к error-логам на стороне клиента.

### LOW-3: `BuildProjectMap` игнорирует корень с Go-файлами
**Файл:** `internal/analyzer/engine.go:39-54`  
Сканируются только директории `internal`, `pkg`, `cmd`. Проект с `main.go` в корне (single-package project) не обнаружит ни одного модуля и получит штраф `-20` к score.

### LOW-4: Только `go` файлы в `ModuleAudit`
`readModuleContent` собирает только `*.go` файлы, исключая `_test.go`. Но для аудита было бы полезно видеть тесты — они показывают контракты и покрытие.

### LOW-5: `.env` в репозитории
Файл `.env` с реальным эндпоинтом (`https://api.vsellm.ru/v1/chat/completions`) закоммичен в git. Добавить в `.gitignore` (уже есть) — но файл уже в истории. `git rm --cached .env`.

---

## Тестовое покрытие

| Пакет | Тесты | Статус |
|-------|-------|--------|
| `mcp/server.go` | 5 (только saveDebugResponse/debug dir) | ⚠️ неполное |
| `analyzer/engine.go` | 0 | ❌ |
| `tools/executor.go` | 0 | ❌ |
| `llm/provider*.go` | 0 | ❌ |
| `config/config.go` | 0 | ❌ |
| HTTP handler (JSON-RPC) | 0 | ❌ |
| Stdio mode | 0 | ❌ |

Единственный тест `TestSaveDebugResponse` проверяет deprecated метод `saveDebugResponse`, а не `persistReport`, который реально используется.

---

## TODO List (приоритет)

### P0 — делать немедленно (блокеры)
- [ ] **CRIT-1** Реализовать `AnthropicProvider.Complete` через Anthropic Messages API
- [ ] **CRIT-2** Добавить таймаут в `http.Client` (рекомендую 90s для LLM)
- [ ] **CRIT-3** Перейти на `http.NewServeMux()` вместо `DefaultServeMux`
- [ ] **CRIT-4** Убрать `--api-key` из CLI флагов; принимать только через ENV; добавить предупреждение в README

### P1 — делать до первого релиза
- [ ] **HIGH-1** Вызвать `cfg.Validate()` после применения CLI overrides в `main.go`
- [ ] **HIGH-2** Обернуть `r.Body` в `http.MaxBytesReader(w, r.Body, 1<<20)` (1MB limit)
- [ ] **HIGH-3** Починить `analyzeGoMod` — парсить строки вида `\tmodule v1.2.3` внутри require-блока
- [ ] **HIGH-4** Убрать самосериализацию отчёта в `AuditModule` (`engine.go:288-289`)
- [ ] **MED-1** Удалить `saveDebugResponse`, обновить тесты на `persistReport`
- [ ] **MED-2** Заменить `formatReport` на markdown-форматтер для читаемого вывода в MCP-клиенте
- [ ] **MED-6** Очистить TODO/FIX-комментарии из кода, перенести контекст в git history

### P2 — делать в следующем спринте
- [ ] **MED-3** Передавать `toolName` явным параметром в `LLMProvider.Complete` вместо string-matching в промпте
- [ ] **MED-4** Инициализировать `te.logger` всегда (не только при `debug=true`), убрать nil-риск
- [ ] **MED-5** Использовать `time.Now().UnixNano()` или `crypto/rand` для уникальных имён файлов
- [ ] **LOW-1** Добавить retry с exponential backoff для LLM-запросов (3 попытки)
- [ ] **LOW-2** Реализовать `ping` и `notifications/list` для полной MCP-совместимости
- [ ] **LOW-3** Расширить `BuildProjectMap` — сканировать Go-файлы в корне проекта
- [ ] **LOW-5** `git rm --cached .env`, убедиться что в CI нет утечки секретов

### P3 — техдолг / улучшения
- [ ] Написать тесты для `analyzer/engine.go` (BuildProjectMap, CheckCompliance, AuditModule)
- [ ] Написать интеграционный тест для `HandleJSONRPC` с mock LLM
- [ ] Написать тест для stdio-режима
- [ ] Написать тест для `config.Validate()`
- [ ] Перенести prompt builders из `llm/provider_impl.go` в отдельный пакет `internal/prompt`
- [ ] Добавить structured logging (slog, Go 1.21) вместо `log.Logger`
- [ ] Добавить graceful shutdown с таймаутом для HTTP-сервера (`http.Server.Shutdown`)
- [ ] Добавить поддержку context cancellation в stdio-цикле (сейчас `context.Background()`)
- [ ] Добавить `LANGUAGE` в список доступных языков вместо hardcode `ru`/`en`

---

## Резюме

| Категория | Найдено |
|-----------|---------|
| Критические | 4 |
| Высокие | 4 |
| Средние | 6 |
| Низкие | 5 |
| **Итого** | **19** |

Проект в состоянии MVP-прототипа: работает для mock-провайдера, реальные LLM-вызовы частично функциональны (OpenAI/compatible), Anthropic — нет. Для production-деплоя обязательно закрыть P0 и P1.
