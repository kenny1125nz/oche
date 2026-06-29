package pool

import (
	"strings"
	"testing"
	"time"
)

func echoAgent(t *testing.T) string {
	t.Helper()
	return "../../testdata/echo-agent.sh"
}

func timedAgent(t *testing.T) string {
	t.Helper()
	return "../../testdata/timed-agent.sh"
}

func lingerAgent(t *testing.T) string {
	t.Helper()
	return "../../testdata/linger-agent.sh"
}

func TestPoolSubmitAndReceive(t *testing.T) {
	p := New(2, nil)
	defer p.Shutdown()

	ch, err := p.Submit("sess-1", echoAgent(t), nil, "", "hello world")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// echo-agent reads stdin until EOF, so close stdin to let it finish
	p.CloseStdin("sess-1")

	var received []string
	for evt := range ch {
		received = append(received, evt.Text)
	}

	if len(received) == 0 {
		t.Fatal("expected at least one event")
	}

	if !strings.HasPrefix(received[0], "spawned") {
		t.Fatalf("first event should start with 'spawned', got %q", received[0])
	}

	found := false
	for _, r := range received {
		if strings.Contains(r, "received: hello world") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected agent output containing 'received: hello world', got %v", received)
	}

	last := received[len(received)-1]
	if !strings.Contains(last, "done") {
		t.Fatalf("last event should contain 'done', got %q", last)
	}
}

func TestPoolMaxConcurrency(t *testing.T) {
	p := New(1, nil)
	defer p.Shutdown()

	ch1, err := p.Submit("sess-1", timedAgent(t), nil, "", "0")
	if err != nil {
		t.Fatalf("Submit 1: %v", err)
	}

	if p.ActiveCount() != 1 {
		t.Fatalf("expected 1 active, got %d", p.ActiveCount())
	}

	ch2, err := p.Submit("sess-2", timedAgent(t), nil, "", "0")
	if err != nil {
		t.Fatalf("Submit 2: %v", err)
	}

	if p.ActiveCount() != 1 {
		t.Fatalf("expected still 1 active, got %d", p.ActiveCount())
	}
	if p.QueuedCount() != 1 {
		t.Fatalf("expected 1 queued, got %d", p.QueuedCount())
	}

	drain := func(ch <-chan ResponseEvent) {
		for range ch {
		}
	}
	done := make(chan struct{})
	go func() {
		drain(ch1)
		drain(ch2)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for agents to complete")
	}
}

func TestPoolKill(t *testing.T) {
	p := New(2, nil)
	defer p.Shutdown()

	ch, err := p.Submit("sess-1", lingerAgent(t), nil, "", "kill me")
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if !p.HasActive("sess-1") {
		t.Fatal("expected active agent")
	}

	if err := p.Kill("sess-1"); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	var foundKilled bool
	var last string
	for evt := range ch {
		if evt.Type == "error" && evt.Text == "killed" {
			foundKilled = true
		}
		last = evt.Text
	}

	if !foundKilled {
		t.Fatal("expected a 'killed' error event")
	}
	if last != "done" {
		t.Fatalf("expected final event 'done', got %q", last)
	}

	if p.HasActive("sess-1") {
		t.Fatal("expected agent to be removed after kill")
	}
}

func TestPoolQueueDispatchesAfterCompletion(t *testing.T) {
	p := New(1, nil)
	defer p.Shutdown()

	ch1, _ := p.Submit("sess-1", timedAgent(t), nil, "", "0")

	// Both should eventually complete
	ch2, _ := p.Submit("sess-2", timedAgent(t), nil, "", "0")

	drain := func(ch <-chan ResponseEvent) []string {
		var out []string
		for evt := range ch {
			out = append(out, evt.Text)
		}
		return out
	}

	out1 := drain(ch1)
	out2 := drain(ch2)

	if out1[len(out1)-1] != "done" {
		t.Fatalf("ch1 should end with 'done', got %q", out1[len(out1)-1])
	}
	if out2[len(out2)-1] != "done" {
		t.Fatalf("ch2 should end with 'done', got %q", out2[len(out2)-1])
	}
}

