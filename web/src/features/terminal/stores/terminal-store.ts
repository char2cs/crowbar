import { create } from 'zustand'
import type { Terminal } from '@/features/terminal/types/terminal'

export type TerminalWidthMode = 'full' | 'editor'
export type TerminalTabLayout = 'horizontal' | 'vertical'
export type TerminalTabSidebarPosition = 'left' | 'right'

interface TerminalStore {
  sessions: Map<string, Partial<Terminal>>
  widthMode: TerminalWidthMode
  tabLayout: TerminalTabLayout
  tabSidebarWidth: number
  tabSidebarPosition: TerminalTabSidebarPosition
  updateSession: (sessionId: string, updates: Partial<Terminal>) => void
  getSession: (sessionId: string) => Partial<Terminal> | undefined
  removeSession: (sessionId: string) => void
  setWidthMode: (mode: TerminalWidthMode) => void
  setTabLayout: (layout: TerminalTabLayout) => void
  setTabSidebarWidth: (width: number) => void
  setTabSidebarPosition: (position: TerminalTabSidebarPosition) => void
}

export const useTerminalStore = create<TerminalStore>()((set, get) => ({
  sessions: new Map(),
  widthMode: 'editor',
  tabLayout: 'horizontal',
  tabSidebarWidth: 180,
  tabSidebarPosition: 'left',

  updateSession: (sessionId: string, updates: Partial<Terminal>) => {
    set((state) => {
      const current = state.sessions.get(sessionId)
      if (current) {
        // OSC7 cwd + title events fire on every PTY write callback and often
        // report a value identical to what's already stored. Skip the Map
        // reallocation entirely in that case — a new Map reference is what
        // fans out and re-renders every subscriber (e.g. every tab label),
        // so a genuine no-op must not produce one.
        let changed = false
        for (const k of Object.keys(updates) as (keyof Terminal)[]) {
          if (!Object.is(current[k], updates[k])) {
            changed = true
            break
          }
        }
        if (!changed) return state
      }
      const newSessions = new Map(state.sessions)
      newSessions.set(sessionId, { ...(current || {}), ...updates })
      return { sessions: newSessions }
    })
  },

  getSession: (sessionId: string) => {
    return get().sessions.get(sessionId)
  },

  removeSession: (sessionId: string) => {
    set((state) => {
      const newSessions = new Map(state.sessions)
      newSessions.delete(sessionId)
      return { sessions: newSessions }
    })
  },

  setWidthMode: (mode: TerminalWidthMode) => {
    set({ widthMode: mode })
  },

  setTabLayout: (tabLayout: TerminalTabLayout) => {
    set({ tabLayout })
  },

  setTabSidebarWidth: (tabSidebarWidth: number) => {
    set({ tabSidebarWidth: Math.max(80, Math.min(400, tabSidebarWidth)) })
  },

  setTabSidebarPosition: (tabSidebarPosition: TerminalTabSidebarPosition) => {
    set({ tabSidebarPosition })
  },
}))
