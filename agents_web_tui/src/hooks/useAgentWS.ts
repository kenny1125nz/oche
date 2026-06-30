import { useEffect, useRef, useCallback } from 'react'
import type { InteractiveTab } from '../types'

type ConnectionProtocol = 'ws' | 'sse'

interface UseAgentConnectionOptions {
  tabs: InteractiveTab[]
  accessToken: string | null
  onOutput: (pid: number, text: string) => void
  onExit: (pid: number) => void
  protocol: ConnectionProtocol
}

function buildConnection(tab: InteractiveTab, accessToken: string, protocol: ConnectionProtocol): WebSocket | EventSource {
  if (protocol === 'ws') {
    const wsUrl = window.location.origin.replace('http', 'ws') + '/ws/agent/' + tab.pid + '?token=' + encodeURIComponent(accessToken)
    return new WebSocket(wsUrl)
  }
  const url = '/api/agent/' + tab.pid + '/stream?token=' + encodeURIComponent(accessToken)
  return new EventSource(url)
}

function isWebSocket(conn: WebSocket | EventSource): conn is WebSocket {
  return conn instanceof WebSocket
}

function onMessage(conn: WebSocket | EventSource, handler: (data: any) => void) {
  if (isWebSocket(conn)) {
    conn.onmessage = (e) => {
      try { handler(JSON.parse(e.data)) } catch { /* ignore */ }
    }
  } else {
    conn.onmessage = (e) => {
      try { handler(JSON.parse(e.data)) } catch { /* ignore */ }
    }
  }
}

function onError(conn: WebSocket | EventSource, handler: () => void) {
  if (isWebSocket(conn)) {
    conn.onerror = handler
  } else {
    conn.onerror = handler
  }
}

function closeConnection(conn: WebSocket | EventSource) {
  if (isWebSocket(conn)) {
    conn.close()
  } else {
    conn.close()
  }
}

export function useAgentConnection(
  tabs: InteractiveTab[],
  accessToken: string | null,
  onOutput: (pid: number, text: string) => void,
  onExit: (pid: number) => void,
  protocol: ConnectionProtocol = 'sse',
) {
  const connRef = useRef<Map<number, WebSocket | EventSource>>(new Map())
  const callbackRef = useRef({ onOutput, onExit })
  callbackRef.current = { onOutput, onExit }

  const tabKey = tabs.map(t => t.pid).sort().join(',')

  useEffect(() => {
    if (!accessToken) return

    const currentPids = new Set(tabs.map(t => t.pid))

    for (const [pid, conn] of connRef.current) {
      if (!currentPids.has(pid)) {
        closeConnection(conn)
        connRef.current.delete(pid)
      }
    }

    for (const tab of tabs) {
      if (connRef.current.has(tab.pid)) continue

      const conn = buildConnection(tab, accessToken, protocol)
      const pid = tab.pid

      onMessage(conn, (data) => {
        if (data.exit || data.type === 'exit') {
          closeConnection(conn)
          connRef.current.delete(pid)
          callbackRef.current.onExit(pid)
          return
        }
        if (data.text) {
          callbackRef.current.onOutput(pid, data.text)
        }
      })

      onError(conn, () => {
        closeConnection(conn)
        connRef.current.delete(pid)
      })

      if (isWebSocket(conn)) {
        conn.onclose = () => {
          connRef.current.delete(pid)
        }
      }

      connRef.current.set(pid, conn)
    }

    return () => {
      for (const [, conn] of connRef.current) {
        closeConnection(conn)
      }
      connRef.current.clear()
    }
  }, [tabKey, accessToken])

  const sendToAgent = useCallback((pid: number, text: string) => {
    const conn = connRef.current.get(pid)
    if (!conn || !isWebSocket(conn) || conn.readyState !== WebSocket.OPEN) return
    conn.send(JSON.stringify({ input: text }))
  }, [])

  const resizeAgent = useCallback((pid: number, cols: number, rows: number) => {
    const conn = connRef.current.get(pid)
    if (!conn || !isWebSocket(conn) || conn.readyState !== WebSocket.OPEN) return
    conn.send(JSON.stringify({ resize: true, cols, rows }))
  }, [])

  return { sendToAgent, resizeAgent }
}

// Re-export with old name for backward compatibility
export const useAgentWS = useAgentConnection
