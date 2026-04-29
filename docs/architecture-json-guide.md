# Guide: Generating `.architecture.json` for MCP Architecture Audit

This document describes how an agent must generate `docs/arch/.architecture.json` so that the MCP server can run a correct static compliance check and build an accurate LLM prompt.

---

## File Location

Always place the file at:

```
<project_root>/docs/arch/.architecture.json
```

The server auto-discovers this path when the `docs` argument is not provided. If the file is absent, the server falls back to generic default rules that will not match the real project.

---

## JSON Schema

```json
{
  "description": "<one-line summary of the architecture>",
  "layers": [ ...LayerRule ],
  "forbidden_dependencies": [ ...DependencyRule ],
  "constraints": [ ...string ]
}
```

### LayerRule

```json
{
  "name": "<short identifier>",
  "description": "<what this layer does>",
  "patterns": ["<relative/path/from/root>"],
  "allow_imports_from": ["<layer-name>", ...]
}
```

| Field | Required | Purpose |
|---|---|---|
| `name` | yes | Identifier. Referenced by `allow_imports_from` and `forbidden_dependencies`. |
| `description` | no | Shown in the LLM prompt as context for the reviewer. |
| `patterns` | yes | Filesystem paths **relative to project root**. Used for (1) existence check via `os.Stat` and (2) prefix-matching of import graph edges. |
| `allow_imports_from` | yes | List of layer **names** this layer is allowed to import. Empty array = no internal imports allowed. |

### DependencyRule

```json
{
  "from": "<layer-name>",
  "to": "<layer-name>",
  "reason": "<why this dependency is forbidden>"
}
```

### constraints

A plain string array. Each entry is a free-text architectural invariant surfaced verbatim in the LLM prompt.

---

## How the Server Uses Each Field

Understanding this prevents subtle mistakes.

### Static checker (`CheckCompliance`)

1. **Layer existence**: for each layer, `os.Stat(project_root + "/" + patterns[i])` must succeed. If the directory does not exist, the layer is reported as missing (MEDIUM severity, −10 points).

2. **Import graph matching**: the import graph contains edges like `internal/tools → internal/analyzer`. Each package path is resolved to a layer by checking which layer's `patterns` entry is a prefix of that path. Examples:
   - `internal/domain` → layer `domain` (pattern `internal/domain`)
   - `internal/analyzer/golang` → layer `analyzer` (pattern `internal/analyzer`)
   - `cmd` → layer `cmd` (pattern `cmd`)

3. **Forbidden dependency check** (CRITICAL, −20 points): if edge `A → B` exists in the import graph and `{from: A_layer, to: B_layer}` is in `forbidden_dependencies`.

4. **allow_imports_from check** (HIGH, −15 points): if edge `A → B` exists and `B_layer` is NOT in `A_layer.allow_imports_from`.

### LLM prompt

The full content of the file is written into the prompt:
- `description` — first line of the rules section
- Each layer with name, description, patterns, allow_imports_from
- All `forbidden_dependencies` with reasons
- All `constraints`

The LLM uses this as the authoritative specification to evaluate compliance.

---

## How to Derive Layers

**Step 1 — identify actual source directories.**

Walk the project tree. Each directory that contains source files and represents a distinct responsibility is a candidate layer. For Go: look for directories under `internal/`, `pkg/`, and the `cmd/` entrypoint.

**Step 2 — map each directory to a layer name.**

The name should reflect the responsibility, not the path. Examples:

| Directory | Layer name |
|---|---|
| `internal/domain` | `domain` |
| `internal/service` | `service` |
| `internal/repository` | `repository` |
| `internal/transport/http` | `http` |
| `cmd` | `cmd` |

**Step 3 — set `patterns` to the relative directory path.**

- `patterns` must be a **relative path from the project root**, using forward slashes.
- A single directory per layer is the common case. Multiple patterns are valid when one layer spans several directories (e.g. `["internal/api/v1", "internal/api/v2"]`).
- The pattern acts as a **prefix**: `internal/analyzer` will match both `internal/analyzer` and `internal/analyzer/golang`.

---

## How to Derive `allow_imports_from`

**Read the actual imports**, not intuition.

For each layer, find all `import` statements in its source files. Filter to internal packages only (same module prefix). Map each imported package to its layer name. The resulting set of layer names is `allow_imports_from`.

Rules:
- `domain` (or the lowest shared-types layer) typically has an **empty** `allow_imports_from`.
- Sibling layers at the same level (e.g. `analyzer` and `llm`) should NOT appear in each other's allow list — they are independent by design.
- An entrypoint layer (`cmd`) may allow all other layers.
- `allow_imports_from` declares **permission**, not obligation. A layer that is allowed but not actually imported is fine.

---

