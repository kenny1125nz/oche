import type { ActiveAgent } from '../types'

const BASE = ''

export async function fetchActiveAgents(accessToken: string): Promise<ActiveAgent[]> {
  const res = await fetch(BASE + '/api/agents/active?token=' + encodeURIComponent(accessToken))
  if (!res.ok) return []
  return res.json()
}

export async function sendAgentInput(pid: number, input: string, accessToken: string): Promise<void> {
  await fetch(BASE + '/api/agent/' + pid + '/input?token=' + encodeURIComponent(accessToken), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ input }),
  })
}
