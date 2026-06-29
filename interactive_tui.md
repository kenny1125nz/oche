# Interactive TUI Enhancement Design Document

This document describes the design enhancements needed to extend the application to support interactive operations between the browser-based terminal side and agent-managed process instances. It addresses three main challenges: (1) **ANSI terminal control escape sequences** to ensure the TUI is properly rendered in a browser GUI, (2) **multi-instance process management** via the Agent Manager, and (3) **typing latency** when the input stream traverses the internet.

## Design Goals

- **ANSI fidelity**: The full ANSI/VT100 stream must reach the browser terminal unmodified so TUI applications render correctly.
- **Unified pool**: All process types — OS commands, non-interactive agents, interactive agents — share the same process instance pool, lifecycle primitives, and streaming IO path.
- **Concurrency safety**: The pool's instance maps (keyed by PID and by session ID) must support concurrent access from multiple WebSocket handlers and REST API calls without data races.
- **Per-instance lifecycle**: Each process has independent cleanup — closing its WebSocket frees its PTY or pipes and kills the process without affecting other sessions in the pool.
- **Programmatic input**: The REST API for stdin injection allows the Agent Manager and external callers to deliver input blocks to any running interactive session, forming the basis of the latency mitigation strategy.

## 1. Architecture Concept: Streaming Process Bridge

The system acts as a transparent bridge between a browser-based terminal emulator (xterm.js) and a server-side process. All three kinds of processes — OS commands, non-interactive agent instances, and interactive agent instances — are just OS processes controlled by the Agent Manager. Each is initiated as an interaction instance that streams its output to the browser and accepts input from it.

Two universal data flows exist:

- **Process → Browser (output)**: All process output must be forwarded byte-for-byte to the browser. For pipe-based processes (OS commands, non-interactive agents) this is plain text; for PTY-backed processes (interactive agents) this includes ANSI/VT100 escape sequences that the browser terminal renders as cursor movement, color, and styling.

- **Browser → Process (input)**: User input arrives over WebSocket and is written to the process's stdin. The process does not distinguish between a single keystroke or a chunk of pasted text.

## 2. Unified Process Instance Model

All three process types share a common lifecycle managed by a **Process Instance Pool** — a thread-safe registry of running process instances, each tracked by a unique session ID.

| Process Type | IO Plumbing | stdin Lifetime | Output Format |
|---|---|---|---|
| OS command | Pipe (stdin/stdout/stderr) | Open for interaction | Raw text stream |
| Non-interactive agent | Pipe (stdin/stdout/stderr) | Open for interaction | Raw text stream |
| Interactive agent | PTY (merged stdin/stdout/stderr) | Open for interaction | Raw ANSI stream |

Key design ideas:

- **Unified lifecycle**: `spawn` creates the process and registers it in the pool; `monitor` streams its output; `terminate` kills it and cleans up. The same primitives serve all three types.

- **OS commands** are rerouted by the shell router (via the `os` prefix) into the same pool, executing as short-lived process instances with standard pipe IO.

- **All instances stream**. There is no non-streaming mode. Output flows byte-for-byte from process creation through to exit, whether the process runs for 100ms or indefinitely.

- **Per-instance isolation**: A WebSocket or REST request targets one process by PID (or session ID for routing). Cleanup (closing PTY or pipes, killing the process) is scoped to that entry so one failure does not cascade.

## 3. Streaming IO Architecture and Latency Mitigation

Because every process instance is a streaming interaction instance, the IO path is uniform:

- **Output streaming**: A dedicated goroutine reads from the process's output (stdout for pipes, the PTY for interactive sessions) and forwards raw bytes to the WebSocket connection. No line-buffering or text framing is applied — ANSI sequences pass through untouched.

- **Input streaming**: Inbound WebSocket messages carry input data that is written directly to the process's stdin. The process reads from its stdin as normal; the streaming transport is invisible to it.

For OS commands and non-interactive agents this works immediately — their output is streamed in real time, and any input fed to their stdin is consumed just as it would be in a local terminal.

### 3.1 Latency Challenge for Interactive Scenarios

Internet round-trip time makes per-character streaming impractical for high-speed typing in interactive sessions. Each keystroke traverses browser → internet → server → PTY → process before the user sees feedback. At typical internet RTTs (50–300ms) this creates a perceptible delay that breaks fluent typing.

### 3.2 Hybrid Input Strategy

The solution combines two input paths, both writing to the same process stdin:

- **Immediate keystrokes** (arrows, Ctrl+C, Tab, etc.) are sent via WebSocket as single keystrokes. These are latency-sensitive actions where the user expects instant feedback from the TUI application.

- **Text entry** (code, commands) is locally buffered and transmitted in a single batch. The process receives the entire block at once, bypassing per-character network RTT.

From the process's perspective both arrive identically — it just reads bytes from stdin. The buffering is a transport optimization invisible to the running application.

### 3.3 REST API for Batched Input

Interactive agent instances add a requirement not covered by WebSocket streaming alone: the Agent Manager and external callers (e.g., automation hooks, the web GUI outside the terminal widget) need a way to inject batched text into a running interactive session programmatically.

The solution is a dedicated REST endpoint:

```
POST /api/process/{pid}/stdin
Content-Type: application/json

{ "input": "def my_function():\n    return True\n" }
```

- `pid` identifies the target process instance by its OS process ID.
- The request body is a JSON object with a single `input` field containing the string to write.
- The backend writes the string verbatim to the process's stdin (with an optional trailing newline for line-oriented programs).
- The response confirms delivery (`202 Accepted`) or returns an error if the PID is not found.

This endpoint serves as the batched delivery mechanism that mitigates typing latency: a caller composes or pastes a block of text locally and delivers it in a single round trip. The same endpoint supports programmatic input injection by the Agent Manager itself.

### 3.4 UI Interaction for Interactive Handoff

All processes run as streaming instances from the moment they are spawned. When a process has been running for more than 3 seconds, the frontend displays a button labeled e.g. "Open Interactive Tab". Clicking it opens a new terminal tab dedicated to that process in a hybrid/buffered interaction mode. The original terminal tab hands over all IO for that process to the new tab and becomes ready to accept new commands — it is no longer involved in the running process's IO.

In this hybrid tab:

- **Output streaming works as normal** — the backend continues forwarding the process's stdout/PTY output to the terminal widget in real time. The user sees all output, including ANSI rendering, exactly as before.
- **Input/keystrokes are not streamed from the browser**. The keyboard is not wired to the terminal's input channel.
- Instead, a **client-side text input box** is rendered below or beside the terminal display. The user composes or pastes text into this box.
- On submission, the text is sent to the backend via the REST API endpoint (`POST /api/process/{pid}/stdin`), which writes it to the target process's stdin.

This design ensures the user can interact with long-running TUI processes without per-character network latency degrading the typing experience. The output remains live and responsive, while input is delivered in application-meaningful blocks. The process itself is unaware of the buffering — it simply reads the text from its stdin as it arrives.

## 4. Implementation Plan

### Task 1 — Align Project Documents with New Design

Update all `.md` documentation files in the project (excluding `interactive_tui.md` itself) to reflect the unified Process Instance Pool model, streaming IO architecture, dual-mode instance support, hybrid input strategy, and REST API for stdin injection. Files that require updates include:

- `agents_manager/technical-design.md` — replace AgentPool references with Process Instance Pool; update Agent struct to include PTY mode, streaming IO, and dual-mode lifecycle descriptions
- `human-ai-agent-interaction-hld.md` — align terminology (process instances, streaming, hybrid input)
- `vision.md` — update terminology and scope to reflect the unified process model

### Task 2 — Rename Pool to Process Instance Pool

Refactor the `Pool` type and its methods (`Submit`, `spawn`, `monitor`, `terminate`) to use neutral process-instance terminology. Introduce a mode field on `Agent` (or the equivalent process struct) that distinguishes `PipeMode` (OS commands, non-interactive agents) from `PTYMode` (interactive agents). Ensure the `byPID` and `bySessionID` maps are both present.

### Task 3 — Implement Streaming IO for All Process Types

Convert the existing `monitor` goroutine from line-buffered (`bufio.Scanner`) to raw byte streaming. The output path must forward bytes verbatim to the WebSocket connection without framing or line splitting. For PTY-backed instances, a single merged output stream replaces separate stdout/stderr pipes.

### Task 4 — Add PTY Plumbing for Interactive Mode

Integrate a PTY library (e.g. `github.com/creack/pty`). When spawning an interactive agent instance, allocate a PTY instead of separate stdin/stdout/stderr pipes. Wire the PTY's file descriptor as the process's stdio. Add a `TerminalInstance` helper that wraps the PTY with resize support (`pty.Setsize`) and raw read/write access.

### Task 5 — Implement REST API Endpoint for Stdin Injection

Add `POST /api/process/{pid}/stdin` that accepts a JSON body with an `input` string field, looks up the target process by PID in the process pool, and writes the string to the process's stdin. Return `202 Accepted` on success or `404 Not Found` if the PID is not active. Wire this endpoint into the existing HTTP server router.

### Task 6 — Frontend: 3-Second Threshold Button and Hybrid Tab

In the browser-based terminal UI, detect when a process has been running for more than 3 seconds and display an "Open Interactive Tab" button. When clicked:
- Open a new terminal tab dedicated to the same process.
- Keep output streaming live in the new tab.
- Disable keyboard-to-stdin forwarding; display a text input box instead.
- On submission, call `POST /api/process/{pid}/stdin` with the composed text.
- The original terminal tab hands off IO and returns to a command-prompt state.

### Task 7 — Frontend: Keystream Mode for Immediate Input

Ensure the default terminal tab streams keystrokes immediately over WebSocket for latency-sensitive actions (arrows, Ctrl+C, Tab, etc.). This provides responsive navigation and control for TUI applications alongside the buffered REST API path.

### Task 8 — Integration Tests

Write integration tests covering:
- Non-interactive agent: spawn, stream output, verify exit.
- Interactive agent: spawn with PTY, write input via REST API, verify ANSI output streamed.
- OS command: route through `os` prefix, stream output.
- Concurrent instances: multiple process types running simultaneously in the pool.
- Cleanup: kill one instance does not affect others.

