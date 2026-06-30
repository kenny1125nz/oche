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
	"syscall"
	"time"

	"github.com/creack/pty"
)

type Pooler interface {
	Submit(sessionID, binary string, args []string, dir string, requestText string) (<-chan ResponseEvent, error)
	SubmitWithMode(sessionID, binary string, args []string, dir string, mode ProcessMode, requestText string) (<-chan ResponseEvent, error)
	SendInput(pid int, text string) error
	ResizePTY(pid int, rows, cols uint16) error
	SubscribeAgent(pid int) (chan string, error)
	UnsubscribeAgent(pid int, ch chan string)
	GetAgent(pid int) *Agent
	GetAgentBySession(sessionID string) *Agent
	KillByPID(pid int) error
	Kill(sessionID string) error
	ActiveList() []AgentInfo
	ActiveCount() int
	QueuedCount() int
	HasActive(sessionID string) bool
	Shutdown()
}

type Pool struct {
	max    int
	active map[string]*Agent   // keyed by unique InstanceID
	byPID  map[int]*Agent      // keyed by OS PID
	bySess map[string][]string // sessionID → list of InstanceIDs
	queue  []queuedRequest
	mu     sync.Mutex
	log    func(string, ...any)
	nextID int64 // monotonic counter for InstanceID generation
}

type queuedRequest struct {
	sessionID   string
	binary      string
	args        []string
	dir         string
	mode        ProcessMode
	requestText string
	resultCh    chan ResponseEvent
}

func New(max int, logFn func(string, ...any)) *Pool {
	if max < 1 {
		max = 4
	}
	if logFn == nil {
		logFn = func(msg string, args ...any) {
			s := msg
			for i := 0; i < len(args); i += 2 {
				key := fmt.Sprintf("%v", args[i])
				var val string
				if i+1 < len(args) {
					val = fmt.Sprintf("%v", args[i+1])
				} else {
					val = "<missing>"
				}
				s += " " + key + "=" + val
			}
			fmt.Fprintln(os.Stderr, s)
		}
	}
	return &Pool{
		max:    max,
		active: make(map[string]*Agent),
		byPID:  make(map[int]*Agent),
		bySess: make(map[string][]string),
		log:    logFn,
	}
}

func (p *Pool) Submit(sessionID, binary string, args []string, dir string, requestText string) (<-chan ResponseEvent, error) {
	return p.SubmitWithMode(sessionID, binary, args, dir, PipeMode, requestText)
}

func (p *Pool) SubmitWithMode(sessionID, binary string, args []string, dir string, mode ProcessMode, requestText string) (<-chan ResponseEvent, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ch := make(chan ResponseEvent, 256)

	if len(p.active) >= p.max {
		pos := len(p.queue) + 1
		p.queue = append(p.queue, queuedRequest{
			sessionID:   sessionID,
			binary:      binary,
			args:        args,
			dir:         dir,
			mode:        mode,
			requestText: requestText,
			resultCh:    ch,
		})
		ch <- ResponseEvent{
			SessionID: sessionID,
			Type:      "progress",
			Text:      fmt.Sprintf("queued, position %d", pos),
			Timestamp: time.Now(),
		}
		return ch, nil
	}

	p.spawn(sessionID, binary, args, dir, mode, requestText, ch)
	return ch, nil
}

func (p *Pool) SendInput(pid int, text string) error {
	p.mu.Lock()
	agent, ok := p.byPID[pid]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("no active process with pid %d", pid)
	}
	return agent.WriteStdin(text)
}

func (p *Pool) SendInputRaw(pid int, data []byte) error {
	p.mu.Lock()
	agent, ok := p.byPID[pid]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("no active process with pid %d", pid)
	}
	return agent.WriteStdinRaw(data)
}

func (p *Pool) ResizePTY(pid int, rows, cols uint16) error {
	p.mu.Lock()
	agent, ok := p.byPID[pid]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("no active process with pid %d", pid)
	}
	return agent.Resize(rows, cols)
}

func (p *Pool) SubscribeAgent(pid int) (chan string, error) {
	p.mu.Lock()
	agent, ok := p.byPID[pid]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no active process with pid %d", pid)
	}
	return agent.Subscribe(), nil
}

func (p *Pool) UnsubscribeAgent(pid int, ch chan string) {
	p.mu.Lock()
	agent, ok := p.byPID[pid]
	p.mu.Unlock()
	if ok {
		agent.Unsubscribe(ch)
	}
}

func (p *Pool) GetAgent(pid int) *Agent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.byPID[pid]
}

func (p *Pool) GetAgentBySession(sessionID string) *Agent {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := p.bySess[sessionID]
	if len(ids) == 0 {
		return nil
	}
	return p.active[ids[len(ids)-1]] // latest agent for the session
}

func (p *Pool) KillByPID(pid int) error {
	p.mu.Lock()
	agent, ok := p.byPID[pid]
	p.mu.Unlock()

	if !ok {
		return fmt.Errorf("no active process with pid %d", pid)
	}

	return p.terminate(agent)
}

