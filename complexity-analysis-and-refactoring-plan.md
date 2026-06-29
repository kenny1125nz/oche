# Complexity Analysis & Refactoring Plan

**Generated:** June 30, 2026  
**Project:** oche (AI Agent Runtime)  
**Languages:** Go (backend) + TypeScript/React (frontend)

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Backend (Go) Analysis](#2-backend-go-analysis)
   - [Aggregate Statistics](#21-aggregate-statistics)
   - [Package-by-Package Breakdown](#22-package-by-package-breakdown)
   - [Top Code Smells (Ranked)](#23-top-code-smells-ranked)
3. [Frontend (TypeScript/React) Analysis](#3-frontend-typescriptreact-analysis)
   - [Aggregate Statistics](#31-aggregate-statistics)
   - [File-by-File Breakdown](#32-file-by-file-breakdown)
   - [Top Code Smells (Ranked)](#33-top-code-smells-ranked)
4. [Cross-Cutting Issues](#4-cross-cutting-issues)
5. [Refactoring Recommendations](#5-refactoring-recommendations)
   - [Phase 1: Quick Wins (Low Risk, High Value)](#51-phase-1-quick-wins)
   - [Phase 2: Structural Refactors (Medium Risk)](#52-phase-2-structural-refactors)
   - [Phase 3: Architecture Improvements (Higher Risk)](#53-phase-3-architecture-improvements)

---

## 1. Executive Summary

This codebase comprises **28 non-test Go source files** (~3,976 lines) and **19 TypeScript/React source files** (~1,800 lines). Overall complexity is moderate, with significant hotspots in the process pool (`pool/pool.go`), HTTP server (`server/`), and terminal UI components (`Terminal.tsx`, `InteractiveTab.tsx`).

**Key findings:**

- **6 functions exceed 80 lines** (the largest is 156 lines)
- **Zero Go interfaces** — making unit testing and dependency swapping harder
- **Several major duplicated code patterns** — most notably streaming logic (3 occurrences) and xterm theme (2 occurrences)
- **No abstraction for storage** — all stores are in-memory with no persistence hook
- **Auth tokens passed in URL query strings** — a security concern

---

## 2. Backend (Go) Analysis

### 2.1 Aggregate Statistics

| Metric | Total |
|---|---|
| Total files | 28 |
| Total lines | 3,976 |
| Total functions/methods | 104 |
| Total struct types | 38 |
| Total interface types | **0** |
| Functions > 80 lines | 6 |
| Functions > 100 lines | 3 |
| Average function length | ~38 lines |

### 2.2 Package-by-Package Breakdown

#### `internal/pool/pool.go` — 632 lines (LARGEST FILE)

| Metric | Value |
|---|---|
| Functions/methods | 25 |
| Structs | 2 (`Pool`, `queuedRequest`) |
| Longest function | `monitor()` — **156 lines** |

**Smells:**

| Severity | Issue | Detail |
|---|---|---|
| **CRITICAL** | `monitor()` is 156 lines | Handles stdout reading (PTY & pipe modes), stderr reading, process exit, error classification, event emission, cleanup, and queue dispatch — should be 3+ functions |
| **CRITICAL** | `spawn()` is 133 lines with 5× duplicate error handling | Each of 5 error paths (PTY error, stdin/out/err pipe errors, cmd.Start failure) repeats `cancel() → removeInstance() → sendError() → close(ch) → return` |
| **HIGH** | PTY vs Pipe mode duplication | The stdout-reading goroutine for PTY mode is nearly identical to non-PTY mode — only PTY cleanup differs |
| **MEDIUM** | Many tiny locked methods | `ActiveCount`, `QueuedCount`, `HasActive`, `GetProcessStartTime` each acquire/release Lock independently |

#### `internal/server/api.go` — 346 lines

| Metric | Value |
|---|---|
| Functions/methods | 15 |
| Longest function | `agentStreamHandler()` — **90 lines** |

**Smells:**

| Severity | Issue | Detail |
|---|---|---|
| **HIGH** | `agentStreamHandler()` mixes 3 concerns | SSE setup, drain loop, history replay, live streaming — all in one function |
| **MEDIUM** | Inner function `drainChan()` | Defined inside `agentStreamHandler()` — untestable in isolation |
| **MEDIUM** | `parseAgentPID()` / `parseProcessPID()` duplication | Nearly identical, differ only in URL prefix |
| **LOW** | No HTTP method validation on several handlers | `sessionsHandler`, `workspacesHandler`, `agentsHandler` accept any method silently |

#### `internal/server/ws.go` — 135 lines

| Metric | Value |
|---|---|
| Functions | 1 (`serve()` — **109 lines**) |

**Smells:**

| Severity | Issue | Detail |
|---|---|---|
| **HIGH** | `serve()` is 109 lines | Entire WebSocket lifecycle in one function: upgrade, session creation, writer goroutine, reader goroutine, resize handling, input routing, cleanup |
| **HIGH** | `runMu` locked across command execution | Writer goroutine holds `runMu` for the entire duration, blocking concurrent input |
| **MEDIUM** | 3-way resize branching | OS process vs agent process vs store-only — fragile conditional logic |

#### `internal/server/ws_agent.go` — 133 lines

| Metric | Value |
|---|---|
| Functions | 1 (`serve()` — **116 lines**) |

**Smells:**

| Severity | Issue | Detail |
|---|---|---|
| **HIGH** | `serve()` is 116 lines (3rd longest) | Contains `drainChan()` inner function, reader goroutine, replay logic, live streaming |
| **HIGH** | Duplicates `agentStreamHandler()` in api.go | Both implement the same drain-replay-live pattern with near-identical structure |
| **MEDIUM** | Double `defer conn.Close()` | The reader goroutine defers `conn.Close()` which races with the parent function's defer |

#### `internal/shell/navigate.go` — 200 lines

| Metric | Value |
|---|---|
| Functions | 4 |
| Longest function | `cmdCd()` — **96 lines** |

**Smells:**

| Severity | Issue | Detail |
|---|---|---|
| **HIGH** | `cmdCd()` is 96 lines | Handles 5 cases (no args, "/", "..", ".", normal dir) with 4-level nesting in the ".." case |
| **MEDIUM** | `cmdLs()` shells out to `/bin/ls` | Fragile — depends on system `ls` being present; should use `os.ReadDir` natively |

#### `internal/shell/shell.go` — 81 lines

| Severity | Issue | Detail |
|---|---|---|
| **MEDIUM** | 15-case switch in `Execute()` | Adding commands requires modifying this switch — no registration pattern |
| **LOW** | `Router` constructor takes 5 positional parameters | Should use a config struct |

#### `internal/shell/forward.go` — 94 lines

| Severity | Issue | Detail |
|---|---|---|
| **MEDIUM** | 4× identical error-response channel creation | Each follows `make(chan, 5) → emit(error) → deliverable → close → return` — extract helper |

#### `internal/shell/session_mgmt.go` — 151 lines

| Severity | Issue | Detail |
|---|---|---|
| **MEDIUM** | `cmdRepl()` has 4 sequential error-check blocks | Each follows `fetch → check → emit → close → return` identically |

#### `internal/skill/skill.go` — 118 lines

| Severity | Issue | Detail |
|---|---|---|
| **MEDIUM** | `GetAll()` has triple-repeated dedup code | Three identical `if !seen[name] { ... append }` blocks for agent/workspace/global — should be a loop |

#### `cmd/agents_manager/main.go` — 267 lines

| Severity | Issue | Detail |
|---|---|---|
| **MEDIUM** | `runServe()` and `runREPL()` share ~50 lines duplicated | Workspace discovery, agent discovery, skill registry, session store, pool creation — extract `initServices()` |
| **LOW** | JWT secret not persisted | `rand.Read(jwtSecret)` on every restart invalidates all tokens |

#### `internal/auth/jwt.go` — 92 lines

| Severity | Issue | Detail |
|---|---|---|
| **MEDIUM** | Custom JWT implementation | Instead of `golang-jwt/jwt` — may miss algorithm confusion protection, `typ` validation, `nbf` check |

#### `internal/auth/auth.go` — 161 lines

| Severity | Issue | Detail |
|---|---|---|
| **LOW** | `NewManager()` takes 7 positional parameters | Should use a config struct |
| **LOW** | Hardcoded claim strings | `"sub": "user"`, `"type": "access"/"refresh"` — should be constants |

#### `internal/server/server.go` — 115 lines

| Severity | Issue | Detail |
|---|---|---|
| **MEDIUM** | `Start()` has 3 inline anonymous handlers | Untestable — should be named handler methods |
| **LOW** | No graceful shutdown | No signal handling, no server.Shutdown() |

#### `internal/shell/os_cmd.go` — 121 lines

| Severity | Issue | Detail |
|---|---|---|
| **MEDIUM** | `term.MakeRaw` error silently ignored | Line 52-53: `if _, err := term.MakeRaw(...); err != nil {}` |
| **MEDIUM** | Package-level `sync.Map` for OS process tracking | Global mutable state |

#### `internal/config/config.go` — 145 lines

| Severity | Issue | Detail |
|---|---|---|
| **LOW** | `Default()` returns 35-line nested literal | Should use builder pattern or smaller defaults |

#### `internal/agent/agent.go` — 165 lines

| Severity | Issue | Detail |
|---|---|---|
| **LOW** | `ProviderConfigs` is a global mutable map | Not thread-safe if modified concurrently |
| **LOW** | `knownSystemAgents` has a duplicate | `"copilot"` and `"github-copilot-cli"` are separate entries for the same tool |

#### Clean files (no significant issues)

- `internal/pool/event.go` (13 lines)
- `internal/session/session.go` (53 lines)
- `internal/session/store.go` (85 lines, though no interface abstraction)
- `internal/workspace/workspace.go` (171 lines, minor)
- `internal/shell/channel.go` (42 lines)
- `internal/shell/info.go` (139 lines)
- `internal/server/sse.go` (89 lines)
- `internal/server/middleware.go` (84 lines)
- `internal/auth/password.go` (79 lines)
- `internal/auth/ratelimit.go` (56 lines)
- `internal/logger/logger.go` (26 lines)

### 2.3 Top Code Smells (Ranked)

| Rank | Issue | Location | Severity |
|---|---|---|---|
| 1 | `monitor()` is 156 lines | `pool/pool.go:423-578` | CRITICAL |
| 2 | `spawn()` 133 lines + 5× duplicate error handling | `pool/pool.go:289-421` | CRITICAL |
| 3 | `serve()` 116 lines + duplicate streaming pattern | `server/ws_agent.go:18-133` | HIGH |
| 4 | `serve()` 109 lines + complex goroutine coordination | `server/ws.go:27-135` | HIGH |
| 5 | `agentStreamHandler()` 90 lines + inner function | `server/api.go:186-276` | HIGH |
| 6 | `cmdCd()` 96 lines with 5 cases | `shell/navigate.go:51-146` | HIGH |
| 7 | `runServe()`/`runREPL()` share ~50 lines | `cmd/agents_manager/main.go` | MEDIUM |
| 8 | 4× duplicate error-channel creation | `shell/forward.go` | MEDIUM |
| 9 | Triple-repeated dedup in `GetAll()` | `skill/skill.go:94-117` | MEDIUM |
| 10 | `cmdRepl()` 4 sequential error checks | `shell/session_mgmt.go:92-151` | MEDIUM |

---

## 3. Frontend (TypeScript/React) Analysis

### 3.1 Aggregate Statistics

| Metric | Total |
|---|---|
| Total files | 19 |
| Total lines | ~1,800 |
| Total components | 8 |
| Total custom hooks | 4 |
| Functions > 50 lines | 6 |
| Functions > 100 lines | 1 |
| Components with 5+ hooks | 4 |

### 3.2 File-by-File Breakdown

#### `src/App.tsx` — 189 lines

| Metric | Value |
|---|---|
| Components | 1 (`App`) |
| Hooks | 9 (4 `useEffect`, 3 `useCallback`, 1 `useState`, 1 `useRef`) |
| Longest | `App()` — 164 lines |

**Smells:**

| Severity | Issue | Detail |
|---|---|---|
| HIGH | Module-level mutable state | `streamBuffers` (Map) and `streamVersions` (Map) at module scope — stale closure risk |
| HIGH | Force-render hack | `[, forceRender] = useState(0)` to trigger re-renders from stream callbacks |
| MEDIUM | 3 concurrent polling intervals | Dashboard every 3s, token refresh every 30s, SSE connection |
| MEDIUM | Prop drilling into InteractiveTab | `tab`, `buffer`, `streamVersion`, `sendToAgent`, `resizeAgent` all passed as props |

#### `src/replShell.ts` — 198 lines

| Metric | Value |
|---|---|
| Functions | 7 class methods |
| Longest | `handleTerminalData()` — **103 lines** |

**Smells:**

| Severity | Issue | Detail |
|---|---|---|
| CRITICAL | `handleTerminalData()` is 103 lines | Handles 10+ terminal escape sequences in a single if/else-if chain (backspace, arrows, history nav, cursor movement, text insert) |
| HIGH | No TypeScript typing on class properties | `busy`, `currentLine`, `cmdQueue` — all implicitly typed |
| MEDIUM | Duplicate history traversal patterns | Up/down arrow handling (lines 95-133) has near-identical terminal-write-and-render logic |
| MEDIUM | Mutable class state | `busy`, `currentLine`, `cursorPos`, `historyIndex` all mutated in place |

#### `src/components/Terminal.tsx` — 161 lines

| Metric | Value |
|---|---|
| Components | 1 (`Terminal`) |
| Hooks | 6+ |
| Longest | `useEffect` 2 (terminal bootstrap) — **71 lines** |

**Smells:**

| Severity | Issue | Detail |
|---|---|---|
| HIGH | 71-line `useEffect` | Combines terminal creation, addon loading, theme config, shell init, data binding, ResizeObserver, cleanup |
| HIGH | 22-line inline theme object duplicated | Same 22-line theme literal exists verbatim in `InteractiveTab.tsx:34-55` |
| MEDIUM | `sendRef`/`termWriteRef` callback-ref pattern | Workaround for stale closures, makes data flow harder to trace |

#### `src/components/InteractiveTab.tsx` — 144 lines

| Metric | Value |
|---|---|
| Components | 1 (`InteractiveTab`) |
| Hooks | 5 (3 `useEffect`, 2 `useRef`, 1 `useState`) |
| Longest | `useEffect` 1 (terminal setup) — **63 lines** |

**Smells:**

| Severity | Issue | Detail |
|---|---|---|
| HIGH | 22-line theme duplicate | Same object as `Terminal.tsx:93-114` — biggest repeated code in the frontend |
| HIGH | 63-line `useEffect` | Terminal instantiation, addon, ResizeObserver, data binding, cleanup — extract into shared hook |
| MEDIUM | `renderedRef` mutable ref | Tracks rendered buffer lines incrementally — fragile if buffers modify out of order |

#### `src/components/AuthScreen.tsx` — 92 lines

| Severity | Issue | Detail |
|---|---|---|
| MEDIUM | `handleSubmit()` 48 lines with duplicated branches | Setup path (25-39) and login path (43-58) share ~90% identical code — only the API call differs |
| MEDIUM | No `useCallback` | `handleSubmit` recreated on every render |

#### `src/components/AgentModal.tsx` — 103 lines

| Severity | Issue | Detail |
|---|---|---|
| MEDIUM | Third instance of SSE streaming pattern | The EventSource setup is nearly identical to `useAgentStreams.ts` and `api.go:agentStreamHandler()` |
| MEDIUM | Unbounded memory growth | `setOutput(prev => prev + ...)` appends without limit |
| LOW | Unhandled promise from async `handleKeyDown` | `onKeyDown={handleKeyDown}` ignores returned promise |

#### `src/components/Dashboard.tsx` — 67 lines

| Severity | Issue | Detail |
|---|---|---|
| LOW | Inline styles throughout | Should use CSS classes |
| LOW | Empty-state messages hardcoded | "No sessions", "No active agents" in JSX conditionals |

#### `src/components/TopBar.tsx` — 54 lines

| Severity | Issue | Detail |
|---|---|---|
| LOW | Inline conditional class concatenation | `'tab' + (active ? ' active' : '')` repeated 3+ times — use `classnames` utility |

#### `src/hooks/useAgentWS.ts` — 88 lines

| Severity | Issue | Detail |
|---|---|---|
| HIGH | ~85% overlap with `useAgentStreams.ts` | Nearly identical structure — different protocol (WS vs SSE) |
| MEDIUM | `tabKey` derived dependency | `tabs.map(t => t.pid).sort().join(',')` — fragile, misses updates if order changes |
| MEDIUM | `window.location.origin.replace('http', 'ws')` | Breaks on HTTPS |
| LOW | No reconnection logic | WS disconnects silently |

#### `src/hooks/useAgentStreams.ts` — 67 lines

| Severity | Issue | Detail |
|---|---|---|
| HIGH | ~85% overlap with `useAgentWS.ts` | Should be unified with a protocol parameter |
| MEDIUM | Same `tabKey` derived dependency | Same issue as `useAgentWS.ts` |

#### `src/hooks/useWebSocket.ts` — 82 lines

| Severity | Issue | Detail |
|---|---|---|
| MEDIUM | Token in URL query string | Line 25: `accessToken` as query param — logged by servers |
| MEDIUM | No exponential backoff for reconnect | Raw `setTimeout` with no jitter or backoff |
| MEDIUM | Deeply nested `onmessage` handler | 5-branch conditional routing |

#### `src/state.tsx` — 136 lines

| Severity | Issue | Detail |
|---|---|---|
| MEDIUM | 18-field `AppState` interface | Combines auth, terminal, session, and UI state |
| MEDIUM | 17-action union in single reducer | Could split into domain-specific reducers |

#### `src/types.ts` — 54 lines

| Severity | Issue | Detail |
|---|---|---|
| LOW | `StreamBuffer` interface is dead code | Defined but never imported or used anywhere |

#### `src/api/auth.ts` — 49 lines

| Severity | Issue | Detail |
|---|---|---|
| LOW | Inconsistent error handling | `setupPassword`/`login` parse response errors; `refreshAccess` throws generic error without parsing body |

#### Clean files (no significant issues)

- `src/main.tsx` (13 lines)
- `src/replBridge.ts` (37 lines, though global singleton pattern)
- `src/utils/filterTerminalQueries.ts` (30 lines)
- `src/api/pool.ts` (17 lines)

### 3.3 Top Code Smells (Ranked)

| Rank | Issue | Location | Severity |
|---|---|---|---|
| 1 | `handleTerminalData()` 103 lines with 10+ sequences | `replShell.ts:61-163` | CRITICAL |
| 2 | Module-level mutable maps | `App.tsx:14-15` | HIGH |
| 3 | xterm theme 22-line duplicate | `Terminal.tsx:93-114`, `InteractiveTab.tsx:34-55` | HIGH |
| 4 | 71-line terminal bootstrap useEffect | `Terminal.tsx:84-154` | HIGH |
| 5 | 63-line terminal setup useEffect | `InteractiveTab.tsx:25-87` | HIGH |
| 6 | ~85% overlap between useAgentWS / useAgentStreams | `hooks/useAgentWS.ts`, `hooks/useAgentStreams.ts` | HIGH |
| 7 | Force-render hack with useState(0) | `App.tsx:21` | MEDIUM |
| 8 | Duplicated login/setup branches | `AuthScreen.tsx:25-58` | MEDIUM |
| 9 | Token in URL query strings | `useWebSocket.ts:25`, `useSSE.ts:14`, `useAgentWS.ts:31` | MEDIUM |
| 10 | Third copy of SSE streaming pattern | `AgentModal.tsx` | MEDIUM |

---

## 4. Cross-Cutting Issues

### Security

| Issue | Files | Risk |
|---|---|---|
| Auth tokens in URL query strings | `useWebSocket.ts`, `useSSE.ts`, `useAgentWS.ts`, `auth.ts`, `pool.ts` | Moderate — tokens may appear in server access logs |
| Custom JWT implementation | `auth/jwt.go` | Moderate — lacks library-level protections (algorithm confusion, `typ` claim, `nbf` claim) |
| `term.MakeRaw` error silently ignored | `shell/os_cmd.go:52-53` | Low — may leave terminal in raw mode on error |

### Architecture

| Issue | Impact |
|---|---|
| **Zero Go interfaces** | All packages depend on concrete implementations; cannot mock in tests or swap storage providers |
| **No storage abstraction** | Session, workspace, and agent stores are hardcoded in-memory maps |
| **Global mutable state** | `replBridge.ts` (module-level Maps), `App.tsx` (module-level stream buffers), `agent/agent.go` (global ProviderConfigs map), `shell/os_cmd.go` (package-level sync.Map) |
| **No graceful shutdown** | `server/server.go` doesn't handle signals or call `server.Shutdown()` |
| **No command registration pattern** | `shell/shell.go` uses a 15-case switch — adding commands means modifying this function |

### Testing

| Issue | Impact |
|---|---|
| Inline anonymous handlers in `server.go` | Cannot unit-test route handlers in isolation |
| No interfaces for key components (Pool, Store, Registry) | Cannot mock dependencies; tests require real process pools and file systems |
| `pool/pool.go:monitor()` and `spawn()` are monolithic | Hard to test individual concerns (stdout reading, error handling, queue dispatch) |

### Maintainability

| Issue | Impact |
|---|---|
| Major duplicated code patterns (streaming, theme, auth setup) | Changes must be made in multiple places — drift risk |
| `replShell.ts` terminal escape sequence handling in one method | Adding new sequences inflates an already-long function |
| No shared `BASE` constant — empty string duplicated in 4 files | `api/auth.ts`, `api/pool.ts`, `useWebSocket.ts`, `useSSE.ts`, `useAgentWS.ts` |

---

## 5. Refactoring Recommendations

### 5.1 Phase 1: Quick Wins

**Effort:** Small | **Risk:** Low | **Value:** High

#### 1. Extract shared xterm theme constant
- **Files:** `Terminal.tsx`, `InteractiveTab.tsx`
- **Action:** Create `src/theme.ts` with the shared 22-line color theme object
- **Rationale:** Eliminates the biggest duplication in the frontend

#### 2. Add `newErrorResponse()` helper
- **Files:** `shell/forward.go`
- **Action:** Replace 4× duplicate error-response channel creation with a helper function
- **Rationale:** Pattern with immediate payoff — ~20 lines eliminated

#### 3. Remove dead code
- **Files:** `src/types.ts` (`StreamBuffer` interface)
- **Action:** Delete unused type
- **Rationale:** Zero risk cleanup

#### 4. Add Go interfaces for key components
- **Files:** `pool/pool.go`, `session/store.go`, `workspace/workspace.go`, `agent/agent.go`
- **Action:** Define interfaces for `Pool`, `SessionStore`, `WorkspaceRegistry`, `AgentRegistry`
- **Rationale:** Enables unit testing without real processes/disks

#### 5. Consolidate `BASE` constant
- **Files:** `api/auth.ts`, `api/pool.ts`, `useWebSocket.ts`, `useSSE.ts`, `useAgentWS.ts`
- **Action:** Create `src/api/config.ts` exporting `const BASE = ''`
- **Rationale:** Single source of truth for API URL

### 5.2 Phase 2: Structural Refactors

**Effort:** Medium | **Risk:** Medium | **Value:** High

#### 6. Split `pool/pool.go:monitor()` (156 lines)
- **Target:** `internal/pool/pool.go`
- **Action:** Extract into:
  - `readStdout(ctx, ...)`
  - `readStderr(ctx, ...)`
  - `waitForExit(cmd, ...)`
  - `handleExit(evt, ...)`
  - `dispatchQueued(...)`
- **Rationale:** Cuts the longest function by 4×, each concern independently testable

#### 7. Split `pool/pool.go:spawn()` (133 lines)
- **Target:** `internal/pool/pool.go`
- **Action:** Extract `handleSpawnError()` for the 5× repeated error pattern; separate PTY and pipe mode into `spawnPTY()` and `spawnPipe()`
- **Rationale:** Eliminates copy-paste; clearer mode separation

#### 8. Unify `useAgentWS` and `useAgentStreams`
- **Target:** `hooks/useAgentWS.ts`, `hooks/useAgentStreams.ts`
- **Action:** Create `hooks/useAgentConnection.ts` with `type ConnectionProtocol = 'ws' | 'sse'`
- **Rationale:** Eliminates ~85% duplicated logic

#### 9. Extract `useXtermTerminal()` custom hook
- **Target:** `Terminal.tsx`, `InteractiveTab.tsx`
- **Action:** Extract terminal bootstrap logic (creation, addon, ResizeObserver, cleanup) into shared hook
- **Rationale:** Eliminates duplicate terminal setup logic in both files

#### 10. Break up `replShell.ts:handleTerminalData()` (103 lines)
- **Target:** `replShell.ts`
- **Action:** Replace if/else-if chain with a map of `string → handler`, e.g.:
  ```typescript
  private handlers: Record<string, () => void> = {
    '\x7f': () => this.handleBackspace(),
    '\x1b[A': () => this.handleArrowUp(),
    '\x1b[B': () => this.handleArrowDown(),
    // ...
  }
  ```
- **Rationale:** Each sequence independently testable; adding new sequences is a single map entry

### 5.3 Phase 3: Architecture Improvements

**Effort:** Large | **Risk:** Higher | **Value:** High (long-term)

#### 11. Unify duplicate streaming pattern (3 occurrences)
- **Target:** `server/api.go:agentStreamHandler()`, `server/ws_agent.go:serve()`, `AgentModal.tsx`
- **Action:** Extract a reusable `StreamSubscription` helper that handles:
  1. Subscribe to agent updates
  2. Drain channel
  3. Replay stdout/stderr history
  4. Enter live select loop
  5. Handle `__agent_exit__` signal
- **Rationale:** Eliminates the most heavily duplicated logic across the full stack

#### 12. Extract `initServices()` factory
- **Target:** `cmd/agents_manager/main.go`
- **Action:** Extract shared initialization (workspace discovery, agent discovery, skill registry, session store, pool, router) into a single `initServices()` function
- **Rationale:** Eliminates ~50 lines of duplication between `runServe()` and `runREPL()`

#### 13. Reduce 15-case switch in `shell/shell.go`
- **Target:** `internal/shell/shell.go`
- **Action:** Replace switch with `map[string]CommandHandler` — register commands via `router.Register("ls", cmdLs)`, etc.
- **Rationale:** Adding commands no longer modifies the switch; commands can be registered from different packages

#### 14. Refactor `AuthScreen.tsx` — merge setup and login branches
- **Target:** `components/AuthScreen.tsx`
- **Action:** Create a shared `authenticate(apiCall)` helper that handles the common "call API → dispatch tokens → store in localStorage → tryConnect" flow
- **Rationale:** Eliminates ~15 lines of duplicated fetch/store/connect code

#### 15. Migrate from custom JWT to library
- **Target:** `auth/jwt.go`
- **Action:** Replace custom HMAC-SHA256 JWT with `github.com/golang-jwt/jwt/v5`
- **Rationale:** Gains algorithm validation, standard claim checking, key rotation support

#### 16. Add graceful shutdown
- **Target:** `server/server.go`, `cmd/agents_manager/main.go`
- **Action:** Add `signal.Notify` for SIGINT/SIGTERM; call `server.Shutdown()` with context timeout
- **Rationale:** Ensures clean process cleanup and connection draining

#### 17. Eliminate global mutable state
- **Target:** `replBridge.ts`, `App.tsx:14-15`, `agent/agent.go:38`, `shell/os_cmd.go`
- **Action:**
  - `replBridge.ts`: Replace module-level Map with React Context
  - `App.tsx`: Move `streamBuffers`/`streamVersions` into component state or a ref
  - `agent/agent.go`: Use `sync.Map` or inject via Registry struct
  - `shell/os_cmd.go`: Pass `osProcesses` as parameter or use a scoped manager

#### 18. Move auth tokens out of URL query strings
- **Target:** `useWebSocket.ts`, `useSSE.ts`, `useAgentWS.ts`, `auth.ts`, `pool.ts`
- **Action:** Use `Authorization: Bearer <token>` header for REST; for WebSocket, pass token during the upgrade handshake or use a short-lived upgrade token
- **Rationale:** Eliminates token exposure in server access logs

---

## Summary: Recommended Order

```
Phase 1 (Quick Wins)
├── 1.  Extract shared xterm theme          [~15 min]
├── 2.  Add newErrorResponse() helper        [~10 min]
├── 3.  Remove dead code                     [~5 min]
├── 4.  Add Go interfaces                    [~1 hr]
└── 5.  Consolidate BASE constant            [~15 min]

Phase 2 (Structural Refactors)
├── 6.  Split monitor() (156→~40 lines)      [~2 hr]
├── 7.  Split spawn() (133→~50 lines)        [~2 hr]
├── 8.  Unify useAgentWS / useAgentStreams   [~2 hr]
├── 9.  Extract useXtermTerminal() hook      [~2 hr]
└── 10. Break up handleTerminalData()        [~2 hr]

Phase 3 (Architecture)
├── 11. Unify streaming pattern (3×)         [~4 hr]
├── 12. Extract initServices() factory       [~2 hr]
├── 13. Command registration pattern         [~3 hr]
├── 14. Refactor AuthScreen branches         [~1 hr]
├── 15. Migrate to golang-jwt library        [~3 hr]
├── 16. Add graceful shutdown                [~2 hr]
├── 17. Eliminate global mutable state       [~3 hr]
└── 18. Move tokens from URL query strings   [~2 hr]
```

**Estimated total effort:** ~30-35 hours across all phases.
