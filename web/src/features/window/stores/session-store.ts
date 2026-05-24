// Stub
import { create } from "zustand"

export interface BufferSession {
  bufferId: string
  filePath?: string
  scrollPosition?: number
  cursorPosition?: { line: number; character: number }
}

export const useSessionStore = create(() => ({ sessionId: null as string | null, isLoaded: false }))
