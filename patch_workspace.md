# Workspace Model — Conceptual Patch

## Motivation

The current workspace model is a flat `{id, path}` pair backed by a config-declared directory glob. Sessions carry a `workspace_id` string but there is no notion of workspace state (which file is being edited, what the source tree looks like, etc.). The terminal UI uses `ls` to list workspaces and `cd` to select them, conflating filesystem navigation with workspace management.

This patch reifies the workspace into a first-class object, introduces sub-workspace navigation, and realigns the CLI vocabulary.

---

## 1. Workspace Object

The flat `{ID, Path}` struct becomes a richer object:

```go
type Workspace struct {
    ID         string   `json:"id"`          // unique identifier = folder basename
    TargetDir  string   `json:"target_dir"`  // absolute filesystem path
    SourceTree []Entry  `json:"source_tree"` // cached/on-demand tree of files/dirs
    ActiveFile string   `json:"active_file"` // currently open file (relative to TargetDir)
}

type Entry struct {
    Name     string  `json:"name"`
    IsDir    bool    `json:"is_dir"`
    Children []Entry `json:"children,omitempty"` // populated on demand (1-2 levels)
}
```

- `ID` — folder basename, unchanged from current model. Serves as both key and display name.
- `TargetDir` — replaces `Path`. The directory that serves as the working directory for OS commands and agent execution.
- `SourceTree` — a lazily-populated tree of the workspace's contents. The server builds this on `cd`/`open` and on demand. Depth is configurable (default: 2 levels). Used by the frontend for file navigation UI.
- `ActiveFile` — tracks which file the user is currently working on within the workspace. Set by the frontend (clicking a file in the tree) or by the `edit` command. Persisted on the session.

No config format change — workspace discovery continues to use directory paths/globs as before. The workspace name is always the folder basename.

---

## 2. Command Changes

### 2.1 `workspaces` (alias `ws`) — replaces current `ls`

Lists all discovered workspaces with their metadata.

```
> workspaces
  myapp      ~/projects/myapp
  docs       ~/projects/docs-site

> ws
  myapp      ~/projects/myapp
  docs       ~/projects/docs-site
```

When a workspace is active:

```
> ws
* myapp      ~/projects/myapp       (active)
  docs       ~/projects/docs-site
```

The active workspace is highlighted with `*`.

### 2.2 `open` — selects a workspace (replaces current `cd`)

```
> open myapp
switched to workspace myapp (~/projects/myapp)
```

Equivalent to the old `cd myapp` behaviour. Accepts a workspace ID or name.

```
> open
usage: open <workspace-id>
```

### 2.3 `cd` — recursive sub-workspace navigation

`cd` no longer switches between top-level workspaces (`open` does that). Instead, `cd` navigates into sub-directories of the **current** workspace's `target_dir`, creating an ad-hoc sub-workspace.

```
> cd src
switched to workspace myapp/src (~/projects/myapp/src)

> cd utils
switched to workspace myapp/src/utils (~/projects/myapp/src/utils)

> cd ..
switched to workspace myapp/src (~/projects/myapp/src)

> cd /
switched to workspace myapp (~/projects/myapp)
```

- Each `cd <subdir>` resolves `<target_dir>/<subdir>` on the filesystem.
- The session's workspace is updated to a new **sub-workspace** with:
  - `ID = parentID + "/" + subdir` (e.g. `myapp/src`)
  - `Name = subdir`
  - `TargetDir = <resolved absolute path>`
- `cd ..` walks up one level (strips the last path component from the workspace ID).
- `cd /` returns to the base workspace.
- These sub-workspaces are **not** persisted in the registry — they are ephemeral session-level overrides derived from the base workspace.
- The frontend prompt updates to reflect the full path: `agent@myapp/src >`.

Rationale: In the terminal context, `cd` is a universal muscle-memory command for navigation. Repurposing it for directory-level workspace navigation feels natural, while `open` handles the higher-level "switch project" operation.

### 2.4 `ls` — repurposed for filesystem listing

`ls` becomes the OS-level directory listing command (equivalent to `os ls`). It lists the contents of the current workspace's `target_dir`.

```
> ls
src/
docs/
package.json
README.md

> ls src/
components/
utils/
index.ts
```

Implementation: `ls` now runs `exec.Command("ls", args...)` with `cmd.Dir` set to the current workspace's `target_dir`. It is a direct alias for `os ls` behaviour. The `os` command remains available for arbitrary OS commands:

```
> os ls -la
> os git status
```

---

## 3. Session → Workspace Binding

### 3.1 Session Workspace

Each session carries:

```go
type Session struct {
    // ... existing fields ...
    WorkspaceID  string `json:"workspace_id"`   // base workspace ID
    SubWorkspace string `json:"sub_workspace"`  // sub-path within workspace (e.g. "src/utils")
    ActiveFile   string `json:"active_file"`    // active file path relative to target_dir
}
```

- `WorkspaceID` — unchanged, references the base workspace from the registry.
- `SubWorkspace` — relative path from the base workspace's `target_dir`. Empty string means at the root of the base workspace.
- `ActiveFile` — relative file path from the current workspace `target_dir`. Updated when the user clicks a file in the frontend or uses an `edit` command.

