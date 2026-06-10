import { enableMapSet } from 'immer'
import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'

// Enable Immer's MapSet plugin so Set mutations work inside immer producers
enableMapSet()

export interface TerminalLayout {
  widthMode: 'full' | 'editor'
  tabLayout: 'horizontal' | 'vertical'
  tabSidebarWidth: number
  tabSidebarPosition: 'left' | 'right'
}

export interface TerminalActions {
  registerSession(sessionId: string): void
  unregisterSession(sessionId: string): void
  hasSession(sessionId: string): boolean
  setWidthMode(mode: 'full' | 'editor'): void
  setTabLayout(layout: 'horizontal' | 'vertical'): void
  setTabSidebarWidth(width: number): void
  setTabSidebarPosition(pos: 'left' | 'right'): void
}

export interface TerminalSlice {
  terminalSessionIds: Set<string>
  terminalLayout: TerminalLayout
  terminalActions: TerminalActions
}

export const createTerminalSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  TerminalSlice
> = (set, get) => ({
  terminalSessionIds: new Set<string>(),
  terminalLayout: {
    widthMode: 'full',
    tabLayout: 'horizontal',
    tabSidebarWidth: 200,
    tabSidebarPosition: 'left',
  },

  terminalActions: {
    registerSession(sessionId) {
      set((state) => {
        state.terminalSessionIds.add(sessionId)
      })
    },

    unregisterSession(sessionId) {
      set((state) => {
        state.terminalSessionIds.delete(sessionId)
      })
    },

    hasSession(sessionId) {
      return get().terminalSessionIds.has(sessionId)
    },

    setWidthMode(mode) {
      set((state) => {
        state.terminalLayout.widthMode = mode
      })
    },

    setTabLayout(layout) {
      set((state) => {
        state.terminalLayout.tabLayout = layout
      })
    },

    setTabSidebarWidth(width) {
      set((state) => {
        state.terminalLayout.tabSidebarWidth = width
      })
    },

    setTabSidebarPosition(pos) {
      set((state) => {
        state.terminalLayout.tabSidebarPosition = pos
      })
    },
  },
})
