package server

import (
	"context"

	"github.com/anomalyco/agents_manager/internal/pool"
)

type streamCallbacks struct {
	writeLine  func(line string)
	writeStderr func(line string) // optional; falls back to writeLine if nil
	writeExit  func()
	flush      func()
}

func streamAgentOutput(ctx context.Context, ch <-chan string, ag *pool.Agent, cb streamCallbacks) {
	drain:
	for {
		select {
		case line := <-ch:
			if line == "__agent_exit__" {
				cb.writeExit()
				return
			}
			if line != "" {
				cb.writeLine(line)
			}
		default:
			break drain
		}
	}

	writeStderr := cb.writeStderr
	if writeStderr == nil {
		writeStderr = cb.writeLine
	}

	for _, line := range ag.StdoutLines() {
		if line != "" {
			cb.writeLine(line)
		}
	}
	for _, line := range ag.StderrLines() {
		if line != "" {
			writeStderr(line)
		}
	}
	if cb.flush != nil {
		cb.flush()
	}

	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return
			}
			if line == "__agent_exit__" {
				cb.writeExit()
				return
			}
			if line != "" {
				cb.writeLine(line)
			}
		case <-ctx.Done():
			return
		}
	}
}
