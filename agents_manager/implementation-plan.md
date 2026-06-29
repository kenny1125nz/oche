# Agents Manager — Implementation Plan

## Phase 0: Project Scaffold

**Goal:** Runnable binary that starts, loads config, and prints "ready".

| # | Task | Details |
|---|------|---------|
| 0.1 | Init Go module | `go mod init`, directory structure |
| 0.2 | Config loader | Parse `agents_manager.yaml`, defaults for all values |
| 0.3 | Logger | Structured logging to stdout (levels: debug, info, warn, error) |
| 0.4 | Main entrypoint | Load config, print "Agents Manager ready on :8080" |

**Deliverable:** `go run .` starts and exits cleanly.

---

## Phase 1: Core — Agent Pool

**Goal:** Spawn CLI agents as OS processes, pipe stdin/stdout, stream output back.

| # | Task | Details |
|---|------|---------|
| 1.1 | Agent struct | pid, cmd handle, stdout/stderr buffers, state |
| 1.2 | AgentPool | Fixed-size pool, spawn via `os/exec`, stdin writer, stdout reader goroutine |
| 1.3 | ResponseEvent channel | Stdout lines wrapped as events, sent to a channel |
| 1.4 | Kill & Reap | `kill` terminates agent (SIGTERM → 5s → SIGKILL), reap cleans up |
| 1.5 | Queue | When pool full, FIFO queue; notify client of position |
| 1.6 | Test harness | Dummy agent binary (`echo`-based) for integration tests |

**Deliverable:** Can spawn an agent, send text to stdin, read stdout lines, kill it.

---

## Phase 2: Core — Session & Shell Commands

**Goal:** Session management and built-in shell commands.

| # | Task | Details |
|---|------|---------|
| 2.1 | SessionStore | In-memory map + BoltDB persistence; CRUD operations |
| 2.2 | WorkspaceRegistry | Discover workspaces from config paths, expand `/*` |
| 2.3 | AgentRegistry | Scan `~/.agents_manager/agents/`, register discovered agents |
| 2.4 | SkillRegistry | Scan `~/.agents_manager/skills/` + workspace `.agents/skills/` + agent `skills/` |
| 2.5 | Shell command router | Parse input, match built-in keywords, dispatch to handler or agent |
| 2.6 | Built-in commands | `ls`, `cd`, `pwd`, `ps`, `use`, `agents`, `os`, `kill`, `history`, `help` |
| 2.7 | RequestRouter | Wire shell commands → agent pool for non-built-in input |

**Deliverable:** Can `ls` workspaces/scripts, `cd` between workspaces, `use` different agents, `ps` running agents.

---

---

## Phase 3: Protocol — HTTP + WebSocket + SSE

**Goal:** Serve the Control Centre SPA. WebSocket provides a browser-based shell terminal (stdin/stdout over WS, not chat). SSE streams dashboard updates.

| # | Task | Details |
|---|------|---------|
| 3.1 | HTTP server | Listen on `:8080`, serve static files from `~/.agents_manager/web/` |
| 3.2 | WebSocket endpoint | `/shell` — upgrade to WS, create session, route text frames to shell router (same stdin/stdout model as SSH, but over WS) |
| 3.3 | SSE endpoint | `/sse/dashboard` — stream agent status/progress events |
| 3.4 | REST API | `/api/sessions`, `/api/workspaces`, `/api/agents` — JSON read-only |

**Deliverable:** Browser loads SPA; `/shell` WebSocket simulates a terminal — input goes to the shell router, output streams back line-by-line. Dashboard updates via SSE.

---

## Phase 4: Authentication

**Goal:** Secure all endpoints with proper token lifecycle.

