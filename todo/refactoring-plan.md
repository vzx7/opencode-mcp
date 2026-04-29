# MCP Server Refactoring Plan

## Goal
Make the MCP server universal and independent from any specific project. The project path (`project_path`) must be passed **only** through tool arguments. Prompt saving occurs **only** in `-debug` mode.

---

## Requirements
1. Remove `defaultPath` from the entire project
2. `project_path` must be passed **only** via tool arguments (not from server config)
3. If `project_path` is not provided in tool call → return error immediately and do nothing
4. Debug directory path is always: `{project_path}/debug` (hardcoded)
5. Prompts are saved **only** when server is started with `-debug` flag (before sending to LLM)
6. Remove `-debug-dir` flag and all related logic
7. All error messages and logs must be in **English**

---

## Changes by File

### 1. `cmd/main.go`
**Remove:**
- Flag `-project` (line 25)
- Flag `-debug-dir` (line 26)
- Variables `project`, `debugDir`
- Logic setting `cfg.Project` and `mcpCfg.ProjectPath`/`mcpCfg.DebugDir`

**Change:**
- `mcpCfg` initialization: remove `ProjectPath` and `DebugDir`
- Remove `cfg.Project` from config validation
- Remove log line `"Project: %s\n", cfg.Project`

---

### 2. `internal/config/config.go`
**Remove:**
- Field `Project string` from `Config` struct (line 18)
- `Project: os.Getenv("PROJECT")` from `Load()` function (line 52)

---

### 3. `internal/mcp/server.go`

#### Structures:
- `Server`: remove `defaultPath` and `debugDir` fields
- `Config`: remove `ProjectPath` and `DebugDir` fields

#### Functions:
- `NewServer`:
  - Don't use `cfg.ProjectPath` and `cfg.DebugDir`
  - Don't set `defaultPath`/`debugDir`
  - Remove debug directory creation logic

- `persistReport`: change signature to accept `debugDir` as argument:
  ```go
  func (s *Server) persistReport(toolName string, report *domain.AuditReport, projectPath, modulePath, debugDir string) string
  ```
  Use `debugDir` parameter instead of `s.debugDir`. If `debugDir` is empty — don't save.

- Remove `resolveDebugDir` method

- `handleToolsCall`:
  - Don't set `input.ProjectPath = s.defaultPath`
  - When calling `persistReport`, compute `debugDir` if debug is enabled:
    ```go
    var debugDir string
    if s.debug && input.ProjectPath != "" {
        debugDir = filepath.Join(input.ProjectPath, "debug")
    }
    ```
  - Pass `debugDir` to `persistReport`

---

### 4. `internal/tools/executor.go`

#### Structures:
- `ToolExecutor`: remove `defaultPath` and `debugDir` fields
- `ToolExecutorConfig`: remove `DefaultPath` and `DebugDir` fields

#### Functions:
- `NewToolExecutor`: don't set `defaultPath`/`debugDir`

- `savePrompt`: make it a standalone function with `debugDir` argument:
  ```go
  func savePrompt(toolName, prompt, language, debugDir string)
  ```
  Save prompt only if `debugDir` is not empty.

- **3 tools** (`ArchitectureReview`, `ArchitectureComplianceCheck`, `ModuleAudit`):
  - Remove fallback to `te.defaultPath` and `"."`
  - If `input.ProjectPath == ""` → return error `"project_path is required"` immediately
  - Set `projectPath := input.ProjectPath`
  - Before sending to LLM, if `te.debug == true`:
    ```go
    debugDir := filepath.Join(projectPath, "debug")
    savePrompt("tool_name", prompt, language, debugDir)
    ```

---

### 5. Tests

#### `internal/tools/executor_test.go`:
- Remove tests with `DefaultPath`
- Update `TestArchitectureReview`: check error is returned for empty `project_path`
- Integration tests: pass `ProjectPath: tmpDir` (create temp dir with Go files)

#### `internal/mcp/server_test.go`:
- Remove tests with `ProjectPath` and `DebugDir`
- Update for new structures

---

### 6. Error Messages and Logs
- All error messages in **English**:
  - `"project_path is required"` (instead of Russian)
- All log messages in **English**

---

## Verification
1. `go build ./...` — successful compilation
2. `go test ./...` — all tests pass
3. Manual check:
   - Without `-debug`: prompts are **not** saved
   - With `-debug`: prompts saved to `{project_path}/debug/input/`
   - Tool without `project_path`: error `"project_path is required"`
   - Server is universal — can work with any project by passing `project_path` in tool call
