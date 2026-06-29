import { useRef, useCallback, useEffect } from 'react'
import { Terminal as XtermTerminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { useStore } from '../state'
import { useWebSocket } from '../hooks/useWebSocket'
import type { StreamData } from '../types'
import { filterTerminalQueries } from '../utils/filterTerminalQueries'
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
  const termRef = useRef<XtermTerminal | null>(null)
  const busyRef = useRef(false)
  const currentLineRef = useRef('')
  const cmdQueueRef = useRef<string[]>([])
  const promptRef = useRef(getPrompt(state.currentAgent, state.currentWorkspace, state.currentSubWorkspace))
  const sendRef = useRef<(msg: string) => void>(() => {})
  const pendingReplRef = useRef<{ sessionId: string } | null>(null)
  const interactiveRef = useRef(false)
  const historyRef = useRef<string[]>([])
  const historyIndexRef = useRef(-1)
  const cursorPosRef = useRef(0)

  useEffect(() => {
    const newPrompt = getPrompt(state.currentAgent, state.currentWorkspace, state.currentSubWorkspace)
    const oldPrompt = promptRef.current
    promptRef.current = newPrompt

    if (termRef.current && !busyRef.current && currentLineRef.current === '') {
      const max = Math.max(oldPrompt.length, newPrompt.length)
      termRef.current.write('\r' + ' '.repeat(max) + '\r' + newPrompt)
    }
  }, [state.currentAgent, state.currentWorkspace, state.currentSubWorkspace])

  const writePrompt = useCallback(() => {
    termRef.current?.write(promptRef.current)
  }, [])

  const handleData = useCallback((data: StreamData) => {
    if (data.session_id && data.session_id.startsWith('repl-')) return
    const text = data.text || ''
    if (text) {
      const converted = text.replace(/\n/g, '\r\n')
      const filtered = filterTerminalQueries(converted, (response) => {
        sendRef.current(JSON.stringify({ input: response }))
      })
      if (filtered) {
        termRef.current?.write(filtered)
      }
    }
  }, [])

  const handleFinalize = useCallback(() => {
    busyRef.current = false
    interactiveRef.current = false

    if (cmdQueueRef.current.length > 0) {
      termRef.current?.write('\r\n')
      const text = cmdQueueRef.current.shift()!
      busyRef.current = true
      sendRef.current(JSON.stringify({ input: text }))
      return
    }

    writePrompt()
  }, [writePrompt])

  const handleSessionId = useCallback((sessionId: string) => {
    if (sessionId.startsWith('repl-')) {
      pendingReplRef.current = { sessionId }
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
    const pending = pendingReplRef.current
    if (pending) {
      pendingReplRef.current = null
      dispatch({
        type: 'OPEN_INTERACTIVE_TAB',
        tab: {
          pid,
          binary: 'repl',
          sessionId: pending.sessionId,
          mode: 'pty',
          createdAt: Date.now(),
        },
      })
      busyRef.current = false
      interactiveRef.current = false
      writePrompt()
    }
  }, [dispatch, writePrompt])

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

  const sendResize = useCallback(() => {
    if (termRef.current) {
      sendRef.current(JSON.stringify({ resize: true, cols: termRef.current.cols, rows: termRef.current.rows }))
    }
  }, [])

  const sendCommand = useCallback((text: string) => {
    if (busyRef.current) {
      cmdQueueRef.current.push(text)
      return
    }
    if (text === 'clear') {
      termRef.current?.write('\r\n')
      termRef.current?.clear()
      writePrompt()
      return
    }
    if (text === 'repl') {
      busyRef.current = true
      termRef.current?.write('\r\n')
      sendRef.current(JSON.stringify({ input: text }))
      return
    }
    busyRef.current = true
    interactiveRef.current = text.startsWith('os ')
    termRef.current?.write('\r\n')
    sendResize()
    sendRef.current(JSON.stringify({ input: text }))
  }, [writePrompt, sendResize])

  const handleTerminalData = useCallback((data: string) => {
    if (busyRef.current && interactiveRef.current) {
      sendRef.current(JSON.stringify({ input: data }))
      return
    }
    if (busyRef.current) return

    if (data === '\r') {
      const line = currentLineRef.current
      currentLineRef.current = ''
      cursorPosRef.current = 0
      historyIndexRef.current = -1
      if (line.trim()) {
        historyRef.current.push(line)
        sendCommand(line)
      } else {
        termRef.current?.write('\r\n')
        writePrompt()
      }
      return
    }

    if (data === '\x7f' || data === '\b') {
      if (cursorPosRef.current > 0) {
        cursorPosRef.current--
        const before = currentLineRef.current.slice(0, cursorPosRef.current)
        const after = currentLineRef.current.slice(cursorPosRef.current + 1)
        currentLineRef.current = before + after
        const tail = currentLineRef.current.slice(cursorPosRef.current)
        termRef.current?.write('\b' + tail + ' ' + '\b'.repeat(tail.length + 1))
      }
      return
    }

    if (data === '\x1b[A' || data === '\x1bOA') {
      if (historyRef.current.length === 0) return
      if (historyIndexRef.current === -1) {
        historyIndexRef.current = historyRef.current.length - 1
      } else if (historyIndexRef.current > 0) {
        historyIndexRef.current--
      } else {
        return
      }
      const entry = historyRef.current[historyIndexRef.current]
      const max = currentLineRef.current.length + promptRef.current.length
      termRef.current?.write('\r' + ' '.repeat(max) + '\r')
      writePrompt()
      termRef.current?.write(entry)
      currentLineRef.current = entry
      cursorPosRef.current = entry.length
      return
    }

    if (data === '\x1b[B' || data === '\x1bOB') {
      if (historyIndexRef.current === -1) return
      const max = currentLineRef.current.length + promptRef.current.length
      if (historyIndexRef.current < historyRef.current.length - 1) {
        historyIndexRef.current++
        const entry = historyRef.current[historyIndexRef.current]
        termRef.current?.write('\r' + ' '.repeat(max) + '\r')
        writePrompt()
        termRef.current?.write(entry)
        currentLineRef.current = entry
        cursorPosRef.current = entry.length
      } else {
        historyIndexRef.current = -1
        termRef.current?.write('\r' + ' '.repeat(max) + '\r')
        writePrompt()
        currentLineRef.current = ''
        cursorPosRef.current = 0
      }
      return
    }

    if (data === '\x1b[D' || data === '\x1bOD') {
      if (cursorPosRef.current > 0) {
        cursorPosRef.current--
        termRef.current?.write('\b')
      }
      return
    }

    if (data === '\x1b[C' || data === '\x1bOC') {
      if (cursorPosRef.current < currentLineRef.current.length) {
        cursorPosRef.current++
        termRef.current?.write('\x1b[C')
      }
      return
    }

    if (data >= ' ') {
      if (cursorPosRef.current === currentLineRef.current.length) {
        currentLineRef.current += data
        termRef.current?.write(data)
      } else {
        const before = currentLineRef.current.slice(0, cursorPosRef.current)
        const after = currentLineRef.current.slice(cursorPosRef.current)
        currentLineRef.current = before + data + after
        termRef.current?.write(data + after + '\b'.repeat(after.length))
      }
      cursorPosRef.current++
    }
  }, [sendCommand, writePrompt])

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

    termRef.current = term

    term.write(promptRef.current)
    term.focus()

    const disposeData = term.onData(handleTerminalData)

    const ro = new ResizeObserver(() => {
      try { fit.fit() } catch { /* ignore */ }
      if (busyRef.current && termRef.current) {
        sendRef.current(JSON.stringify({ resize: true, cols: termRef.current.cols, rows: termRef.current.rows }))
      }
    })
    ro.observe(container)

    return () => {
      disposeData.dispose()
      ro.disconnect()
      term.dispose()
    }
  }, [handleTerminalData])

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div ref={containerRef} className="terminal" id="term-container" style={{ flex: 1 }} />
    </div>
  )
}


