# Agents Manager — Technical Design

## 1. Tech Stack

| Layer              | Choice                   | Rationale                                              |
| ------------------ | ------------------------ | ------------------------------------------------------ |
| Runtime            | Go                      | Single binary, zero-dependency deployment, strong concurrency |
| Process management | `os/exec` + goroutines  | Native OS process spawning, stdio piping, signal handling |
| SSH server         | `golang.org/x/crypto/ssh` | Built-in Go SSH server; no external dependency        |
| WebSocket          | `gorilla/websocket`     | Standard Go WebSocket library                          |
| HTTP / SSE         | `net/http` (stdlib)     | Serve Control Centre SPA + SSE endpoint                |
| Persistence        | BoltDB (embedded)       | Single-file embedded KV store; no external DB process  |
| Frontend (SPA)     | Plain HTML + JS or htmx | Minimal; no build step needed; serve as static files   |

## 2. Component Architecture

```
                          ┌────────────────────────────────────────┐
  Terminal ─── SSH ──────►│                                        │
                          │  ┌──────────────────────────────────┐  │
  Browser ──── WS/SSE ───►│  │     Protocol Wrappers             │  │
  (Control                 │  │  ┌──────────┐  ┌─────────────┐  │  │
   Centre)                │  │  │   SSH    │  │  WebSocket  │  │  │
                          │  │  │  Server  │  │  / SSE      │  │  │
                          │  │  └────┬─────┘  └──────┬──────┘  │  │
                          │  └───────┼────────────────┼────────┘  │
                          │          │                │           │
                          │          ▼                ▼           │
                          │  ┌──────────────────────────────────────┐   │
                          │  │           Agent Manager Core         │   │
                          │  │  ┌──────────┐  ┌──────────────────┐ │   │
                          │  │  │ Request  │  │   Agent Pool     │ │   │
                          │  │  │  Router  │──│   (os/exec)      │ │   │
                          │  │  └────┬─────┘  └────────┬─────────┘ │   │
                          │  │       │                  │           │   │
                          │  │       ▼                  ▼           │   │
                          │  │  ┌──────────┐  ┌──────────────────┐ │   │
                          │  │  │ Session  │  │  Workspace       │ │   │
                          │  │  │  Store   │  │  Registry        │ │   │
                          │  │  └──────────┘  └──────────────────┘ │   │
                          │  │  ┌──────────┐  ┌──────────────────┐ │   │
                          │  │  │  Skill   │  │  Sub-Agent       │ │   │
                          │  │  │ Registry │  │  Coordinator     │ │   │
                          │  │  └──────────┘  └──────────────────┘ │   │
                          │  └──────────────────────────────────────┘   │
                          │                                       │
                          │  ┌────────────────────────────────┐   │
                          │  │  Control Centre (static files)  │   │
                          │  │  Served by HTTP / SSE endpoint  │   │
                          │  └────────────────────────────────┘   │
                          │                                       │
                          └────────────────────────────────────────┘
```

### 2.1 Core — Agent Manager Core

The core owns the process execution model. It is transport-agnostic and provider-agnostic — all agents, OS commands, and interactive sessions are process instances communicating via stdin/stdout (pipe mode) or PTY (interactive mode).

```
ProcessManagerCore {
    workspaces  WorkspaceRegistry       // manages workspaces (project dirs)
    agents      AgentRegistry           // manages registered agents (by provider)
    pool        ProcessInstancePool     // manages OS processes (pipe mode + PTY mode)
    router      RequestRouter           // classifies + dispatches requests
    store       SessionStore            // tracks sessions, persists state
    skills      SkillRegistry           // catalog of skills across all scopes
}

core.Submit(sessionID, requestText) -> stream of ResponseEvents
core.SubmitInteractive(sessionID, binary, args, dir) -> stream of ResponseEvents (PTY mode)
```

#### Model Hierarchy

Agents (by provider) and Workspaces are parallel top-level registries. A session binds them together. The Core provides discovery (agents, skills) to the terminal; the user instructs the agent on what to use.