func TestPoolMultipleAgentsSameSession(t *testing.T) {
	p := New(4, nil)
	defer p.Shutdown()

	ch1, err := p.Submit("sess-1", timedAgent(t), nil, "", "0")
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}

	ch2, err := p.Submit("sess-1", timedAgent(t), nil, "", "second")
	if err != nil {
		t.Fatalf("second submit for same session: %v", err)
	}

	// Both agents should run independently. Drain their channels.
	drain := func(ch <-chan ResponseEvent) {
		for range ch {
		}
	}
	go drain(ch1)
	go drain(ch2)

	// Both agents should be tracked.
	if !p.HasActive("sess-1") {
		t.Fatal("expected sess-1 to be active")
	}
	infos := p.ActiveList()
	if len(infos) != 2 {
		t.Fatalf("expected 2 active agents, got %d", len(infos))
	}
}

func TestPoolPTYMode(t *testing.T) {
	p := New(2, nil)
	defer p.Shutdown()

	ch, err := p.SubmitWithMode("sess-pty", "echo", []string{"hello-pty-world"}, "", PTYMode, "")
	if err != nil {
		t.Fatalf("SubmitWithMode PTY: %v", err)
	}

	var received []string
	for evt := range ch {
		received = append(received, evt.Text)
	}

	if len(received) == 0 {
		t.Fatal("expected at least one event")
	}

	foundHello := false
	foundDone := false
	for _, r := range received {
		if strings.Contains(r, "hello-pty-world") {
			foundHello = true
		}
		if strings.Contains(r, "done") {
			foundDone = true
		}
	}
	if !foundHello {
		t.Fatalf("expected PTY output containing 'hello-pty-world', got %v", received)
	}
	if !foundDone {
		t.Fatalf("expected 'done' event, got %v", received)
	}
}

func TestPoolPTYModeStdinInjection(t *testing.T) {
	p := New(2, nil)
	defer p.Shutdown()

	ch, err := p.SubmitWithMode("sess-pty-stdin", "bash", []string{"-c", "read line; echo you said: $line"}, "", PTYMode, "")
	if err != nil {
		t.Fatalf("SubmitWithMode PTY: %v", err)
	}

	agent := p.GetAgentBySession("sess-pty-stdin")
	if agent == nil {
		t.Fatal("expected agent to exist after spawn")
	}

	agent.WriteStdin("hello from pty")

	var received []string
	for evt := range ch {
		received = append(received, evt.Text)
	}

	found := false
	for _, r := range received {
		if strings.Contains(r, "you said: hello from pty") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected PTY output containing 'you said: hello from pty', got %v", received)
	}
}

func TestPoolRawBytesInDataField(t *testing.T) {
	p := New(2, nil)
	defer p.Shutdown()

	ch, err := p.SubmitWithMode("sess-raw", "echo", []string{"raw-bytes-test"}, "", PTYMode, "")
	if err != nil {
		t.Fatalf("SubmitWithMode: %v", err)
	}

	var receivedData [][]byte
	for evt := range ch {
		if len(evt.Data) > 0 {
			receivedData = append(receivedData, evt.Data)
		}
	}

	if len(receivedData) == 0 {
		t.Fatal("expected at least one event with Data field")
	}

	allData := ""
	for _, d := range receivedData {
		allData += string(d)
	}
	if !strings.Contains(allData, "raw-bytes-test") {
		t.Fatalf("expected raw data to contain 'raw-bytes-test', got %q", allData)
	}
}

