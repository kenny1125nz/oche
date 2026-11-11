# Process Manager

## Overview

The Process Manager is a unified system for managing all child processes — agent calls, OS commands, and long-running TUI applications. A single WebSocket per session handles all process I/O. The client terminal always shows one process at a time, with plumbing that can be rewired between processes.

## Process execution modes

Every spawn request has two orthogonal axes: **plumbing** and **persistence**.

### Plumbing — how I/O is connected

- **Sync** — The process's I/O is wired straight to the client terminal. While it runs, the terminal is occupied. On exit, the terminal returns to the command prompt. This is the default for agent calls and OS commands. Analogous to running a command normally in bash.
- **Async** — The process starts in the background and the terminal immediately returns to the command prompt. The VT parser maintains the screen state for later `attach`. Analogous to `command &` in bash.

### Persistence — what happens on client disconnect

- **Normal** — The process is killed (SIGTERM → SIGKILL after grace period) when the client disconnects.
- **Nohup** — The process survives client disconnection. It can be re-attached later from the same or a new session via `attach`. A nohup sync process still occupies the terminal while the client is connected; if the client disconnects, the process continues running in the background.

The two axes combine freely:

| Plumbing | Persistence | Disconnect behavior                       | Client reconnects          |
| -------- | ----------- | ----------------------------------------- | -------------------------- |
| Sync     | Normal      | Killed                                    | — (gone)                   |
| Sync     | Nohup       | Continues, VT parser keeps screen current | `attach <pid>` to reattach |
| Async    | Normal      | Killed                                    | — (gone)                   |
| Async    | Nohup       | Continues, VT parser keeps screen current | `attach <pid>` to reattach |

## Plumbing and focus

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

## The `attach` command

`attach <pid>` rewires the session's plumbing to point at the given process. The client includes its terminal dimensions: `attach <pid>` sends `{"attach":123,"rows":24,"cols":80}`.

1. The current plumbing (if any) is detached — if it was a sync process, it is pushed to background.
2. The target process's current screen state is read from the VT parser (screen buffer, cursor position, terminal modes), serialized as escape sequences, and sent to the client as an `init` frame. This reproduces the terminal display exactly as it was — no raw byte replay needed.
3. The PTY is resized to the client's dimensions. Any repaint from the process arrives as live output.
4. Live I/O is wired: subsequent client keystrokes go to the process's stdin, and new output streams to the client in real time, continuing to feed the VT parser.

## Switching away from a looked process

Once a user has `attach`-ed to an async process, they need a way to return to the command prompt without killing the process. This is done via a client-side key combination that the client intercepts rather than sending to the process's stdin. `Ctrl+O` is recommended — it is not bound in common TUIs (vim, less, htop, tmux), is not a terminal control character (XON/XOFF is `Ctrl+Q`/`Ctrl+S`), and readline uses it only for an obscure line-editing function that is irrelevant in raw PTY mode. The client sends a control frame over the WebSocket:

```
{ "type": "detach" }
```

The backend responds by:
1. Detaching the current process's terminal I/O (the process continues running, VT parser keeps the screen state current).
2. Returning the session to the command prompt.

This is analogous to `Ctrl+Z` in bash (suspend and return to shell), except the process keeps running rather than stopping.

## Session model

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

## WebSocket message protocol

### Client → Server

| Message                               | When                                 | Effect                                                                   |
| ------------------------------------- | ------------------------------------ | ------------------------------------------------------------------------ |
| `{"input":"ls -la"}`                  | No process active                    | Treated as command, executed via router                                  |
| `{"input":"some text"}`               | Process active (sync or looked)      | Text written to process stdin                                            |
| `{"input":"\x0f"}`                    | Process active                       | `Ctrl+O` (0x0f) intercepted client-side, sends `{"detach":true}` instead |
| `{"attach":123,"rows":24,"cols":80}`  | Any time                             | Switch plumbing to process PID 123 with client's terminal size           |
| `{"resize":true,"rows":24,"cols":80}` | Process active                       | Forward to process's PTY                                                 |
| `{"kick":true}`                       | Prompted when another session exists | Confirm kick, close old session                                          |

### Server → Client

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

## Plumbing switch mechanics

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

## Screen state (VT parser)

Every process's PTY output is fed through a VT100/xterm parser that maintains a screen model in memory:

- **Cell buffer**: grid of `(char, foreground, background, attributes)` for the current visible screen
- **Cursor position**: row, column, visibility
- **Terminal modes**: DECCKM, bracketed paste, alternate screen, etc.
- **Scrollback**: not tracked initially (only visible screen is preserved)

The parser is byte-driven — it processes every escape sequence, control character, and printable byte the same way a terminal emulator would.

### On attach

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

### Detach

When a sync process is detached (pushed to background), the VT parser continues to process the process's PTY output — the screen state stays current. On re-attach, the current screen is snapshot from the parser and sent fresh.

## Process lifecycle

The Process Manager implements `spawn`/`monitor`/`terminate`/`attach`/`detach`.

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

## Implementation Roadmap

### Phase 1: VT parser

Write `internal/vt/parser.go` — VT100/xterm parser that maintains a screen model:

- Cell buffer: grid of `(char, fg, bg, attr)` with scroll on newline
- Cursor positioning (CUP, CUU/CUD/CUF/CUB, HVP, line feed, carriage return)
- SGR attributes (bold, dim, italic, underline, blink, reverse, 8/16/256/RGB colors)
- Alternate screen buffer (`\033[?1049h/l`)
- Terminal mode tracking (DECCKM, bracketed paste, cursor visibility, auto-wrap)
- Screen serialization: emit escape sequences to reproduce current screen state
- Tests: feed known sequences, verify screen state and serialization

### Phase 2: Process manager core

Write `internal/process/manager.go` — the unified process manager:

- Process struct with spawn/monitor/terminate lifecycle
- Plumbing mode (sync/async) and persistence (normal/nohup) in spawn API
- PTY output fed through VT parser in monitor goroutines
- Focused-process tracking (which process is plumbed to which session)
- `ScreenSnapshot()` on Process (serialize VT parser state)
- Queue with max concurrency
- `ActiveList` for listing running processes

### Phase 3: WebSocket handler

Write `internal/ws2/handler.go` — WebSocket handler with the single-WS plumbing model:

- Writer goroutine with `activeCh`/`ctrlCh` channel swapping
- Attach: init frame (screen snapshot from VT parser) → resize → live I/O
- Detach: plumbing swap, VT parser keeps running
- Input routing: single branch (plumbed process gets stdin; no process → dispatch command)

### Phase 4: Wire handler

- Register WS handler on the shell route
- Frontend: single-WS init, process list with `attach`, render `init` frames, `Ctrl+O` interception

### Critical path

```
Phase 1 → Phase 2 → Phase 3 → Phase 4
```

Each phase is independently testable.
