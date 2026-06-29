# Features Checklist — Agents Manager

## Phase 0: Project Scaffold

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 0.1 | Go module init | ✅ | `go.mod` with `go 1.25.0` |
| 0.2 | Config loader | ✅ | YAML parsing, `~/` expansion, layered defaults |
| 0.3 | Structured logger | ✅ | JSON output to stdout, levels debug/info/warn/error |
| 0.4 | Main entrypoint | ✅ | `--serve`, `--repl`, `--demo` flags |

## Phase 1: Core — Agent Pool

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 1.1 | Agent struct | ✅ | `internal/pool/agent.go` — PID, state, buffers, stdin/stdout/stderr |
| 1.2 | AgentPool (os/exec) | ✅ | `internal/pool/pool.go` — spawn, PipeMode + PTYMode |
| 1.3 | ResponseEvent channel | ✅ | `internal/pool/event.go` — typed events streamed to clients |
| 1.4 | Kill & Reap (SIGTERM→SIGKILL) | ✅ | 5s grace period, PTY cleanup |
| 1.5 | FIFO queue | ✅ | Notifies client of position when pool full |
| 1.6 | Test harness | ✅ | 3 test agent scripts (echo, timed, linger) + comprehensive tests |

## Phase 2: Core — Session & Shell Commands

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 2.1 | SessionStore | ⚠️ | In-memory only — **BoltDB persistence not implemented** |
| 2.2 | WorkspaceRegistry | ✅ | `internal/workspace/workspace.go` — exact paths + `/*` glob |
| 2.3 | AgentRegistry | ✅ | Scans `~/.agents_manager/agents/` + `$PATH` system agents |
| 2.4 | SkillRegistry | ✅ | Scans global/workspace/agent scopes, `.desc` files, `--help` fallback |
| 2.5 | Shell command router | ✅ | `internal/shell/shell.go` — dispatches built-ins vs agent forwarding |
| 2.6 | Built-in commands | ✅ | `help`, `workspaces/ws`, `ls`, `cd`, `open`, `pwd`, `ps`, `use`, `agents`, `skills`, `kill`, `history`, `os`, `repl` |
| 2.7 | Request forwarder | ✅ | Forwards non-built-in input to current agent |

## Phase 3: Protocol — HTTP + WebSocket + SSE

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 3.1 | HTTP server | ✅ | `internal/server/server.go` — serves SPA + API on configurable port |
| 3.2 | WebSocket endpoint (`/shell`) | ✅ | `internal/server/ws.go` — bidirectional terminal over WS |
| 3.3 | SSE endpoint (`/sse/dashboard`) | ✅ | `internal/server/sse.go` — 2s polling of session/pool state |
| 3.4 | REST API | ✅ | `/api/sessions`, `/api/workspaces`, `/api/agents`, `/api/pool`, `/api/workspace/{id}/tree`, `/api/session/{id}/active-file`, `/api/process/{pid}/stdin` |

## Phase 4: Authentication

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 4.1 | bcrypt password store | ✅ | JSON file with bcrypt hash |
| 4.2 | Login → JWT tokens | ✅ | Access (1h) + Refresh (30d) tokens |
| 4.3 | Auth endpoints | ✅ | `POST /auth/login`, `POST /auth/refresh`, `POST /auth/setup`, `GET /auth/status` |
| 4.4 | Token middleware | ✅ | Bearer token validation on WS/SSE/API |
| 4.5 | Rate limiting + lockout | ✅ | Per-IP, configurable max attempts and lockout duration |

## Phase 5: Control Centre SPA

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 5.1 | Static HTML shell | ✅ | `web/index.html` — Vite/React SPA with xterm.js terminal |
| 5.2 | Login page | ✅ | JWT-based auth flow (SPA communicates with `/auth/*`) |
| 5.3 | Dashboard (SSE) | ✅ | Real-time session/agent status via SSE |
| 5.4 | Shell terminal (WS) | ✅ | Full xterm.js terminal over WebSocket |
| 5.5 | Top bar (workspace/agent selector) | ✅ | Present in SPA |
| 5.6 | Token auto-refresh | ✅ | Built into SPA |