```
Top Level
  ├── Agent: copilot           (provider: GitHub Copilot CLI)
  │   └── Agent-scoped skills
  ├── Agent: claude-code       (provider: Anthropic Claude Code CLI)
  │   └── Agent-scoped skills
  ├── Agent: opencode          (provider: opencode CLI)
  │   └── Agent-scoped skills
  │
  ├── Workspace: myapp         (path: ~/projects/myapp)
  │   └── Workspace-scoped skills     (project-specific tools)
  ├── Workspace: other-project (path: ~/projects/other)
  │   └── ...
  │
  └── Global skills                  (available everywhere)
```

A **session** is a long-lived connection at the Agents Manager level — one per terminal (SSH) or browser tab (WebSocket). This is a single-user system; the session does not leave and rejoin. Within a session, the user can switch workspace, agent, and specify sub-agents/skills at any time without establishing a new session.

Skills and sub-agents are resolved by merging three scopes: **agent → workspace → global**, with agent-scoped taking precedence.

#### Agent Registry

Agents are discovered automatically by scanning `~/.agents_manager/agents/`. Each sub-directory or executable found is registered as an agent.

```
AgentRegistry {
    agents  map[string]AgentRegistration   // keyed by agent ID
}

AgentRegistration {
    id          string                  // discovered name (e.g., "claude-code", "opencode")
    binary      string                  // discovered CLI binary path
    config      map[string]string       // optional: env vars from .env file in agent dir
    skills      []SkillBinding          // from agent dir's skills/ subdir
}
```

Agents are discovered at startup. Any workspace can use any discovered agent. Switch via `use <agent>`.

#### Workspace Registry

Workspaces are discovered from the config's workspace paths. A path ending in `/*` means each sub-directory is treated as a separate workspace.

```
WorkspaceRegistry {
    workspaces  map[string]Workspace   // keyed by workspace ID
    Discover(cfgPaths []string)        // scans paths, expands /* into sub-dir workspaces
}

Workspace {
    id          string                 // derived from directory name
    path        string                 // filesystem path
    skills      []SkillBinding         // from workspace .agents/skills/ subdir
}
```

#### Skill Registry

Skills are discovered automatically from three locations, scoped as: agent > workspace > global.

```
SkillRegistry {
    // Global skills: discovered from ~/.agents_manager/skills/
    globalSkills  []Skill

    // Resolution merges all three scopes
    GetAvailableSkills(workspaceID, agentID) -> []Skill
}

Skill {
    name        string              // derived from binary filename
    binary      string              // discovered path
    description string              // from accompanying .desc file or binary --help
}
```

Skills are CLI binaries discovered from:
- `~/.agents_manager/skills/` — global, available everywhere
- `<workspace>/.agents/skills/` — workspace-scoped
- `<agent_dir>/skills/` — agent-scoped

#### CLI Agent Integration Model

All agents are CLI-based. The Core treats every binary the same regardless of provider:

| Concern                 | How the Core Handles It                                                      |
| ----------------------- | ---------------------------------------------------------------------------- |
| **Provider diversity**  | Any binary on `$PATH` is a valid agent; registered once, available to all workspaces |
| **Session**             | One long-lived session per terminal connection; workspace and agent switchable at any time |
| **Workspace switching** | `cd <workspace>` — changes the logical workspace context; agent inherits the terminal's OS working directory |
| **Agent switching**     | `use <agent>` — switches the current agent provider for subsequent requests |
| **Communication**       | stdin (request) → process → stdout (response), stderr (diagnostics) — pipe mode or PTY mode for interactive sessions |
| **Output streaming**    | All process output is streamed raw (no line-buffering) — ANSI escape sequences pass through untouched for PTY-backed sessions |
| **Skills**              | `skills` command lists available skills for current workspace + agent + global; user instructs the agent to use specific skills |
| **Sub-agents**          | Agent binaries can spawn their own sub-processes; the Core does not intermediate. The `skills` command provides discovery so the user can give proper instructions |

#### Session Commands (Shell Model)

