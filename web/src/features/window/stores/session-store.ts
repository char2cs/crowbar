// Stub
import { create } from "zustand"

export interface BufferSession {
  type?: string
  bufferId?: string
  path?: string
  name?: string
  isPinned?: boolean
  isPreview?: boolean
  workspaceScope?: unknown
  editorState?: unknown
  sessionId?: string
  initialCommand?: string
  workingDirectory?: string
  remoteConnectionId?: unknown
  url?: string
  zoomLevel?: number
  profileKey?: string
  history?: unknown[]
  historyIndex?: number
  filePath?: string
  scrollPosition?: number
  cursorPosition?: { line: number; character: number }
}

interface SessionState {
  sessionId: string | null
  isLoaded: boolean
  saveSession: (projectPath: string, buffers: BufferSession[], activeBufferPath: string | null) => void
}

export const useSessionStore = create<SessionState>(() => ({
  sessionId: null,
  isLoaded: false,
  saveSession: (_projectPath, _buffers, _activeBufferPath) => {},
}))
