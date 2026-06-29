import { useRef, useCallback, useEffect } from 'react'
import { Terminal as XtermTerminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { useStore } from '../state'
import { useWebSocket } from '../hooks/useWebSocket'
import type { StreamData } from '../types'
import { filterTerminalQueries } from '../utils/filterTerminalQueries'
import { ReplShell } from '../replShell'
import '@xterm/xterm/css/xterm.css'

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

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const term = new XtermTerminal({
      cursorBlink: true,
      convertEol: true,
      fontSize: 13,
      fontFamily: "'Courier New', monospace",
      theme: {
        background: '#0d1117',
        foreground: '#c9d1d9',
        cursor: '#58a6ff',
        selectionBackground: '#264f78',
        black: '#484f58',
        red: '#f85149',
        green: '#7ee787',
        yellow: '#d29922',
        blue: '#58a6ff',
        magenta: '#bc8cff',
        cyan: '#39c5cf',
        white: '#c9d1d9',
        brightBlack: '#6e7681',
        brightRed: '#ff7b72',
        brightGreen: '#7ee787',
        brightYellow: '#d29922',
        brightBlue: '#58a6ff',
        brightMagenta: '#bc8cff',
        brightCyan: '#39c5cf',
        brightWhite: '#f0f6fc',
      },
    })

    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(container)
    fit.fit()

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

    const ro = new ResizeObserver(() => {
      try { fit.fit() } catch { /* ignore */ }
      if (shell.busy) {
        sendRef.current(JSON.stringify({ resize: true, cols: term.cols, rows: term.rows }))
      }
    })
    ro.observe(container)

    return () => {
      disposeData.dispose()
      ro.disconnect()
      term.dispose()
    }
  }, [])

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div ref={containerRef} className="terminal" id="term-container" style={{ flex: 1 }} />
    </div>
  )
}
