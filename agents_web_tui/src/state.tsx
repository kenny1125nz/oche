import { createContext, useContext, useReducer, type ReactNode } from 'react'

export interface AppState {
  accessToken: string | null
  refreshToken: string | null
  wsSessionId: string | null
  activeTab: string
  currentAgent: string
  currentWorkspace: string
  currentSubWorkspace: string
  currentActiveFile: string
  activeCount: number
  queuedCount: number
  sessions: import('./types').Session[]
  activeAgents: import('./types').ActiveAgent[]
  interactiveTabs: import('./types').InteractiveTab[]
  modalOpen: boolean
  modalPid: number | null
  modalBinary: string
  modalSessionId: string
  authenticated: boolean
  authLoading: boolean
  authError: string
  authMode: 'login' | 'setup'
  authTitle: string
  authDesc: string
  showAuth: boolean
}

type Action =
  | { type: 'SET_TOKENS'; accessToken: string; refreshToken?: string }
  | { type: 'SET_WS_SESSION'; sessionId: string }
  | { type: 'CLEAR_TOKENS' }
  | { type: 'SET_TAB'; tab: string }
  | { type: 'SET_CURRENT_AGENT'; agent: string }
  | { type: 'SET_CURRENT_WORKSPACE'; workspace: string }
  | { type: 'SET_CURRENT_SUB_WORKSPACE'; sub: string }
  | { type: 'SET_CURRENT_ACTIVE_FILE'; file: string }
  | { type: 'SET_POOL'; active: number; queued: number }
  | { type: 'SET_SESSIONS'; sessions: import('./types').Session[] }
  | { type: 'SET_ACTIVE_AGENTS'; agents: import('./types').ActiveAgent[] }
  | { type: 'OPEN_INTERACTIVE_TAB'; tab: import('./types').InteractiveTab }
  | { type: 'CLOSE_INTERACTIVE_TAB'; pid: number }
  | { type: 'OPEN_MODAL'; pid: number; binary: string; sessionId: string }
  | { type: 'CLOSE_MODAL' }
  | { type: 'SET_AUTH'; show: boolean; mode: 'login' | 'setup'; title: string; desc: string }
  | { type: 'SET_AUTH_ERROR'; error: string }
  | { type: 'SET_AUTHENTICATED'; value: boolean }

function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
    case 'SET_TOKENS':
      return { ...state, accessToken: action.accessToken, refreshToken: action.refreshToken ?? state.refreshToken }
    case 'SET_WS_SESSION':
      return { ...state, wsSessionId: action.sessionId }
    case 'CLEAR_TOKENS':
      return { ...state, accessToken: null, refreshToken: null }
    case 'SET_TAB':
      return { ...state, activeTab: action.tab }
    case 'SET_CURRENT_AGENT':
      return { ...state, currentAgent: action.agent }
    case 'SET_CURRENT_WORKSPACE':
      return { ...state, currentWorkspace: action.workspace }
    case 'SET_CURRENT_SUB_WORKSPACE':
      return { ...state, currentSubWorkspace: action.sub }
    case 'SET_CURRENT_ACTIVE_FILE':
      return { ...state, currentActiveFile: action.file }
    case 'SET_POOL':
      return { ...state, activeCount: action.active, queuedCount: action.queued }
    case 'SET_SESSIONS':
      return { ...state, sessions: action.sessions }
    case 'SET_ACTIVE_AGENTS':
      return { ...state, activeAgents: action.agents }
    case 'OPEN_INTERACTIVE_TAB':
      return { ...state, interactiveTabs: [...state.interactiveTabs, action.tab], activeTab: 'repl-' + action.tab.pid }
    case 'CLOSE_INTERACTIVE_TAB':
      return {
        ...state,
        interactiveTabs: state.interactiveTabs.filter(t => t.pid !== action.pid),
        activeTab: state.activeTab === 'repl-' + action.pid ? 'dashboard' : state.activeTab,
      }
    case 'OPEN_MODAL':
      return { ...state, modalOpen: true, modalPid: action.pid, modalBinary: action.binary, modalSessionId: action.sessionId }
    case 'CLOSE_MODAL':
      return { ...state, modalOpen: false, modalPid: null }
    case 'SET_AUTH':
      return { ...state, showAuth: action.show, authMode: action.mode, authTitle: action.title, authDesc: action.desc, authError: '' }
    case 'SET_AUTH_ERROR':
      return { ...state, authError: action.error }
    case 'SET_AUTHENTICATED':
      return { ...state, authenticated: action.value }
    default:
      return state
  }
}

const initialState: AppState = {
  accessToken: localStorage.getItem('access_token'),
  refreshToken: localStorage.getItem('refresh_token'),
  wsSessionId: null,
  activeTab: 'terminal',
  currentAgent: localStorage.getItem('current_agent') || '—',
  currentWorkspace: localStorage.getItem('current_workspace') || '—',
  currentSubWorkspace: '',
  currentActiveFile: '',
  activeCount: 0,
  queuedCount: 0,
  sessions: [],
  activeAgents: [],
  interactiveTabs: [],
  modalOpen: false,
  modalPid: null,
  modalBinary: '',
  modalSessionId: '',
  authenticated: false,
  authLoading: false,
  authError: '',
  authMode: 'login',
  authTitle: 'Welcome',
  authDesc: '',
  showAuth: true,
}

const StoreContext = createContext<{ state: AppState; dispatch: React.Dispatch<Action> }>({
  state: initialState,
  dispatch: () => {},
})

export function StoreProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState)
  return <StoreContext.Provider value={{ state, dispatch }}>{children}</StoreContext.Provider>
}

export function useStore() {
  return useContext(StoreContext)
}
