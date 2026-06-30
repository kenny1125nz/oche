import { BASE } from './config'
import type { ActiveAgent } from '../types'

export async function fetchActiveAgents(accessToken: string): Promise<ActiveAgent[]> {
  const res = await fetch(BASE + '/api/agents/active', {
    headers: { 'Authorization': 'Bearer ' + accessToken },
  })
  if (!res.ok) return []
  return res.json()
}

export async function sendAgentInput(pid: number, input: string, accessToken: string): Promise<void> {
  await fetch(BASE + '/api/agent/' + pid + '/input', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + accessToken,
    },
    body: JSON.stringify({ input }),
  })
}
