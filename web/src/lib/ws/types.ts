export interface WorkspaceEvent {
  workspaceId: string
  action: string
}
export interface GitEvent {
  repo: string
  changed: boolean
}
export interface FileEvent {
  workspaceId: string
  path: string
}
export interface ChatChunk {
  chatId: string
  content: string
  done: boolean
}
export interface TerminalFrame {
  sessionId: string
  data: string
  isInput: boolean
}
export interface DaemonStatus {
  status: string
  version?: string
}
