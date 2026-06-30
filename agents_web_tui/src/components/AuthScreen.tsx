import { useState, useCallback } from 'react'
import { useStore } from '../state'
import { checkStatus, setupPassword, login, tryConnect } from '../api/auth'
import type { AuthResponse } from '../types'

export function AuthScreen() {
  const { state, dispatch } = useStore()
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const authenticate = useCallback(async (apiCall: () => Promise<AuthResponse>, failMsg: string) => {
    setLoading(true)
    try {
      const data = await apiCall()
      dispatch({ type: 'SET_TOKENS', accessToken: data.access_token, refreshToken: data.refresh_token })
      localStorage.setItem('access_token', data.access_token)
      if (data.refresh_token) localStorage.setItem('refresh_token', data.refresh_token)

      if (await tryConnect(data.access_token)) {
        dispatch({ type: 'SET_AUTHENTICATED', value: true })
        dispatch({ type: 'SET_AUTH', show: false, mode: 'login', title: '', desc: '' })
      }
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : failMsg)
    } finally {
      setLoading(false)
    }
  }, [dispatch])

  const handleSubmit = async () => {
    setError('')

    if (state.authMode === 'setup') {
      if (password !== confirm) {
        setError('Passwords do not match')
        return
      }
      if (password.length < 4) {
        setError('Password too short')
        return
      }
      await authenticate(() => setupPassword(password), 'Setup failed')
      return
    }

    await authenticate(() => login(password), 'Login failed')
  }

  return (
    <div id="auth-screen">
      <div className="auth-box">
        <h1 id="auth-title">{state.authTitle}</h1>
        <p id="auth-desc">{state.authDesc}</p>
        <div id="auth-error" className="auth-error">{error}</div>
        <input
          type="password"
          placeholder="Password"
          autoComplete="off"
          value={password}
          onChange={e => setPassword(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && handleSubmit()}
        />
        {state.authMode === 'setup' && (
          <input
            type="password"
            placeholder="Confirm password"
            autoComplete="off"
            className={state.authMode === 'setup' ? '' : 'hidden'}
            value={confirm}
            onChange={e => setConfirm(e.target.value)}
            onKeyDown={e => e.key === 'Enter' && handleSubmit()}
          />
        )}
        <button onClick={handleSubmit} disabled={loading}>
          {loading ? '...' : state.authMode === 'setup' ? 'Set Password' : 'Login'}
        </button>
      </div>
    </div>
  )
}