The Agents Manager acts like a shell: it intercepts a small set of built-in keywords and executes them directly. Any input that does not match a built-in command is forwarded to the current agent as a request.

| Command              | Action                                              |
| -------------------- | --------------------------------------------------- |
| `ls`                 | List workspaces and available `.ash` scripts          |
| `cd <workspace>`     | Switch workspace by ID                                |
| `pwd`                | Print current workspace path                          |
| `ps`                 | List active sessions and their running agents          |
| `kill <session_id>`  | Kill a running agent (SIGTERM → SIGKILL)               |
| `use <agent>`        | Switch to a different agent provider                  |
| `agents`             | List available agents                               |
| `skills`             | List available skills (agent + workspace + global)   |
| `os <command>`       | Execute a host OS command                              |
| `history`            | Show recent requests in this session                  |
| `help`               | Show available built-in commands                      |
| `./<script>.ash`     | Execute an Agent Shell script *(low priority)*         |

All other input is treated as an agent instruction and routed to the current agent for execution.

#### Agent Shell Scripts (`.ash`) *(low priority)*

The Manager provides a shell mode (`agents_manager shell`) that extends bash with Manager built-ins and an `agent` command for AI instructions. This means `.ash` scripts are standard bash scripts — no custom interpreter needed.

```
#!/usr/bin/env agents_manager shell
```

**Three execution contexts, all explicit:**

| Context                | Syntax                      | Example                     | Handled by            |
| ---------------------- | --------------------------- | --------------------------- | --------------------- |
| Manager built-in       | bare keyword                | `ls`, `cd myapp`, `use claude` | Manager              |
| Agent instruction      | anything else               | `build a login page`        | Manager → AI Agent    |
| Host OS command        | `os <command>`              | `os npm run build`          | Host bash             |

No shadowing: `ls` always lists workspaces; `os ls` lists files. The separation is explicit at the session level.

Because the separation is handled at the Manager level, bash handles flow control (`if`, `for`, `&&`, `||`, pipes, redirects, variables) natively. No custom scripting language.

**Discovery:**
- Global: `~/.agents_manager/scripts/*.ash`
- Workspace-local: `<workspace>/.agents/scripts/*.ash`
- `ls` lists both workspaces and reachable `.ash` files as executable entries

**Example `deploy.ash` (standard bash):**
```bash
#!/usr/bin/env agents_manager shell
cd backend
os npm run build
os npm test
cd ../frontend
os npm run build
review the backend changes and check for regressions
if [ $? -eq 0 ]; then
  os git push origin staging
  summarize the deployment
fi
```

> `.ash` is just bash with Manager extensions. The execution engine (loading and running `.ash` files) is low priority; the shell mode itself is the foundation.

### 2.2 Wrappers — Protocol Layer

Wrappers are thin adapters. They own nothing except connection state.

| Wrapper              | Accepts           | Feeds to Core                   | Returns to Client              |
| -------------------- | ----------------- | ------------------------------- | ------------------------------ |
| **SSH Server**       | stdin (text)      | `core.Submit(session, text)`    | stdout (streamed Event text)   |
| **WebSocket Server** | WS frame (text)   | `core.Submit(session, text)`    | WS frame (streamed Event text) |
| **SSE Endpoint**     | HTTP GET (session)| `core.Subscribe(session)`       | SSE event stream               |

### 2.3 Frontend — Control Centre SPA

The Control Centre is a single-page application served as static files from the Agents Manager process.

**Design decisions:**

| Decision              | Choice                     | Rationale                                     |
| --------------------- | -------------------------- | --------------------------------------------- |
| Framework             | Plain HTML + JS, or htmx   | No build step, no npm; served as static files |
| Bundling              | None (just `.html`, `.js`) | Keep it trivial for 1-2 users                 |
| Shell terminal transport | WebSocket                  | `ws://<host>:8080/shell` — browser-based terminal, same stdin/stdout model as SSH |
| Dashboard refresh     | SSE                        | Live updates; reuse existing SSE infrastructure |
| SSE fallback          | Use SSE for the CLI panel if WebSocket blocked | Graceful degradation              |
| Styling               | Minimal CSS, terminal-like monospace | CLI aesthetic in browser           |

