package shell

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/anomalyco/agents_manager/internal/pool"
	"github.com/creack/pty"
	"golang.org/x/term"
)

var osProcesses sync.Map

func (r *Router) cmdOs(sessionID string, args []string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 4096)

	if len(args) == 0 {
		emit(ch, "error", "usage: os <command> [args...]")
		close(ch)
		return ch
	}

	cmd := exec.Command("sh", "-c", strings.Join(args, " "))

	var dir string
	var termRows, termCols uint16 = 24, 80
	sess, err := r.store.Get(sessionID)
	if err == nil {
		dir = sess.EffectiveDir(r.workspaces)
		if dir != "" {
			cmd.Dir = dir
		}
		if sess.TermRows > 0 && sess.TermCols > 0 {
			termRows = sess.TermRows
			termCols = sess.TermCols
		}
	}
	cmd.Env = pool.BuildEnv(dir)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: termRows, Cols: termCols})
	if err != nil {
		emit(ch, "error", fmt.Sprintf("start: %v", err))
		close(ch)
		return ch
	}

	if _, err := term.MakeRaw(int(ptmx.Fd())); err != nil {
	}

	osProcesses.Store(sessionID, ptmx)

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				ch <- pool.ResponseEvent{
					Type:      "progress",
					Text:      string(chunk),
					Data:      chunk,
					Timestamp: time.Now(),
				}
			}
			if err != nil {
				break
			}
		}

		waitErr := cmd.Wait()
		ptmx.Close()
		osProcesses.Delete(sessionID)

		if waitErr != nil {
			ch <- pool.ResponseEvent{
				Type:      "error",
				Text:      fmt.Sprintf("exit: %v", waitErr),
				Timestamp: time.Now(),
			}
		}
		ch <- pool.ResponseEvent{
			Type:      "deliverable",
			Text:      "done",
			Timestamp: time.Now(),
		}
		close(ch)
	}()

	return ch
}

func WriteOSInput(sessionID, text string) {
	if v, ok := osProcesses.Load(sessionID); ok {
		if ptmx, ok := v.(*os.File); ok {
			io.WriteString(ptmx, text)
		}
	}
}

func HasOSProcess(sessionID string) bool {
	_, ok := osProcesses.Load(sessionID)
	return ok
}

func ResizeOSProcess(sessionID string, rows, cols uint16) error {
	v, ok := osProcesses.Load(sessionID)
	if !ok {
		return fmt.Errorf("no OS process for session %s", sessionID)
	}
	ptmx, ok := v.(*os.File)
	if !ok {
		return fmt.Errorf("invalid PTY handle for session %s", sessionID)
	}
	return pty.Setsize(ptmx, &pty.Winsize{Rows: rows, Cols: cols})
}
