# Task: Deep Architecture Audit — Implementation

## Context

This is a Go MCP server (`github.com/vzx7/opencode-mcp`) that exposes three tools to AI clients
(Claude Desktop, Cursor, etc.): `architecture_review`, `architecture_compliance_check`, `module_audit`.

The current `architecture_review` tool is too shallow: it only inspects directory names and file
counts, then sends that skeleton to an LLM. The LLM never sees actual source code and there is no
real dependency graph. The result is a "folder linter", not an architecture audit.

**Goal:** make `architecture_review` (and supporting infrastructure) perform a genuinely deep
architectural analysis of a Go project.

---

## Repository layout (relevant parts)

```
internal/
  analyzer/engine.go      — static analysis: BuildProjectMap, CheckCompliance, AuditModule
  domain/models.go        — shared structs: ProjectMap, AuditReport, Issue, Severity, …
  llm/provider_impl.go    — LLM providers (OpenAI, Anthropic, Mock) + prompt builders
  llm/types.go            — LLM types, ToolMarkers, prompt i18n
  tools/executor.go       — ArchitectureReview, ArchitectureComplianceCheck, ModuleAudit
  mcp/server.go           — JSON-RPC routing, report formatting, file persistence
cmd/main.go               — CLI entry point
```

---

## Required changes

### 1. Import graph via `go/ast` — `internal/analyzer/engine.go`

Add a method `BuildImportGraph(pm *domain.ProjectMap) (*domain.ImportGraph, error)` that:

- Walks every `.go` file in the project (excluding `_test.go` and `vendor/`)
- Parses each file with `go/parser` (mode `ImportsOnly`)
- Extracts all `import` paths
- Builds a directed graph: `map[string][]string` — package path → list of imported package paths
- Keeps only intra-project imports (filter by the module name from `go.mod`, e.g. `github.com/vzx7/opencode-mcp`)
- Detects **cyclic imports** (DFS with visited set) and records them
- Detects **layer violations**: a lower layer importing a higher layer (e.g. `domain` → `tools`,
  `domain` → `mcp`). Layer order (low → high): `domain` → `analyzer` → `llm` → `tools` → `mcp`
- Returns `*domain.ImportGraph` (add this struct to `domain/models.go`)

`domain.ImportGraph` should contain:
```go
type ImportGraph struct {
    Edges          map[string][]string // package → imports (intra-project only)
    Cycles         [][]string          // each element is a cycle path
    LayerViolations []LayerViolation
}

type LayerViolation struct {
    From    string // importing package
    To      string // imported package
    Message string
}
```

### 2. Per-file metrics — `internal/analyzer/engine.go`

Add `CollectFileMetrics(pm *domain.ProjectMap) ([]domain.FileMetric, error)` that walks `.go`
source files and for each records:

```go
type FileMetric struct {
    Path         string
    Lines         int
    ExportedFuncs int // count of func declarations starting with uppercase
    ExportedTypes int // count of type declarations starting with uppercase
    HasTests      bool // true if a corresponding _test.go exists
}
```

Large files (> 500 lines) and packages with no tests should surface as issues in the report.

### 3. Code snapshot for LLM — `internal/tools/executor.go`

In `ArchitectureReview`, before calling `BuildReviewPrompt`, build a **code snapshot**:

- Collect all `.go` source files (exclude `_test.go`, `vendor/`)
- Sort by file size descending
- Include files until the cumulative character count reaches **40 000 chars** (a safe LLM context
  slice); skip larger files with a note
- Pass the snapshot as `map[string]string` (relative path → content) into `BuildReviewPrompt`

### 4. Enrich prompt — `internal/llm/provider_impl.go`

Update `BuildReviewPrompt(pm *domain.ProjectMap, snapshot map[string]string, graph *domain.ImportGraph, metrics []domain.FileMetric, language string) string` to include:

- **Source code section** — each file as a fenced Go code block (relative path as heading);
  if the snapshot was truncated, add a note "N files omitted due to size limit"
- **Import graph section** — list all intra-project edges; highlight cycles and layer violations
  (mark them `[CYCLE]` / `[LAYER VIOLATION]`)
- **Metrics section** — table: file | lines | exported funcs | exported types | has tests;
  flag files > 500 lines

The LLM instructions should explicitly ask to:
1. Identify coupling hotspots based on the import graph
2. Comment on the size and responsibility of large files
3. Assess whether the public API surface (exported symbols) is appropriate per layer

### 5. Deterministic issue injection — `internal/tools/executor.go`

After `enrichWithLocalAnalysis`, add `enrichWithGraphAnalysis(report, graph, metrics)` that
programmatically appends `domain.Issue` entries for:

- Each detected import cycle → `SeverityCritical`
- Each layer violation → `SeverityHigh`
- Each file > 500 lines → `SeverityMedium`
- Each package with Go files but no test file → `SeverityLow` (one issue per package, not per file)

These issues must be added regardless of whether the LLM call succeeded.

### 6. Wire everything together — `internal/tools/executor.go:ArchitectureReview`

New execution order:
```
1. BuildProjectMap(projectPath)
2. BuildImportGraph(pm)
3. CollectFileMetrics(pm)
4. readCodeSnapshot(projectPath, maxChars=40000)   // new helper
5. getLLM + getLanguage
6. BuildReviewPrompt(pm, snapshot, graph, metrics, language)
7. llmProvider.Complete(ctx, prompt, language)
8. buildReportFromLLM(llmResponse)
9. enrichWithLocalAnalysis(report, pm)
10. enrichWithGraphAnalysis(report, graph, metrics)  // new
11. return report
```

---

## Constraints

- **No new external dependencies.** Use only stdlib: `go/ast`, `go/parser`, `go/token`.
- Keep the existing `domain.ProjectMap`, `domain.AuditReport`, `domain.Issue` structs intact;
  only add new structs (`ImportGraph`, `LayerViolation`, `FileMetric`).
- `BuildReviewPrompt` signature changes — update all call sites (currently only one:
  `executor.go:ArchitectureReview`).
- All new methods on `*Analyzer` must have unit tests in `internal/analyzer/engine_test.go`.
  At minimum: import graph on the repo itself, cycle detection with a synthetic fixture,
  layer violation detection with a synthetic fixture, file metrics on a known file.
- Do not break `architecture_compliance_check` or `module_audit` — they do not use
  `BuildReviewPrompt` and should remain untouched except for any domain struct additions.
- Existing tests must continue to pass: `go test ./...`.

---

## Definition of done

- `go build ./...` succeeds
- `go test ./...` passes (including new tests)
- Calling `architecture_review` on this repository returns a report that includes:
  - at least the import graph edges section in the LLM prompt (verify with `-debug` flag)
  - deterministic issues from graph/metrics analysis in the final report
  - code snapshot visible in the prompt log
