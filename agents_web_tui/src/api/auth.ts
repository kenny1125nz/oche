import type { AuthStatus, AuthResponse } from '../types'

const BASE = ''

export async function checkStatus(): Promise<AuthStatus> {
  const res = await fetch(BASE + '/auth/status')
  return res.json()
}

export async function setupPassword(password: string): Promise<AuthResponse> {
  const res = await fetch(BASE + '/auth/setup', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  })
  if (!res.ok) {
    const e = await res.json()
    throw new Error(e.error || 'Setup failed')
  }
  return res.json()
}

export async function login(password: string): Promise<AuthResponse> {
  const res = await fetch(BASE + '/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ password }),
  })
  if (!res.ok) {
    const e = await res.json()
    throw new Error(e.error || 'Login failed')
  }
  return res.json()
}

export async function refreshAccess(refreshToken: string): Promise<AuthResponse> {
  const res = await fetch(BASE + '/auth/refresh', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken }),
  })
  if (!res.ok) throw new Error('Refresh failed')
  return res.json()
}

export async function tryConnect(accessToken: string): Promise<boolean> {
  const res = await fetch(BASE + '/api/pool?token=' + encodeURIComponent(accessToken))
  return res.ok
}
