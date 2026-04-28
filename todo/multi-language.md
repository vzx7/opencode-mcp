# Task: Multi-Language Support — Implementation

## Context

The MCP server currently performs analysis only for Go projects. All language-specific logic
(file extensions, test detection, import graph via go/ast, symbol counting, go.mod parsing)
is hardcoded in `internal/analyzer/engine.go` and `internal/tools/executor.go`.

**Goal:** refactor the analyzer layer to support multiple programming languages through a
`ProjectAnalyzer` interface. Go becomes the first (and currently only complete) implementation.
Python and TypeScript stubs are added so new contributors know where to add implementations.

**Principle:** only the analyzer package changes significantly. The rest of the system
(executor, mcp, llm, domain) gets minimal targeted changes.

---

## New file structure

```
internal/analyzer/
  language.go          ← NEW: ProjectAnalyzer interface + LanguageInfo helper
  registry.go          ← NEW: registered analyzers, Detect(), ByName()
  engine.go            ← MODIFIED: language-agnostic Engine, delegates to ProjectAnalyzer
  golang/
    analyzer.go        ← NEW: Go implementation (code extracted from engine.go)
  python/
    analyzer.go        ← NEW: Python stub
  typescript/
    analyzer.go        ← NEW: TypeScript stub
```

---

## Step 1 — Define the interface: `internal/analyzer/language.go`

Create this file:

```go
package analyzer

import "github.com/vzx7/opencode-mcp/internal/domain"

// ProjectAnalyzer encapsulates all language-specific analysis operations.
// Implement this interface to add support for a new programming language.
type ProjectAnalyzer interface {
    // Name returns the language identifier (e.g., "go", "python", "typescript").
    Name() string

    // Detect returns true if this analyzer can handle the project at rootPath.
    // Looks for language-specific marker files (go.mod, pyproject.toml, tsconfig.json).
    Detect(rootPath string) bool

    // SourceExtensions returns file extensions for source files (e.g., []string{".go"}).
    SourceExtensions() []string

    // IsTestFile returns true if the given file path is a test file to be excluded
    // from snapshots and metrics (e.g., _test.go, test_*.py, *.test.ts).
    IsTestFile(path string) bool

    // BuildImportGraph constructs the intra-project import/dependency graph.
    // Only intra-project dependencies should be included (not stdlib or third-party).
    BuildImportGraph(rootPath string) (*domain.ImportGraph, error)

    // CountSymbols counts publicly exported functions and types in a source file.
    // Returns (0, 0) if the language has no concept of exported symbols.
    CountSymbols(src []byte) (functions, types int)

    // SnippetLang returns the language tag for markdown code blocks (e.g., "go", "python").
    SnippetLang() string

    // ModuleName extracts the project module/package name from the project manifest
    // (go.mod → module name, package.json → name field, etc.).
    // Returns empty string if not applicable.
    ModuleName(rootPath string) string
}
```

---

## Step 2 — Registry: `internal/analyzer/registry.go`

```go
package analyzer

import (
    "github.com/vzx7/opencode-mcp/internal/analyzer/golang"
    "github.com/vzx7/opencode-mcp/internal/analyzer/python"
    "github.com/vzx7/opencode-mcp/internal/analyzer/typescript"
)

// registered is the ordered list of available language analyzers.
// Detection runs in order; the first match wins.
var registered = []ProjectAnalyzer{
    &golang.Analyzer{},
    &typescript.Analyzer{},
    &python.Analyzer{},
}

// Detect returns the first analyzer whose Detect() returns true for rootPath.
// Falls back to Go analyzer if nothing matches.
func Detect(rootPath string) ProjectAnalyzer {
    for _, a := range registered {
        if a.Detect(rootPath) {
            return a
        }
    }
    return &golang.Analyzer{}
}

// ByName returns the analyzer registered under the given name.
// ok is false if no analyzer with that name is registered.
func ByName(name string) (ProjectAnalyzer, bool) {
    for _, a := range registered {
        if a.Name() == name {
            return a, true
        }
    }
    return nil, false
}
```

---

## Step 3 — Go implementation: `internal/analyzer/golang/analyzer.go`

