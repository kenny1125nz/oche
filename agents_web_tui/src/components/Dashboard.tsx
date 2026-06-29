import { useStore } from '../state'

export function Dashboard() {
  const { state, dispatch } = useStore()

  const openModal = (pid: number, binary: string, sessionId: string) => {
    dispatch({ type: 'OPEN_MODAL', pid, binary, sessionId })
  }

  return (
    <div className="dashboard-view">
      <h2>Sessions</h2>
      <div id="sessions-panel">
        {state.sessions.length === 0 && (
          <div style={{ color: '#8b949e', fontSize: 13, padding: 20, textAlign: 'center' }}>
            No sessions
          </div>
        )}
        {state.sessions.map(s => (
          <div key={s.id} className="sess-card">
            <div className="sid">
              <span
                className="dot"
                style={{
                  display: 'inline-block',
                  width: 8,
                  height: 8,
                  borderRadius: '50%',
                  marginRight: 6,
                  background: s.active ? '#3fb950' : '#484f58',
                }}
              />
              {s.id}
            </div>
            <div className="meta">
              <span>ws: {s.workspace_id || '—'}</span>
              <span>agent: {s.agent_id || '—'}</span>
            </div>
          </div>
        ))}
      </div>

      <h2 style={{ marginTop: 20 }}>Running Agents</h2>
      <div id="agent-cards">
        {state.activeAgents.length === 0 && (
          <div style={{ color: '#8b949e', fontSize: 13, padding: 20, textAlign: 'center' }}>
            No active agents
          </div>
        )}
        {state.activeAgents.map(a => (
          <div
            key={a.pid}
            className="agent-card"
            onClick={() => openModal(a.pid, a.binary, a.session_id)}
          >
            <div className="card-pid">PID {a.pid}</div>
            <div className="card-meta">
              <span>binary: {a.binary}</span>
              <span>session: {a.session_id}</span>
              {a.dir ? <span>dir: {a.dir}</span> : null}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}