### 3.2 Resolving the effective target directory

```go
func (s *Session) EffectiveDir(reg *workspace.Registry) string {
    ws, err := reg.Get(s.WorkspaceID)
    if err != nil {
        return ""
    }
    if s.SubWorkspace == "" {
        return ws.TargetDir
    }
    return filepath.Join(ws.TargetDir, s.SubWorkspace)
}
```

All subsystems (shell `cmdOs`, pool `Submit`, skills discovery, etc.) use `EffectiveDir()` instead of looking up the raw workspace path.

### 3.3 SSE / API exposure

The dashboard SSE stream and `/api/sessions` endpoint include both `workspace_id` and `sub_workspace`:

```json
{
  "sessions": [{
    "id": "ws-1719500000",
    "workspace_id": "myapp",
    "sub_workspace": "src/utils",
    "active_file": "utils/helpers.ts",
    ...
  }]
}
```

The frontend terminal prompt becomes: `agent@workspace/sub/path >`.

---

## 4. Source Tree & Active File

### 4.1 Source tree endpoint

A new API endpoint serves the source tree:

```
GET /api/workspace/{id}/tree?depth=2
```

Response:

```json
{
  "id": "myapp",
  "target_dir": "/home/user/projects/myapp",
  "tree": [
    {"name": "src", "is_dir": true, "children": [
      {"name": "components", "is_dir": true, "children": []},
      {"name": "utils", "is_dir": true, "children": []},
      {"name": "index.ts", "is_dir": false}
    ]},
    {"name": "package.json", "is_dir": false}
  ]
}
```

The tree is computed on demand by reading the directory. The scanner **respects `.gitignore` rules** to filter out unnecessary files:

- If the workspace contains a `.gitignore`, the tree builder parses it and skips matching paths (e.g. `node_modules/`, `.next/`, `*.pyc`, `dist/`).
- `.git` directories and their contents are always excluded.
- If no `.gitignore` is found, a sensible default set of ignores is applied: `node_modules`, `.git`, `__pycache__`, `.DS_Store`, `*.log`, `.venv`, `vendor` (Go), `target` (Rust), `dist`, `.next`, `.turbo`.
- The filter operates on the server side before the JSON response is sent — the frontend never sees filtered entries.
- Depth defaults to 2 levels. The response is cached briefly (5s TTL) to avoid repeated filesystem scans.

### 4.2 Active file tracking

The frontend sets the active file when a user clicks a file in the source tree:

```
POST /api/session/{id}/active-file
{"file": "src/utils/helpers.ts"}
```

Response: `{"ok": true}`

The active file is included in:
- Session SSE data (for dashboard display)
- Prompt display (frontend shows it in the terminal header/status line)
- Agent context (future: the active file content can be injected into agent prompts)

### 4.3 Frontend display

The top-bar (or a status line below it) shows:

```
agent@workspace/src/utils >  [active: helpers.ts]
```

---

## 5. Implementation Plan

| Step | File(s) | Change |
|------|---------|--------|
| 0 | `human-ai-agent-interaction-hld.md`, `vision.md`, `agents_manager/implementation-plan.md`, `agents_manager/technical-design.md` | Update all project docs to reflect new workspace model, command changes, and data model |
| 1 | `internal/workspace/workspace.go` | Expand `Workspace` struct: add `SourceTree`, `ActiveFile`; rename `Path` → `TargetDir`; add `Entry` type; add `Tree(depth)` method |
| 2 | `internal/session/session.go` | Add `SubWorkspace` and `ActiveFile` fields to `Session`; add `EffectiveDir()` method |
| 3 | `internal/shell/shell.go` | Rename `cmdLs` → `cmdWorkspaces` (keep `ls` as alias); add new `cmdOpen` replacing `cmdCd`; rework `cmdCd` for sub-workspace navigation; rework `cmdLs` as `os ls` alias; update prompt construction; update `forwardToAgent` to use `EffectiveDir()` |
| 4 | `internal/shell/shell.go` — `cmdOs` | Use `EffectiveDir()` for `cmd.Dir` |
| 5 | `internal/shell/shell.go` — `cmdSkills` | Use `EffectiveDir()` for workspace path |
| 6 | `internal/server/api.go` | Add `/api/workspace/{id}/tree` handler; add `/api/session/{id}/active-file` handler; update `workspacesHandler` to emit new fields; update `sessionsHandler` to emit `sub_workspace`/`active_file` |
| 7 | `internal/server/sse.go` | Include `sub_workspace` and `active_file` in session SSE data |
| 8 | Frontend | Update prompt display to show `agent@workspace/sub >`; add source tree panel; add active-file click handler |

---

## 6. Migration Notes

- **Backward compatibility**: The old `ls` → `workspaces` change is the most visible break. Users who type `ls` expecting workspace listing will now get a directory listing. The `ws` shortcut is provided for muscle-memory relief.
- **Session persistence**: Existing sessions with `workspace_id` set will continue to work. `SubWorkspace` defaults to empty string.
- **REPL compatibility**: All command changes apply to both WebSocket sessions and the REPL mode.