## How to Derive `forbidden_dependencies`

`forbidden_dependencies` and `allow_imports_from` are complementary:

- `allow_imports_from` is a **whitelist** of allowed edges — the static checker flags anything not on the list.
- `forbidden_dependencies` is an explicit **blacklist** — adds CRITICAL severity for the most dangerous violations.

Include a `forbidden_dependencies` entry when:
1. An upward dependency would completely invert the architecture (e.g. `domain → tools`).
2. Two sibling layers must never couple (e.g. `analyzer → llm`, `llm → analyzer`).
3. A lower layer must not know about a higher-level concept (e.g. `service → transport`).

Always write the `reason` field — it is shown in the LLM prompt and in violation reports.

Avoid redundancy: if layer A has an empty `allow_imports_from`, it already cannot import anything. You may still add explicit `forbidden_dependencies` for documentation clarity, but it is not required for correct detection.

---

## How to Write `constraints`

Constraints are free-text invariants that the static checker does not verify but the LLM evaluates. Use them for rules that require semantic understanding:

- Interface design rules (`"Only one Provider interface may exist in the system"`)
- State rules (`"No package-level mutable global variables except X"`)
- Initialization rules (`"No init() side effects except in Y"`)
- Ownership rules (`"All shared exported types must be defined in internal/domain"`)
- Concurrency rules (`"All LLM calls must go through the semaphore in ToolExecutor"`)

Keep each entry to one sentence. Do not repeat what `allow_imports_from` already expresses.

---

## Validation Checklist

Before saving the file, verify:

- [ ] Every `patterns` entry is a **relative** path (no leading `/`, no `..`).
- [ ] Every `patterns` entry corresponds to a directory that **actually exists** in the project.
- [ ] All layer names referenced in `allow_imports_from` and `forbidden_dependencies` match a `name` in the `layers` array.
- [ ] No layer has itself in `allow_imports_from`.
- [ ] The lowest-level layer (shared types / domain) has `"allow_imports_from": []`.
- [ ] Sibling layers that must be independent are listed in `forbidden_dependencies` (both directions if needed).
- [ ] The file is valid JSON (no trailing commas, no comments).

---

## Minimal Example

For a Go project with structure:
```
cmd/
internal/
  domain/
  service/
  repository/
  transport/
```

```json
{
  "description": "Three-layer Go service: transport → service → repository, all over domain.",
  "layers": [
    {
      "name": "domain",
      "description": "Shared types. No internal imports.",
      "patterns": ["internal/domain"],
      "allow_imports_from": []
    },
    {
      "name": "repository",
      "description": "Data access. Depends only on domain.",
      "patterns": ["internal/repository"],
      "allow_imports_from": ["domain"]
    },
    {
      "name": "service",
      "description": "Business logic. Depends on domain and repository.",
      "patterns": ["internal/service"],
      "allow_imports_from": ["domain", "repository"]
    },
    {
      "name": "transport",
      "description": "HTTP handlers. Depends on service and domain.",
      "patterns": ["internal/transport"],
      "allow_imports_from": ["domain", "service"]
    },
    {
      "name": "cmd",
      "description": "Entry point. Wires all layers.",
      "patterns": ["cmd"],
      "allow_imports_from": ["domain", "repository", "service", "transport"]
    }
  ],
  "forbidden_dependencies": [
    {
      "from": "domain",
      "to": "service",
      "reason": "domain is the foundation — must have zero internal imports"
    },
    {
      "from": "domain",
      "to": "repository",
      "reason": "domain is the foundation — must have zero internal imports"
    },
    {
      "from": "repository",
      "to": "service",
      "reason": "repository must not depend on business logic"
    },
    {
      "from": "service",
      "to": "transport",
      "reason": "service must not depend on transport protocol"
    }
  ],
  "constraints": [
    "All shared types must be defined in internal/domain, not in layer-local files",
    "No init() functions with side effects",
    "No package-level mutable state"
  ]
}
```

---

## Common Mistakes

| Mistake | Effect | Fix |
|---|---|---|
| `"patterns": ["/home/user/project/internal/domain"]` — absolute path | `os.Stat` succeeds but import prefix matching fails — all packages in that layer are unresolved | Use `"internal/domain"` |
| `"allow_imports_from": ["internal/llm"]` — path instead of name | Layer never matched — treated as unknown layer name | Use the layer `name`: `"llm"` |
| Omitting a real directory from `patterns` | Static checker reports layer missing; import edges not resolved to that layer | Add the directory |
| Sibling layers in each other's `allow_imports_from` | No static violation reported, but LLM may flag the coupling | Remove the cross-entry and add to `forbidden_dependencies` |
| `forbidden_dependencies` uses paths instead of names | Rules never match — violations go undetected | Use layer names |
