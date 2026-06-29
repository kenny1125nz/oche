package shell

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/anomalyco/agents_manager/internal/pool"
)

func (r *Router) cmdOpen(sessionID string, args []string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 10)

	if len(args) == 0 {
		emit(ch, "error", "usage: open <workspace>")
		close(ch)
		return ch
	}

	sess, err := r.store.Get(sessionID)
	if err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	target := args[0]
	ws, err := r.workspaces.Get(target)
	if err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	sess.WorkspaceID = ws.ID
	sess.SubWorkspace = ""
	if err := r.store.Update(sess); err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	emitSession(ch, "deliverable", fmt.Sprintf("switched to workspace %s (%s)", ws.ID, ws.TargetDir), ws.ID, "")
	close(ch)
	return ch
}

func (r *Router) cmdCd(sessionID string, args []string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 10)

	sess, err := r.store.Get(sessionID)
	if err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	if sess.WorkspaceID == "" {
		emit(ch, "error", "no workspace selected (use 'open <workspace>' first)")
		close(ch)
		return ch
	}

	ws, err := r.workspaces.Get(sess.WorkspaceID)
	if err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	if len(args) == 0 {
		emit(ch, "error", "usage: cd <subdir>")
		close(ch)
		return ch
	}

	target := args[0]

	if target == "/" {
		sess.SubWorkspace = ""
		if err := r.store.Update(sess); err != nil {
			emit(ch, "error", err.Error())
			close(ch)
			return ch
		}
		emit(ch, "deliverable", fmt.Sprintf("switched to workspace %s (%s)", ws.ID, ws.TargetDir))
		close(ch)
		return ch
	}

	if target == ".." {
		if sess.SubWorkspace == "" {
			emit(ch, "deliverable", "already at workspace root")
			close(ch)
			return ch
		}
		parts := strings.Split(sess.SubWorkspace, "/")
		if len(parts) <= 1 {
			sess.SubWorkspace = ""
		} else {
			sess.SubWorkspace = strings.Join(parts[:len(parts)-1], "/")
		}
		if err := r.store.Update(sess); err != nil {
			emit(ch, "error", err.Error())
			close(ch)
			return ch
		}
		displayPath := sess.EffectiveDir(r.workspaces)
		emitSession(ch, "deliverable", fmt.Sprintf("switched to workspace %s (%s)", ws.ID, displayPath), ws.ID, "")
		close(ch)
		return ch
	}

	if strings.Contains(target, "/") || target == "." {
		emit(ch, "error", "usage: cd <subdir> (single directory name only)")
		close(ch)
		return ch
	}

	newSub := target
	if sess.SubWorkspace != "" {
		newSub = sess.SubWorkspace + "/" + target
	}

	fullPath := filepath.Join(ws.TargetDir, newSub)
	info, err := os.Stat(fullPath)
	if err != nil || !info.IsDir() {
		emit(ch, "error", fmt.Sprintf("directory not found: %s", fullPath))
		close(ch)
		return ch
	}

	sess.SubWorkspace = newSub
	if err := r.store.Update(sess); err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	emit(ch, "deliverable", fmt.Sprintf("switched to workspace %s (%s)", ws.ID, fullPath))
	close(ch)
	return ch
}

func (r *Router) cmdLs(sessionID string, args []string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 100)

	dir := ""
	sess, err := r.store.Get(sessionID)
	if err == nil {
		dir = sess.EffectiveDir(r.workspaces)
	}

	cmd := exec.Command("ls", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if dir != "" {
		cmd.Dir = dir
	}

	_ = cmd.Run()

	if stdout.Len() > 0 {
		emit(ch, "deliverable", strings.TrimRight(stdout.String(), "\n"))
	}
	if stderr.Len() > 0 {
		for _, line := range strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n") {
			emit(ch, "progress", line)
		}
	}

	close(ch)
	return ch
}

func (r *Router) cmdPwd(sessionID string) <-chan pool.ResponseEvent {
	ch := make(chan pool.ResponseEvent, 10)

	sess, err := r.store.Get(sessionID)
	if err != nil {
		emit(ch, "error", err.Error())
		close(ch)
		return ch
	}

	dir := sess.EffectiveDir(r.workspaces)
	if dir == "" {
		emit(ch, "deliverable", "no workspace set (use open <workspace>)")
		close(ch)
		return ch
	}

	emit(ch, "deliverable", dir)
	close(ch)
	return ch
}
