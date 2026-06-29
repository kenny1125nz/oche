import { useState, useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { useStore } from '../state'
import type { InteractiveTab as InteractiveTabType } from '../types'
import { filterTerminalQueries } from '../utils/filterTerminalQueries'
import '@xterm/xterm/css/xterm.css'

interface Props {
  tab: InteractiveTabType
  buffer: string[]
  streamVersion: number
  sendToAgent: (pid: number, text: string) => void
  resizeAgent: (pid: number, cols: number, rows: number) => void
}

export function InteractiveTab({ tab, buffer, streamVersion, sendToAgent, resizeAgent }: Props) {
  const { state } = useStore()
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)
  const [input, setInput] = useState('')
  const renderedRef = useRef(0)

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const term = new Terminal({
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
    term.focus()
    resizeAgent(tab.pid, term.cols, term.rows)

    termRef.current = term
    fitRef.current = fit

    const disposeData = term.onData((data: string) => {
      sendToAgent(tab.pid, data)
    })

    const ro = new ResizeObserver(() => {
      try { fit.fit() } catch { /* ignore */ }
      if (termRef.current) {
        try {
          resizeAgent(tab.pid, termRef.current.cols, termRef.current.rows)
        } catch { /* ignore */ }
      }
    })
    ro.observe(container)

    return () => {
      disposeData.dispose()
      ro.disconnect()
      term.dispose()
    }
  }, [tab.pid, sendToAgent])

  useEffect(() => {
    if (state.activeTab === 'repl-' + tab.pid && fitRef.current && termRef.current) {
      try { fitRef.current.fit() } catch { /* ignore */ }
      try { termRef.current.focus() } catch { /* ignore */ }
      try { termRef.current.refresh(0, termRef.current.rows - 1) } catch { /* ignore */ }
    }
  }, [state.activeTab, tab.pid])

  useEffect(() => {
    if (!termRef.current) return
    const term = termRef.current
    const newLines = buffer.slice(renderedRef.current)
    for (const line of newLines) {
      const cleaned = filterTerminalQueries(line, (response) => {
        sendToAgent(tab.pid, response)
      })
      if (cleaned) {
        term.write(cleaned)
      }
    }
    renderedRef.current = buffer.length
  }, [buffer, streamVersion, tab.pid])

  const handleSubmit = () => {
    if (!input.trim()) return
    const text = input
    setInput('')
    termRef.current?.write(text + '\r\n')
    sendToAgent(tab.pid, text + '\r')
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleSubmit()
  }

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
      <div ref={containerRef} style={{ flex: 1 }} />
      <div style={{ display: 'flex', alignItems: 'center', padding: '6px 16px', borderTop: '1px solid #30363d', background: '#161b22', flexShrink: 0 }}>
        <span style={{ color: '#58a6ff', marginRight: 8, fontFamily: 'monospace' }}>&gt;</span>
        <input
          style={{ flex: 1, background: 'transparent', border: 'none', color: '#c9d1d9', fontFamily: 'monospace', fontSize: 13, outline: 'none' }}
          type="text"
          placeholder="Type and press Enter to send..."
          value={input}
          onChange={e => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
        />
        <button
          style={{ marginLeft: 8, padding: '2px 12px', background: '#238636', color: '#fff', border: 'none', borderRadius: 4, cursor: 'pointer', fontSize: 12 }}
          onClick={handleSubmit}
        >Send</button>
      </div>
    </div>
  )
}
