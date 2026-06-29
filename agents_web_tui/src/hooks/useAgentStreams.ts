import { useEffect, useRef } from 'react'
import type { InteractiveTab } from '../types'

export function useAgentStreams(
  tabs: InteractiveTab[],
  accessToken: string | null,
  onOutput: (pid: number, text: string) => void,
  onExit: (pid: number) => void,
) {
  const esRef = useRef<Map<number, EventSource>>(new Map())
  const callbackRef = useRef({ onOutput, onExit })
  callbackRef.current = { onOutput, onExit }

  const tabKey = tabs.map(t => t.pid).sort().join(',')

  useEffect(() => {
    if (!accessToken) return

    const currentPids = new Set(tabs.map(t => t.pid))

    for (const [pid, es] of esRef.current) {
      if (!currentPids.has(pid)) {
        es.close()
        esRef.current.delete(pid)
      }
    }

    for (const tab of tabs) {
      if (esRef.current.has(tab.pid)) continue

      const url = '/api/agent/' + tab.pid + '/stream?token=' + encodeURIComponent(accessToken)
      const es = new EventSource(url)

      es.onmessage = (e) => {
        let data: { type?: string; text?: string }
        try {
          data = JSON.parse(e.data)
        } catch {
          return
        }
        if (data.type === 'exit') {
          es.close()
          esRef.current.delete(tab.pid)
          callbackRef.current.onExit(tab.pid)
          return
        }
        if (data.text) {
          callbackRef.current.onOutput(tab.pid, data.text)
        }
      }

      es.onerror = () => {
        es.close()
        esRef.current.delete(tab.pid)
      }

      esRef.current.set(tab.pid, es)
    }

    return () => {
      for (const [, es] of esRef.current) {
        es.close()
      }
      esRef.current.clear()
    }
  }, [tabKey, accessToken])
}
