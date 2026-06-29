import { useEffect, useRef } from 'react'
import type { DashboardData } from '../types'

const BASE = ''

export function useSSE(accessToken: string | null, onDashboard: (data: DashboardData) => void) {
  const esRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!accessToken) return

    if (esRef.current) esRef.current.close()

    const url = BASE + '/sse/dashboard?token=' + encodeURIComponent(accessToken)
    const es = new EventSource(url)
    esRef.current = es

    es.onmessage = (e) => {
      let data: DashboardData
      try {
        data = JSON.parse(e.data)
      } catch {
        return
      }
      if (data.type !== 'dashboard') return
      onDashboard(data)
    }

    return () => {
      es.close()
    }
  }, [accessToken, onDashboard])
}
