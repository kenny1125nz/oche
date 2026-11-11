# Process Management Redesign — Parallel Implementation

## Objective

Build a new unified process management system from scratch in separate packages, running alongside the existing implementation. Existing code serves as reference for interfaces, edge cases, and integration points — not as a foundation to modify.

## Key Problems

1. **Dual process management** — OS commands bypass Pool entirely, missing queue limits, uniform lifecycle, and `ActiveList` tracking.

2. **N+1 WebSocket connections** — Every PTY-mode pool agent requires a separate `/ws/agent/<pid>` connection. The frontend must manage multiple WS connections, reconnection logic, and state synchronization.

3. **Scattered I/O routing** — Three different code paths for sending input (WriteOSInput, WriteStdin, WriteStdinRaw) and resize (ResizeOSProcess, Resize) depending on process type, checked by ad-hoc conditionals in `ws.go`.

4. **`Agent` is a misnomer** — The struct represents any child process (LLM agent binary, REPL, `sh -c "ls"`). The name leaks abstraction into the Pool interface and creates confusion.

5. **Subscribe/broadcast exists only for per-process WS** — The pub/sub mechanism (`SubscribeAgent`, `UnsubscribeAgent`, `broadcast`) has no other consumer. If the per-process WS goes away, so does the complexity.

6. **REPL dance** — `cmdRepl` submits a PTY-mode agent to the Pool, then the frontend must discover the PID and open a `/ws/agent/<pid>` connection for interactive I/O. Two round-trips and a second WS for what should be a single operation.

---

## Design: Unified Process Manager

### Core idea

All processes (agent calls, OS commands, REPLs, long-running TUI apps) are managed uniformly by a single process manager. A single WebSocket per session replaces the N+1 topology. The terminal on the client side always shows one process at a time, with plumbing that can be rewired between processes.

### Relationship to existing code

The existing implementation (`pool.Pool`, `Router.osProcesses`, `ws.go`, `stream.go`, `forward.go`) is left completely untouched. The new implementation lives in its own package tree (e.g., `internal/process/`, `internal/ws2/`) and is wired in behind a feature flag or separate HTTP route. Once the new system is feature-complete and stable, the old code paths are removed in a single cleanup pass — no incremental refactoring of existing code.

Existing code that is useful as **reference**:
- `pool/agent.go` — Process spawning, PTY setup, terminal resize API
- `shell/commands.go` `cmdOs` — How OS commands are currently dispatched
- `ws/ws.go` — WebSocket read/write loop patterns, message protocol
- `router/router.go` — Command routing and session tracking

### Process execution modes

Every spawn request has two orthogonal axes: **plumbing** and **persistence**.

**Plumbing — how I/O is connected:**

- **Sync** — The process's I/O is wired straight to the client terminal. While it runs, the terminal is occupied. On exit, the terminal returns to the command prompt. This is the default for agent calls, `os` commands, and REPL. Analogous to running a command normally in bash.
- **Async** — The process starts in the background and the terminal immediately returns to the command prompt. The VT parser maintains the screen state for later `attach`. Analogous to `command &` in bash.

**Persistence — what happens on client disconnect:**

- **Normal** — The process is killed (SIGTERM → SIGKILL after grace period) when the client disconnects.
- **Nohup** — The process survives client disconnection. It can be re-attached later from the same or a new session via `attach`. A nohup sync process still occupies the terminal while the client is connected; if the client disconnects, the process continues running in the background.

The two axes combine freely:

| Plumbing | Persistence | Disconnect behavior                       | Client reconnects          |
| -------- | ----------- | ----------------------------------------- | -------------------------- |
| Sync     | Normal      | Killed                                    | — (gone)                   |
| Sync     | Nohup       | Continues, VT parser keeps screen current | `attach <pid>` to reattach |
| Async    | Normal      | Killed                                    | — (gone)                   |
| Async    | Nohup       | Continues, VT parser keeps screen current | `attach <pid>` to reattach |

### Plumbing and focus

The client has one terminal view. At any moment it is plumbed to at most one process. The session switch is the hub that rewires the WebSocket to the target process's I/O:

```
                      ┌──────────────────┐
  WebSocket (client)──│  Session Switch  │── process A (sync)
                      │  (plumbing hub)  │── process B (async)
                      │                  │── process C (async, nohup)
                      └──────────────────┘
```

- **Sync** processes own the terminal until they exit. If marked nohup, they survive client disconnect (VT parser keeps screen current, re-attachable via `attach`).
- **Async** processes are spawned detached. The client can `attach <pid>` to re-plumb the terminal to that process. On attach, the current screen is snapshot from the VT parser and sent to the client; then live I/O takes over.

