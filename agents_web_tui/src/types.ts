export interface StreamData {
  type?: string
  session_id?: string
  text?: string
  data?: string
  workspace_id?: string
  agent_id?: string
}

export interface Session {
  id: string
  active: boolean
  workspace_id?: string
  sub_workspace?: string
  active_file?: string
  agent_id?: string
}

export interface DashboardData {
  type: 'dashboard'
  pool: { active: number; queued: number }
  sessions: Session[]
}

export interface ActiveAgent {
  pid: number
  binary: string
  mode: string
  session_id: string
  dir?: string
  state?: string
  started_at?: string
}

export interface InteractiveTab {
  pid: number
  binary: string
  sessionId: string
  mode: string
  createdAt: number
}

export interface AuthStatus {
  password_set: boolean
}

export interface AuthResponse {
  access_token: string
  refresh_token?: string
}
