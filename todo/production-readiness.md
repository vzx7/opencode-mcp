# Task: Production Readiness — Implementation

## Context

This is a Go MCP server (`github.com/vzx7/opencode-mcp`) that exposes three tools to AI clients
(Claude Desktop, Cursor, etc.): `architecture_review`, `architecture_compliance_check`, `module_audit`.

The core functionality is complete and working. The following issues block production use:
critical bugs, security gaps, dead code, and missing integration tests.

**Do not refactor or add features beyond what is listed below.**

---

## Repository layout (relevant parts)

```
internal/
  mcp/server.go          — HTTP + stdio transport, JSON-RPC 2.0 routing
  tools/executor.go      — tool dispatch, LLM calls, report persistence
  tools/executor_test.go — unit tests
  analyzer/engine.go     — static project analysis
  llm/provider_impl.go   — OpenAI / Anthropic / Mock provider
  llm/provider.go        — MockProvider, NewProvider, LLM interface
  llm/provider_test.go   — unit tests
  domain/models.go       — shared data structs
cmd/main.go              — CLI flags, config loading, server startup
```

---

## Changes to implement

### 1. Fix context propagation in HTTP handler — `internal/mcp/server.go`

**Problem:** `HandleJSONRPC` creates `ctx := context.Background()` at line ~149 and passes it to
all tool calls. When the HTTP client disconnects, the LLM call continues running for up to 10
minutes (DefaultTimeout), wasting resources and blocking goroutines.

**Fix:** Replace `ctx := context.Background()` with `ctx := r.Context()` in `HandleJSONRPC`.

The stdio path in `StartStdio` can keep `context.Background()` since there is no request lifecycle
there.

---

### 2. Remove dead code `saveDebugResponse` — `internal/mcp/server.go`

**Problem:** `saveDebugResponse` (≈40 lines, around line 545) is defined but never called. It was
replaced by `persistReport`. It imports `math/rand` solely for `rand.Intn(9999)`.

**Fix:**
- Delete the entire `saveDebugResponse` method.
- Remove the `"math/rand"` import from `server.go` (verify nothing else uses it first).

---

### 3. Fix goroutine leak in stdio mode — `internal/mcp/server.go`

**Problem:** `StartStdio` spawns a new goroutine for every `decoder.Decode` call:

```go
go func() {
    reqCh <- decoder.Decode(&req)
}()
```

When `shutdownCh` fires before the read completes, the goroutine is abandoned — it is blocked
forever trying to send on `reqCh` (capacity 1, receiver gone).

**Fix:** Use a single persistent goroutine for reading instead of spawning one per iteration.
Move the decode loop into a dedicated goroutine started once before the select loop, and use a
`msgCh chan decodeResult` (where `decodeResult` holds `req JSONRPCRequest` and `err error`).

```go
type decodeResult struct {
    req JSONRPCRequest
    err error
}

msgCh := make(chan decodeResult, 1)
go func() {
    for {
        var req JSONRPCRequest
        err := decoder.Decode(&req)
        msgCh <- decodeResult{req, err}
        if err != nil {
            return
        }
    }
}()

for {
    select {
    case <-shutdownCh:
        return
    case res := <-msgCh:
        if res.err != nil { ... }
        // handle res.req
    }
}
```

---

### 4. Add path traversal validation — `internal/tools/executor.go`

**Problem:** `project_path` and `module_path` from the client are passed directly to
`filepath.Walk`, `os.ReadFile`, and `exec.Command("git", "-C", rootPath, ...)` without validation.
A client can pass `"../../etc"` or an absolute path outside the intended scope.

**Fix:** Add a `validatePath` helper at the top of `executor.go`:

```go
func validatePath(p string) error {
    if p == "" {
        return nil
    }
    cleaned := filepath.Clean(p)
    if strings.Contains(cleaned, "..") {
        return fmt.Errorf("path traversal not allowed: %q", p)
    }
    return nil
}
```

Call it at the start of `ArchitectureReview`, `ArchitectureComplianceCheck`, and `ModuleAudit`
for both `projectPath` and `modulePath` before any filesystem operation.

Note: absolute paths that resolve outside of the working directory are acceptable for now
(the server is a local tool), but `..` segments must be rejected.

---

### 5. Add concurrency limit for LLM calls — `internal/tools/executor.go`

**Problem:** There is no limit on simultaneous LLM calls. Ten concurrent `architecture_review`
requests = ten parallel HTTP requests to the LLM API, causing rate-limit errors (429) that the
retry logic then handles with delays, tying up goroutines.

**Fix:** Add a semaphore to `ToolExecutor` that caps concurrent LLM calls at 3:

```go
type ToolExecutor struct {
    // existing fields ...
    llmSem chan struct{} // semaphore
}
```

Initialize in `NewToolExecutor`:

```go
llmSem: make(chan struct{}, 3),
```

Add a `acquireLLM` / `releaseLLM` pair and wrap every `llmProvider.Complete(...)` call:

```go
func (te *ToolExecutor) acquireLLM(ctx context.Context) error {
    select {
    case te.llmSem <- struct{}{}:
        return nil
    case <-ctx.Done():
        return ctx.Err()
    }
}

func (te *ToolExecutor) releaseLLM() {
    <-te.llmSem
}
```

Apply to all three tool methods (`ArchitectureReview`, `ArchitectureComplianceCheck`,
`ModuleAudit`) immediately before the `llmProvider.Complete` call.

---

### 6. Add prompt size warning — `internal/tools/executor.go`

**Problem:** For very large projects the LLM prompt can exceed the model's context window.
The API returns an error that falls through to the 75-score fallback with no indication of why.

**Fix:** After building the prompt and before calling `llmProvider.Complete`, log a warning if the
prompt exceeds 200 000 characters (rough proxy for ~50k tokens):

```go
const warnPromptChars = 200_000

if len(prompt) > warnPromptChars && logger != nil {
    logger.Printf("[WARN] prompt size %d chars may exceed model context window", len(prompt))
}
```

Add this check in all three tool methods, right after the prompt is built.

---

### 7. Add integration tests — `internal/tools/executor_test.go`

**Problem:** All existing tests use mocks. No test exercises the full pipeline:
real project → analyzer → prompt builder → mock LLM → report. Regressions in prompt generation
or report parsing are invisible.

**Fix:** Add a `TestArchitectureReviewIntegration` test that:

1. Uses the current repository (`"../../.."` relative to the test file) as the project path,
   OR creates a minimal temporary Go project with a `go.mod`, two packages, and one import between
   them.
2. Uses `MockProvider` (no real LLM calls).
3. Calls `te.ArchitectureReview(ctx, input)` and asserts:
   - `report != nil`
   - `report.Score >= 0 && report.Score <= 100`
   - `report.Summary != ""`
   - No panic

Similarly add `TestArchitectureComplianceIntegration` and `TestModuleAuditIntegration` using
the same temporary project.

Place these tests in a new file `internal/tools/integration_test.go` (same package `tools`).

---

## Verification

After all changes:

```bash
go build ./...
go test ./...
```

Both must pass with zero errors. No new linter warnings (unused imports, declared but not used).

## Constraints

- Do not add new dependencies (no external packages).
- Do not change the public API of any existing function (callers in `server.go` and `executor.go`
  must keep working as-is).
- Do not alter prompt content, JSON files, or scoring logic.
- Keep each change focused — do not opportunistically refactor surrounding code.