Create package `golang` (note: this is a sub-package, not the keyword).
Move all Go-specific code from `engine.go` into this file:

```go
package golang

import (
    "bufio"
    "go/ast"
    "go/parser"
    "go/token"
    "os"
    "path/filepath"
    "strings"

    "github.com/vzx7/opencode-mcp/internal/domain"
)

type Analyzer struct{}

func (a *Analyzer) Name() string { return "go" }

func (a *Analyzer) Detect(rootPath string) bool {
    _, err := os.Stat(filepath.Join(rootPath, "go.mod"))
    return err == nil
}

func (a *Analyzer) SourceExtensions() []string { return []string{".go"} }

func (a *Analyzer) IsTestFile(path string) bool {
    return strings.HasSuffix(filepath.Base(path), "_test.go")
}

func (a *Analyzer) SnippetLang() string { return "go" }

func (a *Analyzer) ModuleName(rootPath string) string {
    // Extract from go.mod: "module github.com/org/repo" → "github.com/org/repo"
    data, err := os.ReadFile(filepath.Join(rootPath, "go.mod"))
    if err != nil {
        return ""
    }
    for _, line := range strings.Split(string(data), "\n") {
        line = strings.TrimSpace(line)
        if strings.HasPrefix(line, "module ") {
            parts := strings.Fields(line)
            if len(parts) >= 2 {
                return parts[1]
            }
        }
    }
    return ""
}

func (a *Analyzer) BuildImportGraph(rootPath string) (*domain.ImportGraph, error) {
    // Move the ENTIRE current BuildImportGraph body from engine.go here.
    // Replace a.getModulePrefix() with a.ModuleName(rootPath).
    // Replace a.rootPath with rootPath.
    // The findCycles and findLayerViolations helpers stay in engine.go (language-agnostic).
    // This method calls those helpers after building edges.
    //
    // Keep the logic: parse each .go file with parser.ImportsOnly,
    // filter imports by modulePrefix, build edges map[pkgPath][]string.
    panic("implement: move from engine.go BuildImportGraph body")
}

func (a *Analyzer) CountSymbols(src []byte) (functions, types int) {
    // Move countExportedSymbols from engine.go here verbatim.
    // Rename to CountSymbols to match interface.
    fset := token.NewFileSet()
    f, err := parser.ParseFile(fset, "", src, parser.ParseComments)
    if err != nil {
        return 0, 0
    }
    for _, decl := range f.Decls {
        switch d := decl.(type) {
        case *ast.FuncDecl:
            if d.Name != nil && d.Name.IsExported() {
                functions++
            }
        case *ast.GenDecl:
            for _, spec := range d.Specs {
                if s, ok := spec.(*ast.TypeSpec); ok && s.Name != nil && s.Name.IsExported() {
                    types++
                }
            }
        }
    }
    return
}

// hasTestFile is an internal helper — not part of the interface.
func hasTestFile(path string) bool {
    dir := filepath.Dir(path)
    base := filepath.Base(path)
    ext := filepath.Ext(base)
    name := base[:len(base)-len(ext)]
    _, err := os.Stat(filepath.Join(dir, name+"_test.go"))
    return err == nil
}
```

**Important:** `hasTestFile` (checking sibling test file) remains Go-specific internal logic.
It is called from `Engine.CollectFileMetrics` via `lang.IsTestFile`? No — `HasTests` in `FileMetric`
is "does a test file exist for this source file", which is Go-specific.

To handle `HasTests` generically, add a method to the interface:

```go
// HasTestCounterpart returns true if a test file exists alongside the given source file.
// For Go: checks for a_test.go next to a.go.
// For languages without per-file test conventions: returns false.
HasTestCounterpart(sourcePath string) bool
```

Add `HasTestCounterpart` to the `ProjectAnalyzer` interface (Step 1) and implement it in the
Go analyzer as the current `hasTestFile` logic.

---

## Step 4 — Python stub: `internal/analyzer/python/analyzer.go`

