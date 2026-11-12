# OCHE

Research and tooling for **human / AI-agent / software interaction**.

OCHE explores how interfaces must change when an AI agent sits between human intent and software execution: multi-modal input, structured prose, agent-to-agent protocols, ambient notifications, and delegated autonomy. It includes an agent manager service and a web TUI.

## Components

- `agents_manager/` — Go service for managing agents (`go.mod`, internal packages, web assets)
- `agents_web_tui/` — React + Vite web terminal UI
- design documents covering interaction models, re-design, process management, and workspace patching

## Getting started

Agent manager:

```bash
cd agents_manager
go build ./...
go test ./...
```

Web TUI:

```bash
cd agents_web_tui
npm install
npm run dev
```

## Documentation

- `vision.md` — ideal interaction forms across human, agent, and software
- `human-ai-agent-interaction-hld.md` — high-level design
- `re-design.md`, `interactive_tui.md`, `process_manager.md` — component designs

## Status

Research + early implementation.
