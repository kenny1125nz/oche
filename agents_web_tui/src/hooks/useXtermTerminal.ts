import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { xtermTheme } from '../theme'
import '@xterm/xterm/css/xterm.css'

interface UseXtermTerminalOptions {
  containerRef: React.RefObject<HTMLDivElement | null>
  onData?: (data: string) => void
  onResize?: (cols: number, rows: number) => void
  fontSize?: number
}

export function useXtermTerminal({ containerRef, onData, onResize, fontSize = 13 }: UseXtermTerminalOptions) {
  const termRef = useRef<Terminal | null>(null)
  const fitRef = useRef<FitAddon | null>(null)

  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const term = new Terminal({
      cursorBlink: true,
      convertEol: true,
      fontSize,
      fontFamily: "'Courier New', monospace",
      theme: xtermTheme,
    })

    const fit = new FitAddon()
    term.loadAddon(fit)
    term.open(container)
    fit.fit()
    term.focus()

    termRef.current = term
    fitRef.current = fit

    const disposeData = onData
      ? term.onData(onData)
      : { dispose: () => {} }

    const ro = new ResizeObserver(() => {
      try { fit.fit() } catch { /* ignore */ }
      if (onResize && termRef.current) {
        try { onResize(termRef.current.cols, termRef.current.rows) } catch { /* ignore */ }
      }
    })
    ro.observe(container)

    return () => {
      disposeData.dispose()
      ro.disconnect()
      term.dispose()
    }
  }, [])

  return { termRef, fitRef }
}
