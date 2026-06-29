package pool

import "time"

type ResponseEvent struct {
	SessionID   string    `json:"session_id"`
	Type        string    `json:"type"`
	Text        string    `json:"text"`
	Data        []byte    `json:"data,omitempty"`
	WorkspaceID string    `json:"workspace_id,omitempty"`
	AgentID     string    `json:"agent_id,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}
