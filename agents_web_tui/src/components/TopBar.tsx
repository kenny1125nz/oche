import { useStore } from '../state'

export function TopBar() {
  const { state, dispatch } = useStore()

  const handleLogout = () => {
    dispatch({ type: 'SET_AUTHENTICATED', value: false })
    dispatch({ type: 'CLEAR_TOKENS' })
    dispatch({ type: 'SET_AUTH', show: true, mode: 'login', title: 'Login', desc: 'Enter your password to connect.' })
    localStorage.removeItem('access_token')
    localStorage.removeItem('refresh_token')
  }

  const closeRepl = (pid: number) => {
    dispatch({ type: 'CLOSE_INTERACTIVE_TAB', pid })
    if (state.activeTab === 'repl-' + pid) {
      dispatch({ type: 'SET_TAB', tab: 'terminal' })
    }
  }

  return (
    <div className="topbar">
      <div className="tabs">
        <span
          className={'tab' + (state.activeTab === 'terminal' ? ' active' : '')}
          onClick={() => dispatch({ type: 'SET_TAB', tab: 'terminal' })}
        >
          Terminal
        </span>
        <span
          className={'tab' + (state.activeTab === 'dashboard' ? ' active' : '')}
          onClick={() => dispatch({ type: 'SET_TAB', tab: 'dashboard' })}
        >
          Dashboard
        </span>
        {state.interactiveTabs.map(tab => (
          <span key={tab.pid} className={'tab repl-tab' + (state.activeTab === 'repl-' + tab.pid ? ' active' : '')}>
              <span onClick={() => dispatch({ type: 'SET_TAB', tab: 'repl-' + tab.pid })}>
                {state.currentAgent}@{state.currentWorkspace}-{tab.pid}
              </span>
            <span className="tab-close" onClick={() => closeRepl(tab.pid)}>&times;</span>
          </span>
        ))}
      </div>
      <span style={{ flex: 1 }} />
      <span className="pool-badge" id="pool-badge">
        pool: {state.activeCount} active, {state.queuedCount} queued
      </span>
      <span className="badge" id="btn-logout" title="Log out" onClick={handleLogout}>
        logout
      </span>
    </div>
  )
}
