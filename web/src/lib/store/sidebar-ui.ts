import type { StateCreator } from 'zustand'
import { saveSidebarUI } from '@/lib/persistence/sidebar-ui'

// The sidebar's own UI state, apart from the repo tree it draws.

export type SidebarTab = 'chats' | 'files' | 'git'

const SIDEBAR_TABS: readonly SidebarTab[] = ['chats', 'files', 'git']

/** A persisted mode, or Chats for anything this build has no panel for. */
export function parseSidebarTab(value: unknown): SidebarTab {
  return SIDEBAR_TABS.find((tab) => tab === value) ?? 'chats'
}

export interface SidebarUIState {
  /**
   * Chats-panel rows the user has folded — folder ids and chat ids together,
   * since both kinds hold children; unknown means OPEN. Held here, not in the
   * panel, because the panel remounts on every workspace switch.
   */
  collapsedChatRows: Set<string>
  /** The mode the user chose; what shows also depends on availability (sidebar-panel-tabs.ts). */
  activeTab: SidebarTab
  /** Fold a Chats-panel row away, or open it again. */
  toggleChatRow: (rowId: string) => void
  setActiveTab: (tab: SidebarTab) => void
}

export function initialSidebarUIState(): Pick<SidebarUIState, 'collapsedChatRows' | 'activeTab'> {
  return {
    collapsedChatRows: new Set<string>(),
    activeTab: 'chats',
  }
}

// Folds and mode share one record, so every write carries both.
function persist(state: Pick<SidebarUIState, 'collapsedChatRows' | 'activeTab'>): void {
  void saveSidebarUI({
    collapsedChatRows: [...state.collapsedChatRows],
    activeTab: state.activeTab,
  })
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
      persist({ collapsedChatRows: next, activeTab: s.activeTab })
      return { collapsedChatRows: next }
    }),
  setActiveTab: (tab) =>
    set((s) => {
      if (s.activeTab === tab) return s
      persist({ collapsedChatRows: s.collapsedChatRows, activeTab: tab })
      return { activeTab: tab }
    }),
})