func (p *Pool) ActiveList() []AgentInfo {
	p.mu.Lock()
	defer p.mu.Unlock()

	list := make([]AgentInfo, 0, len(p.active))
	for _, a := range p.active {
		list = append(list, AgentInfo{
			SessionID: a.SessionID,
			PID:       a.PID,
			Binary:    a.Binary,
			Args:      a.Args,
			Dir:       a.Dir,
			Mode:      a.Mode,
			State:     a.State,
			StartedAt: a.StartedAt,
		})
	}
	return list
}

func (p *Pool) CloseStdin(sessionID string) error {
	agent := p.GetAgentBySession(sessionID)
	if agent == nil {
		return fmt.Errorf("no active process for session %s", sessionID)
	}
	agent.CloseStdin()
	return nil
}

func (p *Pool) Kill(sessionID string) error {
	agent := p.GetAgentBySession(sessionID)
	if agent == nil {
		return fmt.Errorf("no active process for session %s", sessionID)
	}
	return p.terminate(agent)
}

func (p *Pool) ActiveCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.active)
}

func (p *Pool) QueuedCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.queue)
}

func (p *Pool) HasActive(sessionID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.bySess[sessionID]) > 0
}

func (p *Pool) GetProcessStartTime(pid int) (time.Time, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	agent, ok := p.byPID[pid]
	if !ok {
		return time.Time{}, false
	}
	return agent.StartedAt, true
}

func (p *Pool) Shutdown() {
	p.mu.Lock()
	agents := make([]*Agent, 0, len(p.active))
	for _, a := range p.active {
		agents = append(agents, a)
	}
	p.mu.Unlock()

	var wg sync.WaitGroup
	for _, a := range agents {
		wg.Add(1)
		go func(ag *Agent) {
			defer wg.Done()
			ag.CloseStdin()
			_ = p.terminate(ag)
		}(a)
	}
	wg.Wait()
}

func BuildEnv(dir string) []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env)+2)
	for _, e := range env {
		if !strings.HasPrefix(e, "PWD=") {
			filtered = append(filtered, e)
		}
	}
	if dir != "" {
		filtered = append(filtered, "PWD="+dir)
	}
	filtered = append(filtered, "TERM=xterm-256color")
	return filtered
}

func fmtAgentCmd(binary string, args []string, dir string) string {
	s := binary
	for _, a := range args {
		s += " " + a
	}
	if dir != "" {
		s += " (dir: " + dir + ")"
	}
	return s
}

func (p *Pool) spawnError(sessionID string, cancel context.CancelFunc, id string, msg string, ch chan ResponseEvent) {
	cancel()
	p.removeInstance(id, sessionID)
	ch <- ResponseEvent{
		SessionID: sessionID,
		Type:      "error",
		Text:      msg,
		Timestamp: time.Now(),
	}
	close(ch)
}

func (p *Pool) spawn(sessionID, binary string, args []string, dir string, mode ProcessMode, requestText string, ch chan ResponseEvent) {
	ctx, cancel := context.WithCancel(context.Background())
	agent := NewAgent(sessionID, binary, args, dir, mode, cancel)

	p.nextID++
	id := fmt.Sprintf("inst-%d", p.nextID)
	agent.InstanceID = id
	p.active[id] = agent
	p.bySess[sessionID] = append(p.bySess[sessionID], id)

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	cmd.Env = BuildEnv(dir)

	p.log("spawn process", "session", sessionID, "binary", binary, "args", args, "dir", dir, "mode", mode)

	if mode == PTYMode {
		p.spawnPTY(sessionID, agent, cmd, ch)
		return
	}

	p.spawnPipe(sessionID, agent, cmd, requestText, ch)
}

func (p *Pool) spawnPTY(sessionID string, agent *Agent, cmd *exec.Cmd, ch chan ResponseEvent) {
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		p.spawnError(sessionID, agent.cancel, agent.InstanceID, fmt.Sprintf("pty spawn: %v", err), ch)
		return
	}
	agent.SetPTY(ptmx)
	agent.stdin = ptmx
	agent.Cmd = cmd
	agent.PID = cmd.Process.Pid
	p.byPID[agent.PID] = agent

	p.log("process spawned", "session", sessionID, "pid", agent.PID, "cmd", fmtAgentCmd(agent.Binary, agent.Args, agent.Dir), "mode", PTYMode)

	ch <- ResponseEvent{
		SessionID: sessionID,
		Type:      "ack",
		Text:      fmt.Sprintf("spawned (pid %d)", cmd.Process.Pid),
		Timestamp: time.Now(),
	}

	go p.monitor(agent, ptmx, nil, ch)
}

