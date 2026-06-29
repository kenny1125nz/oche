import { useEffect, useRef, useCallback } from 'react'
import type { StreamData } from '../types'

const BASE = ''

interface UseWebSocketOptions {
  accessToken: string | null
  onData: (data: StreamData) => void
  onFinalize: () => void
  onSpawned?: (pid: number) => void
  onSessionId?: (sessionId: string) => void
  onSessionState?: (workspaceId: string, agentId: string) => void
  enabled: boolean
}

export function useWebSocket({ accessToken, onData, onFinalize, onSpawned, onSessionId, onSessionState, enabled }: UseWebSocketOptions) {
  const wsRef = useRef<WebSocket | null>(null)
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>()

  const connect = useCallback(() => {
    if (!accessToken || !enabled) return
    if (wsRef.current) wsRef.current.close()

    const protocol = location.protocol === 'https:' ? 'wss://' : 'ws://'
    const url = protocol + location.host + BASE + '/shell?token=' + encodeURIComponent(accessToken)
    const ws = new WebSocket(url)
    wsRef.current = ws

    ws.onmessage = (e) => {
      let data: StreamData
      try {
        data = JSON.parse(e.data)
      } catch {
        return
      }

      if (data.type === 'deliverable' && data.text === 'done') {
        onFinalize()
        return
      }
      if (data.type === 'error') {
        onData(data)
        return
      }
      if (data.type === 'ack') {
        if (data.session_id && onSessionId) {
          onSessionId(data.session_id)
        }
        const pidMatch = data.text?.match(/\(pid (\d+)\)/)
        if (pidMatch && onSpawned) {
          onSpawned(parseInt(pidMatch[1], 10))
        }
        return
      }
      if (onSessionState && (data.workspace_id || data.agent_id)) {
        onSessionState(data.workspace_id || '', data.agent_id || '')
      }
      onData(data)
    }

    ws.onclose = () => {
      if (document.getElementById('app')?.classList.contains('hidden')) return
      reconnectTimer.current = setTimeout(connect, 3000)
    }
  }, [accessToken, enabled, onData, onFinalize, onSpawned, onSessionId, onSessionState])

  const send = useCallback((msg: string) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(msg)
    }
  }, [])

  useEffect(() => {
    connect()
    return () => {
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current)
      if (wsRef.current) wsRef.current.close()
    }
  }, [connect])

  return { send }
}