| # | Task | Details |
|---|------|---------|
| 4.1 | Password store | bcrypt hash in `~/.agents_manager/passwd`; first-run wizard sets password |
| 4.2 | Login endpoint | Validate password → issue access JWT (1h) + refresh JWT (30d) |
| 4.3 | Auth endpoints | `POST /auth/login`, `POST /auth/refresh` |
| 4.4 | Token middleware | Validate Bearer token on WS, SSE, and API endpoints |
| 4.5 | Rate limiting | Per-IP rate limit on login; account lockout after 5 failures |

**Deliverable:** Unauthenticated requests rejected; login → token → access all endpoints.

---

## Phase 5: Control Centre SPA

**Goal:** Browser-based control panel with dashboard and shell terminal.

| # | Task | Details |
|---|------|---------|
| 5.1 | Static HTML shell | Single `index.html` with two-panel layout |
| 5.2 | Login page | Password form → `/auth/login` → store tokens in cookie |
| 5.3 | Dashboard panel | SSE connection to `/sse/dashboard`; renders session cards with status/progress |
| 5.4 | Shell terminal panel | WebSocket to `/shell`; browser-based terminal emulating stdin/stdout with scrollback; runs the same shell router as SSH |
| 5.5 | Top bar | Current workspace + agent selector; clickable to switch |
| 5.6 | Token auto-refresh | Refresh access token before expiry; transparent to user |

**Deliverable:** Full Control Centre — dashboard + browser-based shell terminal — usable from any device.

---

## Phase 6: Protocol — SSH Server (Lower Priority)

**Goal:** Accept SSH connections for low-end terminal-only devices.

| # | Task | Details |
|---|------|---------|
| 6.1 | SSH server | Listen on `:2222`, accept connections |
| 6.2 | Public key auth | Verify against `authorized_keys` |
| 6.3 | Password auth | Verify bcrypt hash, enforce strength rules, rate-limit |
| 6.4 | Session per connection | Each SSH conn creates a session; stdin → shell router, stdout ← ResponseEvent stream |
| 6.5 | PTY / terminal | Raw terminal mode for proper CLI experience |

**Deliverable:** `ssh user@host -p 2222` → shell prompt, built-in commands work, agent requests work.

---

## Phase 7: Agent Shell Scripts (Low Priority)

**Goal:** `.ash` script execution via `agents_manager shell` mode.

| # | Task | Details |
|---|------|---------|
| 8.1 | Shell mode | `agents_manager shell` — bash with Manager built-ins loaded |
| 8.2 | `./script.ash` execution | Discover and run `.ash` files; `os` prefix for OS commands |
| 8.3 | Script discovery | `ls` lists `.ash` files; `~/.agents_manager/scripts/` + workspace `.agents/scripts/` |

**Deliverable:** `./deploy.ash` runs a bash script with Manager commands and OS commands.

---

## Phase 8: Polish

**Goal:** Production hardening.

| # | Task | Details |
|---|------|---------|
| 9.1 | Crash recovery | Sessions survive restart via BoltDB persistence |
| 9.2 | Multi-device sync | Same session state visible from SSH + WebSocket simultaneously |
| 9.3 | Graceful shutdown | SIGTERM → finish active requests → save state → exit |
| 9.4 | Resource limits | Configurable memory/CPU limits per agent process |
| 9.5 | Logging & metrics | Structured logs; optional Prometheus metrics endpoint |
| 9.6 | Packaging | Single binary distribution; `go build` with embedded static files |

**Deliverable:** Production-ready single binary.

---

## Dependency Graph

```
Phase 0 ──► Phase 1 ──► Phase 2 ──► Phase 3 (HTTP/WS/SSE)
                                  ├──► Phase 4 (Auth)
                                  └──► Phase 6 (SSH)
                                       Phase 5 (SPA, depends on 3 + 4)
                                       Phase 7 (.ash scripts, depends on 2)
                                       Phase 8 (Polish, depends on everything)
```

Phases 3, 4, and 6 can run in parallel after Phase 2. Phase 5 (SPA) requires 3 + 4. SSH (Phase 6) is lower priority.