**Layout:**

```
┌──────────────────────────────────────────────────────────────┐
│  Control Centre                         [ws: myapp] [claude] │
├──────────────────────────┬───────────────────────────────────┤
│  📊 Dashboard            │  💬 Shell Terminal                │
│                          │                                    │
│  a1b2c3  running         │  agent> Type a request...          │
│  Step 2/4: tests         │  > build login page                │
│                          │  agent> Got it. Building...        │
│  d4e5f6  waiting         │  agent> Plan: 1) ...              │
│  queued position 1       │  agent> [done] ...                │
│                          │                                    │
│                          │  ls  cd  ps  use  agents  history      │
└──────────────────────────┴───────────────────────────────────┘
```

- **Top bar**: current workspace and agent — clickable to switch
- **Left panel**: Dashboard — SSE stream from `/sse/dashboard` for live updates
- **Right panel**: Shell Terminal — WebSocket to `/shell`; emulates stdin/stdout terminal in browser
- Command bar at bottom provides quick access to `/workspace`, `/use`, `!skills`, `!spawn`

## 3. Authentication

The Agents Manager runs on the user's working machine. Authentication is designed for a single-user or small-team (1-2 people) trusted environment — not multi-tenant SaaS.

### 3.1 Per-Protocol Model

| Protocol            | Method                              | Rationale                                         |
| ------------------- | ----------------------------------- | ------------------------------------------------- |
| **SSH**             | Public key + password auth          | Key preferred; password requires minimum 12 chars, mixed case, digits |
| **WebSocket / SSE** | Bearer token in query string        | `ws://host:8080/shell?token=<access_token>`         |
| **Control Centre**  | Login page → cookie/JWT             | User logs in with password; token stored in cookie  |
| **Dashboard API**   | Bearer token (Authorization header) | Same access token as WS/SSE                       |

### 3.2 Token Lifecycle

Standard access/refresh token pattern. User password is set on first run.

- `POST /auth/login` → access token (1h) + refresh token (30d)
- `POST /auth/refresh` → new access token
- Tokens carried as Bearer in WS query string, SSE header, or HTTP-only cookie
- Password change invalidates all issued tokens

### 3.3 Network Binding

| Service             | Default bind    |
| ------------------- | --------------- |
| SSH                 | `0.0.0.0:2222`  |
| HTTP + WS + SSE     | `0.0.0.0:8080`  |

### 3.4 Threat Model

| Threat                     | Mitigation                                          |
| -------------------------- | --------------------------------------------------- |
| Unauthorized SSH access    | Public key auth preferred; password auth requires strong password (min 12 chars, mixed case, digits); rate-limited |
| Stolen access token        | Short expiry (1 hour); refresh token stored server-side; revocable by password change |
| Brute-force login          | Rate-limited per IP; account lockout after 5 failed attempts                   |
| Token in process list      | Tokens passed via environment or cookie, not CLI args                          |

## 4. Core Data Model

```
// Session is the single long-lived connection for the user.
// Workspace and agent are mutable — the user switches them within the session.
Session {
    id          string       // uuid
    transport   "ssh"|"ws"   // which wrapper owns this session
    workspaceID string       // current workspace (mutable: /workspace <id>)
    agentID     string       // current agent (mutable: /use <id>)
    state       "idle"|"running"    // whether an agent is currently executing
    activePID   int|null     // OS process ID of the currently running agent, if any
    history     []Request    // past requests in this session
    createdAt   time
    updatedAt   time
}

Request {
    id          string
    text        string
    workspaceID string       // snapshot of workspace at request time
    agentID     string       // snapshot of agent at request time
    startedAt   time
    completedAt time|null
}

ResponseEvent {
    sessionID  string
    type       "ack"|"progress"|"deliverable"|"error"|"skill_catalog"
    text       string       // human-readable output line
    data       json|null    // structured payload for skill catalog
    timestamp  time
}
```

