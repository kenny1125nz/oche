package session

import (
	"path/filepath"
	"time"

	"github.com/anomalyco/agents_manager/internal/workspace"
)

type State string

const (
	StateIdle    State = "idle"
	StateRunning State = "running"
)

type Session struct {
	ID           string    `json:"id"`
	Transport    string    `json:"transport"`
	WorkspaceID  string    `json:"workspace_id"`
	SubWorkspace string    `json:"sub_workspace"`
	ActiveFile   string    `json:"active_file"`
	AgentID      string    `json:"agent_id"`
	State        State     `json:"state"`
	History      []Request `json:"history"`
	TermCols     uint16    `json:"term_cols"`
	TermRows     uint16    `json:"term_rows"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Request struct {
	ID          string     `json:"id"`
	Text        string     `json:"text"`
	WorkspaceID string     `json:"workspace_id"`
	AgentID     string     `json:"agent_id"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

func (s *Session) EffectiveDir(reg *workspace.Registry) string {
	if s.WorkspaceID == "" {
		return ""
	}
	ws, err := reg.Get(s.WorkspaceID)
	if err != nil {
		return ""
	}
	if s.SubWorkspace == "" {
		return ws.TargetDir
	}
	return filepath.Join(ws.TargetDir, s.SubWorkspace)
}
