# Refactoring Review — замечания по итогам агента

Code review изменений согласно `refactoring-plan.md`.

---

## Что сделано корректно

- `defaultPath` полностью удалён из executor и server
- `project_path is required` возвращается во всех трёх инструментах
- Флаги `-project` и `-debug-dir` убраны из `cmd/main.go`
- `config.go` — поле `Project` удалено
- `savePrompt` вызывается только при `te.debug == true`
- `persistReport` сохраняет только при непустом `debugDir`

---

## Проблемы

### 1. `TestArchitectureReview` почти пустой — нарушение плана

**Файл:** `internal/tools/executor_test.go:288`

План явно требовал: *"Integration tests: pass `ProjectPath: tmpDir` (create temp dir with Go files)"*

Реализован только один sub-test — проверка ошибки на пустом пути. Нет положительного теста, который:
- создаёт `tmpDir` с `.go` файлами
- вызывает `ArchitectureReview` успешно
- проверяет что вернулся ненулевой отчёт

Аналогично отсутствует `TestModuleAudit` с проверкой пустого `project_path`.

---

### 2. `parseLLMResponse` + `buildReportFromLLM` — мёртвая обёртка

**Файл:** `internal/tools/executor.go:320, 356`

```go
func (te *ToolExecutor) buildReportFromLLM(response string) *domain.AuditReport {
    return parseLLMResponse(response) // тривиальная обёртка, ничего не добавляет
}
```

`ArchitectureReview` вызывает `te.buildReportFromLLM()`, а `ArchitectureComplianceCheck` и `ModuleAudit` вызывают `parseLLMResponse()` напрямую. Рефакторинг не завершён — нужно выбрать один способ и применить его везде.

---

### 3. Обработка ошибок LLM несогласованна между инструментами

**Файл:** `internal/tools/executor.go`

`ArchitectureReview` при ошибке LLM возвращает `fmt.Errorf("LLM call failed: %w", err)`.

`ArchitectureComplianceCheck` и `ModuleAudit` молча игнорируют ошибку LLM:
```go
llmResponse, err := llmProvider.Complete(ctx, prompt, language)
if err == nil {
    // обрабатывать только при успехе
}
// при ошибке — возвращается только статический отчёт без предупреждения
```

Три инструмента — разное поведение при одной и той же ситуации. Клиент не знает, что LLM не отработал.

---

### 4. `persistReport` молча игнорирует ошибки записи файлов

**Файл:** `internal/mcp/server.go:472, 488`

```go
if err := os.WriteFile(...); err != nil {
    // пусто — ошибка теряется
} else {
    s.logger.Printf("[OK] md saved: ...")
}
```

Ошибка записи не логируется и не возвращается. Go-идиома — обрабатывать ошибку в `if`, а не в `else`. Должно быть:
```go
if err := os.WriteFile(...); err != nil {
    s.logger.Printf("[ERROR] failed to save md: %v", err)
} else {
    s.logger.Printf("[OK] md saved: ...")
}
```

---

### 5. `CLAUDE.md` — устаревшие флаги

**Файл:** `CLAUDE.md:23, 70`

```bash
# строка 23 — оба флага удалены из кода:
go run ./cmd -debug -project /path/to/project -debug-dir /tmp/audit

# строка 70 — упоминание -debug-dir:
When `-debug` flag is set, reports are written to `<project>/debug/` (or `-debug-dir`) ...
```

Кто прочитает документацию — получит `flag provided but not defined`.

---

### 6. `ArchitectureReview` читает `debug`/`logger` из мьютекса дважды

**Файл:** `internal/tools/executor.go:247–249, 279–282`

Первое чтение на строке 247 (для логирования snapshot), второе на строке 279 (для логирования LLM-запроса). В `ArchitectureComplianceCheck` и `ModuleAudit` — одно чтение внутри `if llmProvider != nil`. Несогласованность без причины.

---

## Итог

| # | Проблема | Severity |
|---|---|---|
| 1 | Отсутствуют интеграционные тесты для `ArchitectureReview` и `ModuleAudit` | High |
| 2 | Мёртвая обёртка `buildReportFromLLM` — несогласованные вызовы | Medium |
| 3 | LLM-ошибки молча игнорируются в 2 из 3 инструментов | Medium |
| 4 | `persistReport` не логирует ошибки записи файлов | Low |
| 5 | `CLAUDE.md` содержит удалённые флаги | Low |
| 6 | Двойное чтение мьютекса в `ArchitectureReview` | Low |
