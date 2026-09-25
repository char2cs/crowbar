import type { StateCreator } from 'zustand'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'

// The sidebar's own UI state, apart from the repo tree it draws.

export type SidebarTab = 'workspaces' | 'chats' | 'files' | 'git'

export interface SidebarUIState {
  /**
   * Chats-panel rows the user has folded — folder ids and chat ids together,
   * since both kinds hold children; unknown means OPEN. Held here, not in the
   * panel, because the panel remounts on every workspace switch.
   */
  collapsedChatRows: Set<string>
  /** Persisted active tab so re-mounts don't reset it. */
  activeTab: SidebarTab
  /** Fold a Chats-panel row away, or open it again. */
  toggleChatRow: (rowId: string) => void
  setActiveTab: (tab: SidebarTab) => void
}

export function initialSidebarUIState(): Pick<SidebarUIState, 'collapsedChatRows' | 'activeTab'> {
  return {
    collapsedChatRows: new Set<string>(),
    // The carousel's default-visible panel is Files; a cold-start default that
    // names another panel leaves neither tab underlined.
    activeTab: 'files',
  }
}

export const createSidebarUISlice: StateCreator<SidebarUIState, [], [], SidebarUIState> = (
  set,
) => ({
  ...initialSidebarUIState(),
  toggleChatRow: (rowId) =>
    set((s) => {
      const next = new Set(s.collapsedChatRows)
      if (next.has(rowId)) next.delete(rowId)
      else next.add(rowId)
      void saveSidebarUI({ collapsedChatRows: [...next] })
      return { collapsedChatRows: next }
    }),
  setActiveTab: (tab) => set({ activeTab: tab }),
})