### The `attach` command

`attach <pid>` rewires the session's plumbing to point at the given process. The client includes its terminal dimensions: `attach <pid>` sends `{"attach":123,"rows":24,"cols":80}`.

1. The current plumbing (if any) is detached — if it was a sync process, it is pushed to background.
2. The target process's current screen state is read from the VT parser (screen buffer, cursor position, terminal modes), serialized as escape sequences, and sent to the client as an `init` frame. This reproduces the terminal display exactly as it was — no raw byte replay needed.
3. The PTY is resized to the client's dimensions. Any repaint from the process arrives as live output.
4. Live I/O is wired: subsequent client keystrokes go to the process's stdin, and new output streams to the client in real time, continuing to feed the VT parser.

### Switching away from a looked process

Once a user has `attach`-ed to an async process, they need a way to return to the command prompt without killing the process. This is done via a client-side key combination that the client intercepts rather than sending to the process's stdin. `Ctrl+O` is recommended — it is not bound in common TUIs (vim, less, htop, tmux), is not a terminal control character (XON/XOFF is `Ctrl+Q`/`Ctrl+S`), and readline uses it only for an obscure line-editing function that is irrelevant in raw PTY mode. The client sends a control frame over the WebSocket:

```
{ "type": "detach" }
```

The backend responds by:
1. Detaching the current process's terminal I/O (the process continues running, VT parser keeps the screen state current).
2. Returning the session to the command prompt.

This is analogous to `Ctrl+Z` in bash (suspend and return to shell), except the process keeps running rather than stopping.

### Session model

The application is single-user. There is at most one active WebSocket session at any time. A session is identified by a server-generated ID, but there is no multi-tenant session store — the session ID is purely for correlating logs and process ownership.

If a second browser instance connects while one is already active:

1. The server sends a prompt to the new client: *"Another session is active. Kick it out and take over? (Y/N)"*
2. If confirmed, the old WebSocket connection is closed. Any nohup processes continue running; sync processes without nohup are killed.
3. If declined, the new connection is closed.

This avoids silent session conflicts while keeping the backend stateless with respect to sessions.

| Scenario                                            | Behavior                                                                             |
| --------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Client disconnects, normal process (sync or async)  | Process is killed (SIGTERM → SIGKILL after grace period)                             |
| Client disconnects, nohup process (sync or async)   | Process continues; VT parser keeps screen current; process shows in `ActiveList`     |
| Client reconnects (new connection after disconnect) | No automatic reattachment; user lists processes and `attach <pid>` to resume         |
| Second client connects while one is active          | Prompt to kick existing session; on confirm, old WS is closed and new one takes over |

### What the new design eliminates

- **One WebSocket** — No `/ws/agent/<pid>` per-process connections. The client opens one WS and all I/O flows through it.
- **No subscribe/broadcast** — The pub/sub mechanism existed only to feed the per-process WS. The new design has no such mechanism.
- **No `Router.osProcesses`** — OS commands go through the new ProcessManager like everything else, using the same spawn/monitor/terminate lifecycle.
- **No three-way input routing** — Input goes to whichever process is currently plumbed to the terminal, or is treated as a command if no process is active.
- **`Agent` renamed to `Process`** — The struct represents any child process, not just LLM agents.

### Key concepts for the new backend

- The ProcessManager tracks which process (if any) is currently plumbed to each session's terminal.
- Every process's PTY output is fed through a VT parser that maintains the current screen state (cell buffer + cursor + terminal modes). On `attach`, the screen state is serialized as escape sequences and sent to the client — no raw byte replay needed.
- The `attach` command snapshots the VT parser's screen state to the client, then wires live I/O.
- The `detach` control frame reverses this: live I/O is disconnected, the process continues running in the background (PTY output keeps feeding the VT parser), and the terminal returns to accepting commands.

## Technical details

### Plumbing switch mechanics

The session's WebSocket writer goroutine reads from an `activeCh` that can be swapped at runtime:

```
ws reader goroutine          ws writer goroutine
┌─────────────────┐          ┌──────────────────────┐
│ read from WS     │          │ select {             │
│ detect Ctrl+O   │─────────→│   case evt ←activeCh │
│ send "detach"    │  signal  │   case newCh ←ctrlCh │
│ detect "/attach" │          │ }                    │
└─────────────────┘          └──────────────────────┘
                                    │
                          ┌─────────┴──────────┐
                          │   activeCh points   │
                          │   to one of:        │
                          │   • process output  │
                          │   • command prompt  │
                          └────────────────────┘
```