```go
package python

import (
    "os"
    "path/filepath"
    "strings"

    "github.com/vzx7/opencode-mcp/internal/domain"
)

type Analyzer struct{}

func (a *Analyzer) Name() string { return "python" }

func (a *Analyzer) Detect(rootPath string) bool {
    for _, marker := range []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt"} {
        if _, err := os.Stat(filepath.Join(rootPath, marker)); err == nil {
            return true
        }
    }
    return false
}

func (a *Analyzer) SourceExtensions() []string { return []string{".py"} }

func (a *Analyzer) IsTestFile(path string) bool {
    base := filepath.Base(path)
    return strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")
}

func (a *Analyzer) HasTestCounterpart(sourcePath string) bool { return false }

func (a *Analyzer) BuildImportGraph(rootPath string) (*domain.ImportGraph, error) {
    // TODO: parse Python imports (import X, from X import Y)
    // Filter to intra-project modules only.
    return &domain.ImportGraph{
        Edges:           make(map[string][]string),
        Cycles:          [][]string{},
        LayerViolations: []domain.LayerViolation{},
    }, nil
}

func (a *Analyzer) CountSymbols(src []byte) (int, int) {
    // TODO: count top-level `def` (functions) and `class` (types)
    return 0, 0
}

func (a *Analyzer) SnippetLang() string { return "python" }

func (a *Analyzer) ModuleName(rootPath string) string {
    // TODO: parse [project].name from pyproject.toml or name= from setup.py
    return ""
}
```

---

## Step 5 — TypeScript stub: `internal/analyzer/typescript/analyzer.go`

```go
package typescript

import (
    "os"
    "path/filepath"
    "strings"

    "github.com/vzx7/opencode-mcp/internal/domain"
)

type Analyzer struct{}

func (a *Analyzer) Name() string { return "typescript" }

func (a *Analyzer) Detect(rootPath string) bool {
    for _, marker := range []string{"tsconfig.json", "tsconfig.base.json"} {
        if _, err := os.Stat(filepath.Join(rootPath, marker)); err == nil {
            return true
        }
    }
    return false
}

func (a *Analyzer) SourceExtensions() []string { return []string{".ts", ".tsx"} }

func (a *Analyzer) IsTestFile(path string) bool {
    base := filepath.Base(path)
    return strings.HasSuffix(base, ".test.ts") ||
        strings.HasSuffix(base, ".spec.ts") ||
        strings.HasSuffix(base, ".test.tsx") ||
        strings.HasSuffix(base, ".spec.tsx")
}

func (a *Analyzer) HasTestCounterpart(sourcePath string) bool { return false }

func (a *Analyzer) BuildImportGraph(rootPath string) (*domain.ImportGraph, error) {
    // TODO: parse ES module imports (import X from 'Y', require('Y'))
    // Filter to intra-project paths only (relative paths or tsconfig paths aliases).
    return &domain.ImportGraph{
        Edges:           make(map[string][]string),
        Cycles:          [][]string{},
        LayerViolations: []domain.LayerViolation{},
    }, nil
}

func (a *Analyzer) CountSymbols(src []byte) (int, int) {
    // TODO: count exported functions and interfaces/types
    return 0, 0
}

func (a *Analyzer) SnippetLang() string { return "typescript" }

func (a *Analyzer) ModuleName(rootPath string) string {
    // TODO: read "name" from package.json
    return ""
}
```

---

## Step 6 — Refactor `internal/analyzer/engine.go`

### Rename and embed language analyzer

```go
// Engine replaces Analyzer. It is the language-agnostic orchestrator.
type Engine struct {
    rootPath string
    lang     ProjectAnalyzer
}

// New auto-detects the project language.
func New(rootPath string) *Engine {
    return &Engine{rootPath: rootPath, lang: Detect(rootPath)}
}

// NewWithLang uses the named language analyzer. Falls back to auto-detection if unknown.
func NewWithLang(rootPath, langName string) *Engine {
    lang, ok := ByName(langName)
    if !ok {
        lang = Detect(rootPath)
    }
    return &Engine{rootPath: rootPath, lang: lang}
}

// SnippetLang returns the markdown code block language tag for this project.
func (e *Engine) SnippetLang() string { return e.lang.SnippetLang() }

// LangName returns the detected/selected language name.
func (e *Engine) LangName() string { return e.lang.Name() }

// isSourceFile returns true if path is a non-test source file for this language.
func (e *Engine) isSourceFile(path string) bool {
    base := filepath.Base(path)
    ext := filepath.Ext(base)
    for _, se := range e.lang.SourceExtensions() {
        if ext == se && !e.lang.IsTestFile(path) {
            return true
        }
    }
    return false
}
```

