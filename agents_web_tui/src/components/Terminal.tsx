import { useRef, useCallback, useEffect } from 'react'
import { useStore } from '../state'
import { useWebSocket } from '../hooks/useWebSocket'
import { useXtermTerminal } from '../hooks/useXtermTerminal'
import type { StreamData } from '../types'
import { filterTerminalQueries } from '../utils/filterTerminalQueries'
import { ReplShell } from '../replShell'

function getPrompt(agent: string, workspace: string, subWorkspace: string): string {
  const a = agent !== '—' ? agent : 'agents'
  const w = workspace !== '—' ? workspace : '~'
  const path = subWorkspace ? w + '/' + subWorkspace : w
  return `${a}@${path} > `
}

export function Terminal() {
  const { state, dispatch } = useStore()
  const containerRef = useRef<HTMLDivElement>(null)
  const shellRef = useRef<ReplShell | null>(null)
  const sendRef = useRef<(msg: string) => void>(() => {})
  const termWriteRef = useRef<(d: string) => void>(() => {})

  useEffect(() => {
    const prompt = getPrompt(state.currentAgent, state.currentWorkspace, state.currentSubWorkspace)
    shellRef.current?.rewritePrompt(prompt)
  }, [state.currentAgent, state.currentWorkspace, state.currentSubWorkspace])

  const handleData = useCallback((data: StreamData) => {
    if (data.session_id && data.session_id.startsWith('repl-')) return
    const text = data.text || ''
    if (text) {
      const converted = text.replace(/\n/g, '\r\n')
      const filtered = filterTerminalQueries(converted, (response) => {
        sendRef.current(JSON.stringify({ input: response }))
      })
      if (filtered) {
        termWriteRef.current(filtered)
      }
    }
  }, [])

  const handleFinalize = useCallback(() => {
    shellRef.current?.handleFinalize()
  }, [])

  const handleSessionId = useCallback((sessionId: string) => {
    if (sessionId.startsWith('repl-')) {
      const shell = shellRef.current
      if (shell) shell.pendingRepl = { sessionId }
    } else {
      dispatch({ type: 'SET_WS_SESSION', sessionId })
    }
  }, [dispatch])

  const handleSessionState = useCallback((workspaceId: string, agentId: string) => {
    if (workspaceId) {
      localStorage.setItem('current_workspace', workspaceId)
      dispatch({ type: 'SET_CURRENT_WORKSPACE', workspace: workspaceId })
    }
    if (agentId) {
      localStorage.setItem('current_agent', agentId)
      dispatch({ type: 'SET_CURRENT_AGENT', agent: agentId })
    }
  }, [dispatch])

  const handleSpawned = useCallback((pid: number) => {
    shellRef.current?.handleSpawned(pid, dispatch)
  }, [dispatch])

  const { send: sendFn } = useWebSocket({
    accessToken: state.accessToken,
    onData: handleData,
    onFinalize: handleFinalize,
    onSessionId: handleSessionId,
    onSessionState: handleSessionState,
    onSpawned: handleSpawned,
    enabled: state.authenticated,
  })

  sendRef.current = sendFn

  const { termRef } = useXtermTerminal({
    containerRef,
    fontSize: 13,
    onResize: (cols, rows) => {
      if (shellRef.current?.busy) {
        sendRef.current(JSON.stringify({ resize: true, cols, rows }))
      }
    },
  })

  useEffect(() => {
    const term = termRef.current
    if (!term) return

    termWriteRef.current = (d) => term.write(d)

    const shell = new ReplShell(
      {
        write: (d) => term.write(d),
        clear: () => term.clear(),
        get cols() { return term.cols },
        get rows() { return term.rows },
      },
      (msg) => sendRef.current(msg),
    )
    shell.prompt = getPrompt(state.currentAgent, state.currentWorkspace, state.currentSubWorkspace)
    shellRef.current = shell

    term.write(shell.prompt)
    term.focus()

    const disposeData = term.onData((data) => shell.handleTerminalData(data))

    return () => {
      disposeData.dispose()
    }
  }, [])

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div ref={containerRef} className="terminal" id="term-container" style={{ flex: 1 }} />
    </div>
  )
}
