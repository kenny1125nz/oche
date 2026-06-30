import { useEffect, useCallback, useRef, useState } from 'react'
import { useStore } from './state'
import { checkStatus, tryConnect, refreshAccess } from './api/auth'
import { fetchActiveAgents } from './api/pool'
import { useSSE } from './hooks/useSSE'
import { useAgentConnection } from './hooks/useAgentWS'


import { AuthScreen } from './components/AuthScreen'
import { TopBar } from './components/TopBar'
import { Terminal } from './components/Terminal'
import { Dashboard } from './components/Dashboard'
import { AgentModal } from './components/AgentModal'
import { InteractiveTab } from './components/InteractiveTab'

export default function App() {
  const { state, dispatch } = useStore()
  const wsSessionRef = useRef(state.wsSessionId)
  wsSessionRef.current = state.wsSessionId
  const streamBufsRef = useRef(new Map<number, string[]>())
  const streamVersRef = useRef(new Map<number, number>())
  const [, forceRender] = useState(0)

  const onDashboard = useCallback((data: { type: string; pool: { active: number; queued: number }; sessions: import('./types').Session[] }) => {
    dispatch({ type: 'SET_POOL', active: data.pool.active, queued: data.pool.queued })
    dispatch({ type: 'SET_SESSIONS', sessions: data.sessions })

    const wsSid = wsSessionRef.current
    const match = wsSid
      ? data.sessions.find(s => s.id === wsSid)
      : data.sessions[0]
    if (match) {
      const wsName = match.workspace_id || '—'
      const agentName = match.agent_id || '—'
      if (match.workspace_id) localStorage.setItem('current_workspace', match.workspace_id)
      if (match.agent_id) localStorage.setItem('current_agent', match.agent_id)
      dispatch({ type: 'SET_CURRENT_WORKSPACE', workspace: wsName })
      dispatch({ type: 'SET_CURRENT_SUB_WORKSPACE', sub: match.sub_workspace || '' })
      dispatch({ type: 'SET_CURRENT_ACTIVE_FILE', file: match.active_file || '' })
      dispatch({ type: 'SET_CURRENT_AGENT', agent: agentName })
    }
  }, [dispatch])

  const onStreamOutput = useCallback((pid: number, text: string) => {
    const cleaned = text
      .replace(/\x1b\[\?2026h/g, '')
      .replace(/\x1b\[\?u/g, '')
      .replace(/\x1b\[u/g, '')
      .replace(/\x1b\[\?2031[hl]/g, '')
      .replace(/\x1b\[\?2027[hl]/g, '')
      .replace(/\x1b\[\?(?:100[0236]|101[56])[hl]/g, '')
      .replace(/\x1b\[\?\d+\$p/g, '')
    let buf = streamBufsRef.current.get(pid)
    if (!buf) {
      buf = []
      streamBufsRef.current.set(pid, buf)
    }
    buf.push(cleaned)
    streamVersRef.current.set(pid, (streamVersRef.current.get(pid) || 0) + 1)
    forceRender(n => n + 1)
  }, [])

  const onStreamExit = useCallback((pid: number) => {
    streamBufsRef.current.delete(pid)
    streamVersRef.current.delete(pid)
    dispatch({ type: 'CLOSE_INTERACTIVE_TAB', pid })
    forceRender(n => n + 1)
  }, [dispatch])

  useSSE(state.accessToken?.length ? state.accessToken : null, onDashboard)
  const { sendToAgent, resizeAgent } = useAgentConnection(state.interactiveTabs, state.accessToken, onStreamOutput, onStreamExit, 'ws')

  useEffect(() => {
    if (state.activeTab !== 'dashboard' || !state.accessToken) return
    const token: string = state.accessToken
    const interval = setInterval(async () => {
      const agents = await fetchActiveAgents(token)
      dispatch({ type: 'SET_ACTIVE_AGENTS', agents })
    }, 3000)
    return () => clearInterval(interval)
  }, [state.activeTab, state.accessToken, dispatch])

  useEffect(() => {
    if (state.activeTab !== 'dashboard' || !state.accessToken) return
    const token: string = state.accessToken
    fetchActiveAgents(token).then(agents =>
      dispatch({ type: 'SET_ACTIVE_AGENTS', agents })
    )
  }, [state.activeTab, state.accessToken, dispatch])

  useEffect(() => {
    if (!state.accessToken) return
    const token: string = state.accessToken
    const interval = setInterval(async () => {
      if (!state.refreshToken) return
      const exp = parseJWT(token)?.exp || 0
      const now = Date.now() / 1000
      if (exp - now < 120) {
        try {
          const data = await refreshAccess(state.refreshToken)
          dispatch({ type: 'SET_TOKENS', accessToken: data.access_token, refreshToken: data.refresh_token })
          localStorage.setItem('access_token', data.access_token)
          if (data.refresh_token) localStorage.setItem('refresh_token', data.refresh_token)
        } catch {
          // ignore
        }
      }
    }, 30000)
    return () => clearInterval(interval)
  }, [state.accessToken, state.refreshToken, dispatch])

  useEffect(() => {
    ;(async () => {
      if (state.authenticated) return
      const status = await checkStatus()

      if (!status.password_set) {
        dispatch({
          type: 'SET_AUTH',
          show: true,
          mode: 'setup',
          title: 'Set Password',
          desc: 'First run — choose a password to protect this instance.',
        })
        return
      }

      if (!state.accessToken) {
        dispatch({
          type: 'SET_AUTH',
          show: true,
          mode: 'login',
          title: 'Login',
          desc: 'Enter your password to connect.',
        })
        return
      }

      if (await tryConnect(state.accessToken)) {
        dispatch({ type: 'SET_AUTHENTICATED', value: true })
        dispatch({ type: 'SET_AUTH', show: false, mode: 'login', title: '', desc: '' })
      } else {
        localStorage.removeItem('access_token')
        localStorage.removeItem('refresh_token')
        dispatch({ type: 'CLEAR_TOKENS' })
        dispatch({
          type: 'SET_AUTH',
          show: true,
          mode: 'login',
          title: 'Login',
          desc: 'Session expired — enter your password.',
        })
      }
    })()
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <>
      {state.showAuth && <AuthScreen />}
      <div id="app" className={state.showAuth ? 'hidden' : ''}>
        <TopBar />
        <div className={'tab-content' + (state.activeTab === 'terminal' ? ' active' : '')} id="tab-terminal">
          <Terminal />
        </div>
        <div className={'tab-content' + (state.activeTab === 'dashboard' ? ' active' : '')} id="tab-dashboard">
          <Dashboard />
        </div>
        {state.interactiveTabs.map(tab => (
          <div
            key={tab.pid}
            className={'tab-content' + (state.activeTab === 'repl-' + tab.pid ? ' active' : '')}
            id={'tab-repl-' + tab.pid}
          >
            <InteractiveTab tab={tab} buffer={streamBufsRef.current.get(tab.pid) || []} streamVersion={streamVersRef.current.get(tab.pid) || 0} sendToAgent={sendToAgent} resizeAgent={resizeAgent} />
          </div>
        ))}
      </div>
      <AgentModal />
    </>
  )
}

function parseJWT(token: string): { exp?: number } | null {
  try {
    const parts = token.split('.')
    return JSON.parse(atob(parts[1]))
  } catch {
    return null
  }
}