### Changes inside engine.go methods

**`BuildProjectMap`:**
- Replace `strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")`
  with `e.isSourceFile(path)` in `scanGoFiles` (rename to `scanSourceFiles`)
- Replace `info.Name() == "internal" || info.Name() == "pkg" || info.Name() == "cmd"` stays as-is
  (these directory names are architecture conventions, not Go-specific — they apply to many languages)
- Remove `GoFiles int` field from `Module`? No — rename to `SourceFiles int` in domain.
  Actually keep `GoFiles` for now to avoid domain changes; it's just a counter name.
  Rename it to `SourceFiles` in `domain.Module` as part of this task.

**`analyzeGoMod`:** Delete from engine.go. The logic moves to `golang/analyzer.go` as `ModuleName()`.
In `BuildProjectMap`, call `e.lang.ModuleName(e.rootPath)` if you need the module name.

**`BuildImportGraph`:** Delegate entirely:
```go
func (e *Engine) BuildImportGraph(pm *domain.ProjectMap) (*domain.ImportGraph, error) {
    graph, err := e.lang.BuildImportGraph(e.rootPath)
    if err != nil {
        return nil, err
    }
    // findCycles and findLayerViolations stay in engine.go as package-level functions
    // they are language-agnostic (operate on map[string][]string)
    graph.Cycles = findCycles(graph.Edges)
    graph.LayerViolations = findLayerViolations(graph.Edges)
    return graph, nil
}
```

**`CollectFileMetrics`:**
- Replace `.go`/`_test.go` checks with `e.isSourceFile(path)`
- Replace `countExportedSymbols(src)` with `e.lang.CountSymbols(src)`
- Replace `hasTestFile(path)` with `e.lang.HasTestCounterpart(path)`

**`CollectGitHotspots`:**
- Replace:
  ```go
  if line != "" && strings.HasSuffix(line, ".go") && !strings.HasSuffix(line, "_test.go") {
  ```
  with:
  ```go
  if line != "" && e.isSourceFile(line) {
  ```

**`AuditModule`:**
- Replace `.go` / `_test.go` checks with `e.isSourceFile()`

**Remove from engine.go:**
- All `go/ast`, `go/parser`, `go/token` imports
- `scanGoFiles` → becomes `scanSourceFiles` using `e.isSourceFile()`
- `analyzeGoMod`
- `getModulePrefix`
- `countExportedSymbols`
- `hasTestFile`

**Keep in engine.go (language-agnostic):**
- `findCycles`
- `indexOf`
- `computePackageLevels`
- `findLayerViolations`
- `detectLayers`
- `defaultLayerPatterns`
- `CheckCompliance`

---

## Step 7 — Update `internal/tools/executor.go`

### ToolInput: add ProgrammingLanguage

```go
type ToolInput struct {
    ProjectPath        string `json:"project_path"`
    ModulePath         string `json:"module_path,omitempty"`
    Provider           string `json:"provider"`
    LLM                string `json:"llm"`
    Language           string `json:"language,omitempty"`           // prompt language
    ProgrammingLanguage string `json:"programming_language,omitempty"` // source code language
}
```

### Engine construction

Replace every `analyzer.New(projectPath)` with:
```go
analyzerEngine := analyzer.NewWithLang(projectPath, input.ProgrammingLanguage)
```

(`NewWithLang` falls back to auto-detect when `ProgrammingLanguage` is empty.)

### `buildCodeSnapshot`

Change signature:
```go
func (te *ToolExecutor) buildCodeSnapshot(
    projectPath string,
    includePaths []string,
    maxChars int,
    engine *analyzer.Engine,  // ← add this
) (map[string]string, []string, []string)
```

