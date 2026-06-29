type ReplDataHandler = (pid: number, text: string) => void
type ReplExitHandler = (pid: number) => void

const sessionToPid = new Map<string, number>()
let onData: ReplDataHandler | null = null
let onExit: ReplExitHandler | null = null

export function registerReplSession(sessionId: string, pid: number) {
  sessionToPid.set(sessionId, pid)
}

export function unregisterReplSession(sessionId: string) {
  sessionToPid.delete(sessionId)
}

export function setReplDataHandler(handler: ReplDataHandler | null) {
  onData = handler
}

export function setReplExitHandler(handler: ReplExitHandler | null) {
  onExit = handler
}

export function pushReplData(sessionId: string, text: string) {
  const pid = sessionToPid.get(sessionId)
  if (pid && onData) {
    onData(pid, text)
  }
}

export function pushReplExit(sessionId: string) {
  const pid = sessionToPid.get(sessionId)
  if (pid && onExit) {
    onExit(pid)
    sessionToPid.delete(sessionId)
  }
}
