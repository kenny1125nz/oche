import { useEffect, useRef, useCallback } from 'react'
import type { InteractiveTab } from '../types'

export function useAgentWS(
  tabs: InteractiveTab[],
  accessToken: string | null,
  onOutput: (pid: number, text: string) => void,
  onExit: (pid: number) => void,
) {
  const wsRef = useRef<Map<number, WebSocket>>(new Map())
  const callbackRef = useRef({ onOutput, onExit })
  callbackRef.current = { onOutput, onExit }

  const tabKey = tabs.map(t => t.pid).sort().join(',')

  useEffect(() => {
    if (!accessToken) return

    const currentPids = new Set(tabs.map(t => t.pid))

    for (const [pid, socket] of wsRef.current) {
      if (!currentPids.has(pid)) {
        socket.close()
        wsRef.current.delete(pid)
      }
    }

    for (const tab of tabs) {
      if (wsRef.current.has(tab.pid)) continue

      const wsUrl = window.location.origin.replace('http', 'ws') + '/ws/agent/' + tab.pid + '?token=' + encodeURIComponent(accessToken)
      const socket = new WebSocket(wsUrl)

      socket.onopen = () => {
      }

      socket.onmessage = (e) => {
        let data: { text?: string; exit?: boolean }
        try {
          data = JSON.parse(e.data)
        } catch {
          return
        }
        if (data.exit) {
          socket.close()
          wsRef.current.delete(tab.pid)
          callbackRef.current.onExit(tab.pid)
          return
        }
        if (data.text) {
          callbackRef.current.onOutput(tab.pid, data.text)
        }
      }

      socket.onerror = (evt) => {
        socket.close()
        wsRef.current.delete(tab.pid)
      }

      socket.onclose = () => {
        wsRef.current.delete(tab.pid)
      }

      wsRef.current.set(tab.pid, socket)
    }

    return () => {
      for (const [, socket] of wsRef.current) {
        socket.close()
      }
      wsRef.current.clear()
    }
  }, [tabKey, accessToken])

  const sendToAgent = useCallback((pid: number, text: string) => {
    const socket = wsRef.current.get(pid)
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    socket.send(JSON.stringify({ input: text }))
  }, [])

  const resizeAgent = useCallback((pid: number, cols: number, rows: number) => {
    const socket = wsRef.current.get(pid)
    if (!socket || socket.readyState !== WebSocket.OPEN) return
    socket.send(JSON.stringify({ resize: true, cols, rows }))
  }, [])

  return { sendToAgent, resizeAgent }
}