func (p *Pool) spawnPipe(sessionID string, agent *Agent, cmd *exec.Cmd, requestText string, ch chan ResponseEvent) {
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		p.spawnError(sessionID, agent.cancel, agent.InstanceID, fmt.Sprintf("stdin pipe: %v", err), ch)
		return
	}
	agent.stdin = stdinPipe

	if requestText != "" {
		io.WriteString(stdinPipe, requestText+"\n")
	}
	stdinPipe.Close()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		p.spawnError(sessionID, agent.cancel, agent.InstanceID, fmt.Sprintf("stdout pipe: %v", err), ch)
		return
	}

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		p.spawnError(sessionID, agent.cancel, agent.InstanceID, fmt.Sprintf("stderr pipe: %v", err), ch)
		return
	}

	agent.Cmd = cmd

	if err := cmd.Start(); err != nil {
		p.log("spawn failed", "session", sessionID, "error", err)
		p.spawnError(sessionID, agent.cancel, agent.InstanceID, fmt.Sprintf("spawn: %v", err), ch)
		return
	}

	agent.PID = cmd.Process.Pid
	p.byPID[agent.PID] = agent

	p.log("process spawned", "session", sessionID, "pid", agent.PID, "cmd", fmtAgentCmd(agent.Binary, agent.Args, agent.Dir), "mode", PipeMode)

	ch <- ResponseEvent{
		SessionID: sessionID,
		Type:      "ack",
		Text:      fmt.Sprintf("spawned (pid %d)", cmd.Process.Pid),
		Timestamp: time.Now(),
	}

	go p.monitor(agent, stdoutPipe, stderrPipe, ch)
}

func readOutput(agent *Agent, r io.Reader, ch chan ResponseEvent, target *bytes.Buffer, onDone func()) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			agent.mu.Lock()
			target.Write(chunk)
			agent.mu.Unlock()
			text := string(chunk)
			agent.broadcast(text)
			ch <- ResponseEvent{
				SessionID: agent.SessionID,
				Type:      "progress",
				Text:      text,
				Data:      chunk,
				Timestamp: time.Now(),
			}
		}
		if err != nil {
			if onDone != nil {
				onDone()
			}
			return
		}
	}
}

func (p *Pool) monitor(agent *Agent, stdout, stderr io.Reader, ch chan ResponseEvent) {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		var onDone func()
		if agent.Mode == PTYMode {
			onDone = func() {
				agent.mu.Lock()
				if agent.pty != nil {
					agent.pty.Close()
					agent.pty = nil
				}
				agent.mu.Unlock()
			}
		}
		readOutput(agent, stdout, ch, agent.stdout, onDone)
	}()

	if agent.Mode != PTYMode && stderr != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			readOutput(agent, stderr, ch, agent.stderr, nil)
		}()
	}

	wg.Wait()

	exitErr := agent.Cmd.Wait()
	agent.CloseStdin()

	p.mu.Lock()
	p.removeInstance(agent.InstanceID, agent.SessionID)
	delete(p.byPID, agent.PID)
	p.mu.Unlock()

	p.emitExitEvent(agent, exitErr, ch)

	agent.broadcast("__agent_exit__")
	close(ch)

	p.mu.Lock()
	p.dispatchNext()
	p.mu.Unlock()
}

func (p *Pool) emitExitEvent(agent *Agent, err error, ch chan ResponseEvent) {
	sessionID := agent.SessionID

	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			agent.ExitCode = ee.ExitCode()
		}
		stderrText := strings.TrimSpace(agent.stderr.String())
		stderrSuffix := ""
		if stderrText != "" {
			stderrSuffix = " | stderr: " + stderrText
		}
		p.log("process exited with error",
			"session", sessionID, "pid", agent.PID,
			"binary", agent.Binary, "args", agent.Args,
			"dir", agent.Dir, "exit_code", agent.ExitCode,
			"mode", agent.Mode,
			"stderr", stderrText)
		if agent.State == StateKilled {
			ch <- ResponseEvent{
				SessionID: sessionID,
				Type:      "error",
				Text:      "killed",
				Timestamp: time.Now(),
			}
		} else {
			agent.State = StateError
			ch <- ResponseEvent{
				SessionID: sessionID,
				Type:      "error",
				Text:      fmt.Sprintf("exited with code %d%s", agent.ExitCode, stderrSuffix),
				Timestamp: time.Now(),
			}
		}
	} else {
		agent.State = StateDone
	}

	ch <- ResponseEvent{
		SessionID: sessionID,
		Type:      "deliverable",
		Text:      "done",
		Timestamp: time.Now(),
	}
}

// removeInstance removes an agent instance from both p.active and
// p.bySess. Must be called with p.mu held.
func (p *Pool) removeInstance(id, sessionID string) {
	delete(p.active, id)
	ids := p.bySess[sessionID]
	for i, v := range ids {
		if v == id {
			p.bySess[sessionID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
}

func (p *Pool) terminate(agent *Agent) error {
	agent.State = StateKilled
	agent.cancel()

	if agent.pty != nil {
		agent.pty.Close()
	}

	if err := agent.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("SIGTERM: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := agent.Cmd.Process.Wait()
		done <- err
	}()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		if err := agent.Cmd.Process.Signal(syscall.SIGKILL); err != nil {
			return fmt.Errorf("SIGKILL: %w", err)
		}
		<-done
		return nil
	}
}

func (p *Pool) dispatchNext() {
	if len(p.queue) == 0 || len(p.active) >= p.max {
		return
	}

	req := p.queue[0]
	p.queue = p.queue[1:]

	p.spawn(req.sessionID, req.binary, req.args, req.dir, req.mode, req.requestText, req.resultCh)
}