Replace inside:
```go
// before
strings.HasSuffix(info.Name(), ".go") && !strings.HasSuffix(info.Name(), "_test.go")

// after
engine.isSourceFile(path)
```

Note: `engine.isSourceFile` is currently unexported. Either export it as `IsSourceFile`,
or add a helper method. Export it: `func (e *Engine) IsSourceFile(path string) bool`.

Update all callers of `buildCodeSnapshot` to pass the engine.

### `readModuleContent`

Same change: pass the engine and use `engine.IsSourceFile(path)`.

### `BuildReviewPrompt` call

```go
prompt := llm.BuildReviewPrompt(
    pm, snapshot, snapshotOrder, omitted, graph, metrics,
    hotspots,
    analyzerEngine.SnippetLang(),  // ← add this
    language,
)
```

### `BuildModuleAuditPrompt` call

```go
prompt := llm.BuildModuleAuditPrompt(modulePath, moduleContent, analyzerEngine.SnippetLang(), language)
```

---

## Step 8 — Update `internal/llm/provider_impl.go`

### `BuildReviewPrompt` signature

```go
func BuildReviewPrompt(
    projectMap *ProjectMap,
    snapshot map[string]string,
    snapshotOrder []string,
    omitted []string,
    graph *ImportGraph,
    metrics []FileMetric,
    hotspots []GitHotspot,
    snippetLang string,   // ← add before language
    language string,
) string
```

Inside the function, replace all `"\n` + backtick + `go\n"` with:
```go
"\n```" + snippetLang + "\n"
```
There are 2 occurrences (snapshot section and module audit section if applicable).

### `BuildModuleAuditPrompt` signature

```go
func BuildModuleAuditPrompt(modulePath string, files map[string]string, snippetLang string, language string) string
```

Replace the hardcoded ` ```go` similarly.

---

## Step 9 — Update `internal/mcp/server.go`

### Tool schema: add programming_language to all 3 tools

In `handleToolsList`, add to each tool's `Properties`:
```go
"programming_language": {
    Type:        "string",
    Description: "Programming language of the project (go, python, typescript). Auto-detected if omitted.",
},
```

### Argument parsing: add to each tool's case

```go
if pl, ok := arguments["programming_language"].(string); ok {
    input.ProgrammingLanguage = pl
}
```

---

## Step 10 — Update domain model

In `internal/domain/models.go`, rename `GoFiles int` to `SourceFiles int` in the `Module` struct.
Update all references in `engine.go`.

---

## Step 11 — Update tests

### `internal/llm/provider_test.go`

All `BuildReviewPrompt` calls: add `"go"` as `snippetLang` parameter before `"en"` / `"ru"`.
All `BuildModuleAuditPrompt` calls: add `"go"` as `snippetLang`.

### `internal/tools/executor_test.go`

`TestBuildCodeSnapshot`: update calls to `buildCodeSnapshot` — pass a minimal engine.
Create a helper in the test to build a Go engine for a temp dir:
```go
eng := analyzer.New(tmpDir)
snapshot, order, omitted := te.buildCodeSnapshot(tmpDir, nil, 5000, eng)
```

### `internal/tools/integration_test.go`

Same — pass engine to `buildCodeSnapshot` calls if directly tested.

---

## Verification

```bash
go build ./...
go test ./...
```

Both must pass. No new linter errors.

Manually verify auto-detection:
- Project with `go.mod` → engine.LangName() == "go"
- Project with `tsconfig.json` → engine.LangName() == "typescript"
- Project with `pyproject.toml` → engine.LangName() == "python"
- Unknown project → engine.LangName() == "go" (fallback)

---

## Constraints

- Do not change the public API of `ToolExecutor` methods (callers in `server.go` must work as-is)
- Do not change `domain/models.go` beyond renaming `GoFiles` → `SourceFiles`
- Do not change JSON prompt files
- Python and TypeScript implementations are stubs — they compile and return empty/zero results;
  `BuildImportGraph` returns an empty graph, `CountSymbols` returns (0, 0)
- No new external dependencies
