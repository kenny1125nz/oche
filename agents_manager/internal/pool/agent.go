package pool

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

type State string

const (
	StateRunning State = "running"
	StateDone    State = "done"
	StateError   State = "error"
	StateKilled  State = "killed"
)

type ProcessMode string

const (
	PipeMode ProcessMode = "pipe"
	PTYMode  ProcessMode = "pty"
)

type Agent struct {
	InstanceID string
	SessionID  string
	PID        int
	Binary     string
	Args       []string
	Dir        string
	Cmd        *exec.Cmd
	Mode       ProcessMode
	State      State
	StartedAt  time.Time
	ExitCode   int

	stdin  io.WriteCloser
	stdout *bytes.Buffer
	stderr *bytes.Buffer
	cancel context.CancelFunc

	pty *os.File

	mu      sync.RWMutex
	subs    []chan string
}

type AgentInfo struct {
	SessionID string      `json:"session_id"`
	PID       int         `json:"pid"`
	Binary    string      `json:"binary"`
	Args      []string    `json:"args"`
	Dir       string      `json:"dir"`
	Mode      ProcessMode `json:"mode"`
	State     State       `json:"state"`
	StartedAt time.Time   `json:"started_at"`
}

func NewAgent(sessionID, binary string, args []string, dir string, mode ProcessMode, cancel context.CancelFunc) *Agent {
	return &Agent{
		SessionID: sessionID,
		Binary:    binary,
		Args:      args,
		Dir:       dir,
		Mode:      mode,
		State:     StateRunning,
		StartedAt: time.Now(),
		stdout:    new(bytes.Buffer),
		stderr:    new(bytes.Buffer),
		cancel:    cancel,
	}
}

func (a *Agent) WriteStdin(data string) error {
	a.mu.RLock()
	stdin := a.stdin
	a.mu.RUnlock()
	if stdin == nil {
		return nil
	}
	_, err := io.WriteString(stdin, data+"\n")
	return err
}

func (a *Agent) WriteStdinRaw(data []byte) error {
	a.mu.RLock()
	stdin := a.stdin
	a.mu.RUnlock()
	if stdin == nil {
		return nil
	}
	_, err := stdin.Write(data)
	return err
}

func (a *Agent) CloseStdin() {
	a.mu.RLock()
	stdin := a.stdin
	a.mu.RUnlock()
	if stdin != nil {
		stdin.Close()
	}
}

func (a *Agent) Subscribe() chan string {
	a.mu.Lock()
	defer a.mu.Unlock()
	ch := make(chan string, 256)
	a.subs = append(a.subs, ch)
	return ch
}

func (a *Agent) Unsubscribe(ch chan string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, s := range a.subs {
		if s == ch {
			a.subs = append(a.subs[:i], a.subs[i+1:]...)
			break
		}
	}
	close(ch)
}

func (a *Agent) broadcast(line string) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, ch := range a.subs {
		select {
		case ch <- line:
		default:
		}
	}
}

func (a *Agent) StdoutLines() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return linesFromBuffer(a.stdout)
}

func (a *Agent) StderrLines() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return linesFromBuffer(a.stderr)
}

func (a *Agent) PTY() *os.File {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.pty
}

func (a *Agent) SetPTY(f *os.File) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pty = f
}

func (a *Agent) Resize(rows, cols uint16) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.pty == nil {
		return fmt.Errorf("no PTY allocated for agent PID %d", a.PID)
	}
	return pty.Setsize(a.pty, &pty.Winsize{Rows: rows, Cols: cols})
}

func linesFromBuffer(buf *bytes.Buffer) []string {
	if buf.Len() == 0 {
		return nil
	}
	return strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
}