func TestPoolConcurrentMixedModes(t *testing.T) {
	p := New(3, nil)
	defer p.Shutdown()

	ch1, err := p.Submit("sess-conc-1", echoAgent(t), nil, "", "pipe-mode")
	if err != nil {
		t.Fatalf("Submit 1: %v", err)
	}
	p.CloseStdin("sess-conc-1")

	ch2, err := p.SubmitWithMode("sess-conc-2", "echo", []string{"pty-mode-output"}, "", PTYMode, "")
	if err != nil {
		t.Fatalf("SubmitWithMode 2: %v", err)
	}

	ch3, err := p.Submit("sess-conc-3", timedAgent(t), nil, "", "0")
	if err != nil {
		t.Fatalf("Submit 3: %v", err)
	}

	if p.ActiveCount() != 3 {
		t.Fatalf("expected 3 active, got %d", p.ActiveCount())
	}

	drain := func(ch <-chan ResponseEvent) []string {
		var out []string
		for evt := range ch {
			out = append(out, evt.Text)
		}
		return out
	}

	out1 := drain(ch1)
	out2 := drain(ch2)
	out3 := drain(ch3)

	if out1[len(out1)-1] != "done" {
		t.Fatalf("ch1 should end with 'done', got %q", out1[len(out1)-1])
	}
	if out2[len(out2)-1] != "done" {
		t.Fatalf("ch2 should end with 'done', got %q", out2[len(out2)-1])
	}
	if out3[len(out3)-1] != "done" {
		t.Fatalf("ch3 should end with 'done', got %q", out3[len(out3)-1])
	}
}

func TestPoolKillIsolation(t *testing.T) {
	p := New(2, nil)
	defer p.Shutdown()

	ch1, _ := p.SubmitWithMode("sess-iso-1", "sleep", []string{"30"}, "", PTYMode, "")
	ch2, _ := p.Submit("sess-iso-2", timedAgent(t), nil, "", "0")

	if p.ActiveCount() != 2 {
		t.Fatalf("expected 2 active, got %d", p.ActiveCount())
	}

	if err := p.Kill("sess-iso-1"); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	for evt := range ch1 {
		if evt.Type == "error" && evt.Text == "killed" {
			break
		}
	}

	if p.ActiveCount() != 1 {
		t.Fatalf("expected 1 active after killing one, got %d", p.ActiveCount())
	}

	var out2 []string
	for evt := range ch2 {
		out2 = append(out2, evt.Text)
	}
	if !strings.Contains(out2[len(out2)-1], "done") {
		t.Fatalf("surviving agent should complete normally, got %q", out2[len(out2)-1])
	}
}

func TestPoolSendInputRaw(t *testing.T) {
	p := New(2, nil)
	defer p.Shutdown()

	ch, err := p.SubmitWithMode("sess-raw-input", "bash", []string{"-c", "read -r line; echo line: $line"}, "", PTYMode, "")
	if err != nil {
		t.Fatalf("SubmitWithMode: %v", err)
	}

	agent := p.GetAgentBySession("sess-raw-input")
	if agent == nil {
		t.Fatal("expected agent to exist")
	}

	if err := p.SendInputRaw(agent.PID, []byte("raw-input-test\n")); err != nil {
		t.Fatalf("SendInputRaw: %v", err)
	}

	var received []string
	for evt := range ch {
		received = append(received, evt.Text)
	}

	found := false
	for _, r := range received {
		if strings.Contains(r, "line: raw-input-test") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected output containing 'line: raw-input-test', got %v", received)
	}
}

func TestPoolAgentInfoIncludesMode(t *testing.T) {
	p := New(2, nil)
	defer p.Shutdown()

	p.SubmitWithMode("sess-info-pty", "echo", []string{"test"}, "", PTYMode, "")
	p.Submit("sess-info-pipe", echoAgent(t), nil, "", "test")
	p.CloseStdin("sess-info-pipe")

	list := p.ActiveList()

	var foundPTY, foundPipe bool
	for _, info := range list {
		if info.Mode == PTYMode && info.SessionID == "sess-info-pty" {
			foundPTY = true
		}
		if info.Mode == PipeMode && info.SessionID == "sess-info-pipe" {
			foundPipe = true
		}
	}

	if !foundPTY {
		t.Fatal("expected AgentInfo with PTYMode")
	}
	if !foundPipe {
		t.Fatal("expected AgentInfo with PipeMode")
	}
}