The core is responsible for routing `ResponseEvent` values back to the correct session's transport wrapper. The wrapper then formats the event for its protocol (raw text for SSH, JSON frame for WebSocket, SSE event for SSE).

## 5. Process Instance Pool Design

All process types (OS commands, non-interactive agents, interactive agents) share a unified **Process Instance Pool**. Each instance is tracked by session ID and OS PID.

```
ProcessInstancePool {
    max       int                   // max concurrent processes (default 4)
    active    map[string]*Agent     // running processes by session ID
    byPID     map[int]*Agent        // running processes by OS PID
    queue     []QueuedRequest       // FIFO wait queue when pool is full
}

Agent {
    sessionID  string               // owning session
    pid        int                  // OS process ID
    binary     string               // resolved binary path
    cmd        *exec.Cmd            // os/exec command handle
    mode       ProcessMode          // "pipe" | "pty"
    state      "running"|"done"|"error"|"killed"
    startedAt  time
    stdoutBuf  *bytes.Buffer        // raw byte buffer of stdout (or merged PTY output)
    stderrBuf  *bytes.Buffer        // raw byte buffer of stderr (pipe mode only)
}
```

| Process Type | IO Plumbing | stdin Lifetime | Output Format |
|---|---|---|---|
| OS command | Pipe (stdin/stdout/stderr) | Open for interaction | Raw byte stream |
| Non-interactive agent | Pipe (stdin/stdout/stderr) | Open for interaction | Raw byte stream |
| Interactive agent | PTY (merged stdio) | Open for interaction | Raw ANSI stream |

Key behaviors:
- **Spawn**: For pipe mode, `fork/exec` with separate stdin/stdout/stderr pipes. For PTY mode, allocate a PTY via `creack/pty` and attach all stdio to the PTY slave.
- **Monitor**: goroutine reads process output in raw 4096-byte chunks, wraps as `ResponseEvent` with both `Text` (string) and `Data` (raw bytes), sends to core for routing. No line-buffering or text framing — ANSI sequences pass through untouched.
- **Limit**: if `len(active) >= max`, queue the request. Notify the client of queue position.
- **Kill**: `SIGTERM` → wait 5s → SIGKILL. Close PTY if interactive. Emit error event.
- **Reap**: on exit, collect exit code, flush remaining buffer, free slot. Next queued request spawns.
- **Stdin injection**: `POST /api/process/{pid}/stdin` writes input string to the target process's stdin, supporting both pipe and PTY modes.

## 6. Configuration

```yaml
# agents_manager.yaml
auth:
  password_file: ~/.agents_manager/passwd    # bcrypt hash; set on first run
  access_token_expiry: 3600                  # 1 hour
  refresh_token_expiry: 2592000              # 30 days
  max_login_attempts: 5
  lockout_duration: 300                      # 5 minutes

ssh:
  port: 2222
  bind: 0.0.0.0
  host_key_path: ~/.agents_manager/ssh_host_key
  authorized_keys_path: ~/.ssh/authorized_keys
  password_auth: true
  password_min_length: 12
  password_require_mixed_case: true
  password_require_digit: true
  password_max_attempts: 3

http:
  port: 8080
  bind: 0.0.0.0
  static_dir: ~/.agents_manager/web     # Control Centre SPA

  # Endpoints on same port:
  #   /         → Control Centre SPA
  #   /shell    → WebSocket upgrade (browser terminal)
  #   /sse/*    → SSE streams
  #   /api/*    → REST API
  #   /auth/*   → Login / refresh

agent_pool:
  max_instances: 4
  stdout_buffer_size: 1048576   # 1MB per agent, in RAM

# Workspaces: list of directories. "xx/*" means each sub-directory is a workspace.
workspaces:
  - ~/projects/*
  - ~/work/myapp

# All skills, agents, and scripts are self-discovered from the home directory.
# No manual agent/skill registration needed.
home: ~/.agents_manager          # agents/  skills/  scripts/  discovered automatically

store:
  path: ~/.agents_manager/sessions.db   # BoltDB file
```