## Phase 6: SSH Server (Lower Priority)

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 6.1 | SSH listener on `:2222` | ❌ | **Not implemented** — config exists but no SSH server code |
| 6.2 | Public key auth | ❌ | **Not implemented** |
| 6.3 | Password auth (bcrypt) | ❌ | **Not implemented** (auth module exists but no SSH integration) |
| 6.4 | Session per SSH conn | ❌ | **Not implemented** |
| 6.5 | PTY/raw terminal over SSH | ❌ | **Not implemented** |

## Phase 7: Agent Shell Scripts (Low Priority)

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 7.1 | `agents_manager shell` mode | ❌ | **Not implemented** |
| 7.2 | `.ash` script execution | ❌ | **Not implemented** |
| 7.3 | Script discovery (`ls`) | ❌ | **Not implemented** |

## Phase 8: Polish

| # | Feature | Status | Notes |
|---|---------|--------|-------|
| 8.1 | Crash recovery (BoltDB) | ❌ | **Not implemented** — sessions are in-memory only |
| 8.2 | Multi-device sync | ❌ | Blocked on BoltDB persistence |
| 8.3 | Graceful shutdown | ⚠️ | Pool drains on `Shutdown()` but no OS `SIGTERM` handler in `main()` |
| 8.4 | Resource limits per agent | ❌ | **Not implemented** |
| 8.5 | Prometheus metrics | ❌ | **Not implemented** (structured logs exist) |
| 8.6 | Single binary with embedded static files | ❌ | Static files not embedded via `//go:embed` — relies on filesystem path |

## Additional Features Implemented (Not in Original Docs)

| Feature | Status | Location |
|---------|--------|----------|
| `repl` command (interactive PTY agent session) | ✅ | `internal/shell/session_mgmt.go:92` |
| `open <workspace>` command | ✅ | `internal/shell/navigate.go:14` |
| `cd <subdir>` sub-workspace navigation | ✅ | `internal/shell/navigate.go:51` |
| `ws` alias for `workspaces` | ✅ | `internal/shell/shell.go:52` |
| Per-agent WebSocket stream `/ws/agent/{pid}` | ✅ | `internal/server/ws_agent.go` |
| Terminal query filter (DSR/DECRQM responses) | ✅ | `internal/server/ws_agent.go:16-47` |
| Workspace file tree API with gitignore support | ✅ | `internal/workspace/workspace.go:139`, `/api/workspace/{id}/tree` |
| Active file tracking per session | ✅ | `/api/session/{id}/active-file` |
| Agent I/O REST endpoints (input/stream/output) | ✅ | `/api/agent/{pid}/input`, `/api/agent/{pid}/stream`, `/api/agent/{pid}/output` |
| Stdin injection REST endpoint | ✅ | `/api/process/{pid}/stdin` |

## Suggestions for Future Addition

1. **SSH server** — the single largest missing feature; `golang.org/x/crypto/ssh` is already available in `go.mod`
2. **BoltDB persistence** — sessions vanish on restart; implement `internal/session/store.go` BoltDB backend (already designed, `go.etcd.io/bbolt` compatible)
3. **Graceful shutdown** — add `os.Signal` handler for `SIGTERM`/`SIGINT` in `main.go` to drain pool + save state
4. **`.ash` script execution** — `agents_manager shell` mode that extends bash with built-in commands
5. **Resource limits** — per-agent CPU/memory limits via cgroups or `exec.Cmd.SysProcAttr`
6. **Embed static files** — use `//go:embed` to bundle the web SPA into the binary for single-file distribution
7. **Prometheus metrics endpoint** — expose `/metrics` for observability
8. **Provider config per agent** — `ProviderConfig` exists in `agent.go` but isn't configurable from YAML; could load per-agent `.yaml` files from `~/.agents_manager/agents/<name>/`
9. **Multi-device session sync** — once BoltDB is in place, same session state visible across SSH + WebSocket simultaneously