When a swap is requested (attach, detach, process exit):
1. The writer receives a new channel on `ctrlCh`.
2. It stops reading from the old `activeCh` (discarding remaining events).
3. It sets `activeCh = newCh` and resumes reading.

For `attach`: the old process's output channel is discarded (the process is now backgrounded, VT parser keeps running). The new channel is the target process's output channel. Before live output, the init frame (screen snapshot) is sent.

For `detach`: the current process's output channel is discarded (process continues, VT parser stays active). A new channel is used that signals "ready for commands" — subsequent user input is dispatched as commands, not stdin.

### WebSocket message protocol

**Client → Server:**

| Message                               | When                                 | Effect                                                                   |
| ------------------------------------- | ------------------------------------ | ------------------------------------------------------------------------ |
| `{"input":"ls -la"}`                  | No process active                    | Treated as command, executed via router                                  |
| `{"input":"some text"}`               | Process active (sync or looked)      | Text written to process stdin                                            |
| `{"input":"\x0f"}`                    | Process active                       | `Ctrl+O` (0x0f) intercepted client-side, sends `{"detach":true}` instead |
| `{"attach":123,"rows":24,"cols":80}`  | Any time                             | Switch plumbing to process PID 123 with client's terminal size           |
| `{"resize":true,"rows":24,"cols":80}` | Process active                       | Forward to process's PTY                                                 |
| `{"kick":true}`                       | Prompted when another session exists | Confirm kick, close old session                                          |

**Server → Client:**

| Message                                                       | When                                | Effect                                                                  |
| ------------------------------------------------------------- | ----------------------------------- | ----------------------------------------------------------------------- |
| `{"type":"init","screen":"...","modes":{...},"process_id":N}` | After `attach` succeeds             | Screen snapshot from VT parser — client renders this before live output |
| `{"type":"ack","text":"..."}`                                 | Command accepted or process spawned | Acknowledgement                                                         |
| `{"type":"progress","text":"...","process_id":N}`             | Process output (after init)         | Content from the active process                                         |
| `{"type":"deliverable","text":"done","process_id":N}`         | Process exited                      | Terminal returns to command mode                                        |
| `{"type":"error","text":"..."}`                               | Error                               | Display error in terminal                                               |
| `{"type":"prompt","action":"kick"}`                           | Second session connects             | Ask user if they want to take over                                      |
| `{"type":"detached","process_id":N}`                          | Detach succeeded                    | Confirmation that process N is now backgrounded                         |

All `progress` events from processes carry a `process_id` so the client can correlate output even when plumbing switches.

### Screen state (VT parser)

Every process's PTY output is fed through a VT100/xterm parser that maintains a screen model in memory:

- **Cell buffer**: grid of `(char, foreground, background, attributes)` for the current visible screen
- **Cursor position**: row, column, visibility
- **Terminal modes**: DECCKM, bracketed paste, alternate screen, etc.
- **Scrollback**: not tracked initially (only visible screen is preserved)

The parser is byte-driven — it processes every escape sequence, control character, and printable byte the same way a terminal emulator would. This is well-understood code (~400-600 lines in practice).

#### On attach

The client includes its terminal dimensions in the attach request: `{"attach":123,"rows":24,"cols":80}`.

Ordering is critical — resize must happen **after** init to avoid repaint bytes corrupting the snapshot:

1. Read the current screen state from the VT parser (at the PTY's current dimensions).
2. Serialize it as a minimal sequence of escape commands:
   - Clear screen + reset attributes
   - For each row: set cursor position, write cell characters with SGR attributes
   - Apply terminal modes (DECCKM, bracketed paste, etc.)
3. Send as `{"type":"init","screen":"...","modes":{...}}` to the client.
4. Resize the PTY to the client's dimensions. The process may emit a repaint at the new size — this arrives as normal live output.
5. Switch to live I/O — subsequent PTY bytes are forwarded directly.

The result: the client sees the snapshot first (correct content at old dimensions), then the resize repaint corrects the dimensions without losing any screen state. No raw byte replay, no ring buffer, no TUI/non-TUI distinction.

#### Detach

When a sync process is detached (pushed to background), the VT parser continues to process the process's PTY output — the screen state stays current. On re-attach, the current screen is snapshot from the parser and sent fresh.

### Process lifecycle in the new ProcessManager

The new `ProcessManager` (in `internal/process/`) implements `spawn`/`monitor`/`terminate`/`attach`/`detach` from scratch. The existing Pool's lifecycle logic is a reference for the basic goroutine-per-process pattern.

- **spawn** — Accepts a plumbing parameter (sync/async). All processes have their PTY output wired through a VT parser that maintains the screen model. For sync, the live channel is set; for async, no live channel until attach.
- **monitor** — The goroutine reads from the process and pushes bytes through the VT parser. If a live channel is set (process is attached), bytes are also forwarded to it. For nohup processes, `monitor` is not cancelled on WebSocket disconnect — it runs until the process exits.
- **terminate** — SIGTERM → SIGKILL after grace period.
- **attach** — A live channel is installed on the process. The VT parser's current screen state is sent as an `init` frame before live output begins. Input is connected.
- **detach** — Live channel is removed; process continues, VT parser keeps ingesting output.

### Detach flow in detail

1. Client captures `Ctrl+O` keystroke (does not send to server as input).
2. Client sends `{"detach":true}` control frame.
3. Server receives frame on reader goroutine, signals writer goroutine via `ctrlCh`.
4. Writer swaps `activeCh`:
   - Current process's output channel is removed from the select.
   - The process's PTY output continues feeding the VT parser (screen stays current).
   - A fresh command-mode channel is installed as `activeCh`.
5. Server sends `{"type":"detached","process_id":N}` to client.
6. Client terminal shows the command prompt. Subsequent input is dispatched as commands.

---

## Implementation Roadmap

All phases build **new packages** alongside existing code. Existing files are never modified — they serve as reference for patterns, interfaces, and edge cases.

### Phase 1: VT parser (independent, lowest risk)

Write `internal/vt/parser.go` — VT100/xterm parser that maintains a screen model:

- Cell buffer: grid of `(char, fg, bg, attr)` with scroll on newline
- Cursor positioning (CUP, CUU/CUD/CUF/CUB, HVP, line feed, carriage return)
- SGR attributes (bold, dim, italic, underline, blink, reverse, 8/16/256/RGB colors)
- Alternate screen buffer (`\033[?1049h/l`)
- Terminal mode tracking (DECCKM, bracketed paste, cursor visibility, auto-wrap)
- Screen serialization: emit escape sequences to reproduce current screen state
- Tests: feed known sequences, verify screen state and serialization

**Reference**: existing PTY output handling in `pool/agent.go` monitor goroutine.

### Phase 2: Process manager core

Write `internal/process/manager.go` — the new unified process manager:

- Process struct (renamed from `Agent`) with spawn/monitor/terminate lifecycle
- Plumbing mode (sync/async) and persistence (normal/nohup) in spawn API
- PTY output fed through VT parser in monitor goroutines
- Focused-process tracking (which process is plumbed to which session)
- `ScreenSnapshot()` on Process (serialize VT parser state)
- Queue with max concurrency (modeled on existing Pool's queue)
- `ActiveList` for listing running processes

**Reference**: `pool/pool.go`, `pool/agent.go` for spawn/monitor/terminate patterns, queue, and tracking maps. `shell/commands.go` `cmdOs` for OS command dispatch patterns. `router/router.go` for session-process association.

### Phase 3: New WebSocket handler

Write `internal/ws2/handler.go` — new WebSocket handler with the single-WS plumbing model:

- Writer goroutine with `activeCh`/`ctrlCh` channel swapping
- Attach: init frame (screen snapshot from VT parser) → resize → live I/O
- Detach: plumbing swap, VT parser keeps running
- Input routing: single branch (plumbed process gets stdin; no process → dispatch command)
- No subscribe/broadcast, no per-process WS

**Reference**: `ws/ws.go` for read/write loop structure and message types. `ws/stream.go` and `ws/agenthub.go` for the per-process WS patterns (being replaced).

### Phase 4: Wire new handler (no old code removed yet)

- Register new WS handler on a separate route (e.g., `/shell2`)
- New frontend client connects to `/shell2` while old client continues using `/shell`
- Both implementations run side-by-side for testing and comparison
- Frontend: single-WS init, process list with `attach`, render `init` frames, `Ctrl+O` interception

**No existing code is removed or modified in this phase.**

### Phase 5: Cutover (single cleanup pass)

Once the new implementation is feature-complete and stable:

- Remove old WS handler, `wsAgentHandler`, `stream.go`, subscribe/broadcast, `/ws/agent/` route
- Remove `Router.osProcesses`, `WriteOSInput`, `ResizeOSProcess`, `forward.go`
- Remove old Pool's `cmdOs` path (now handled by new ProcessManager)
- Rename `/shell2` → `/shell`
- Delete old `pool/`, `ws/`, and related dead code

### Critical path

```
Phase 1 → Phase 2 → Phase 3 → Phase 4 → Phase 5

Each phase is independently testable alongside existing code.
```
