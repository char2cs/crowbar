import type { StateCreator } from 'zustand'
import { ALL_HOSTS } from '@/features/hosts/lib/host-filter'
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
  /**
   * The host whose projects the switcher lists, or ALL_HOSTS. A preference only: what shows is
   * `shownHostOf` (host-filter.ts), where the active project's host wins.
   */
  hostFilter: string
  /** Fold a Chats-panel row away, or open it again. */
  toggleChatRow: (rowId: string) => void
  setActiveTab: (tab: SidebarTab) => void
  setHostFilter: (hostId: string) => void
}

type Persisted = Pick<SidebarUIState, 'collapsedChatRows' | 'activeTab' | 'hostFilter'>

export function initialSidebarUIState(): Persisted {
  return {
    collapsedChatRows: new Set<string>(),
    activeTab: 'chats',
    hostFilter: ALL_HOSTS,
  }
}

// Folds, mode and host filter share one record, so every write carries all three.
function persist(state: Persisted): void {
  void saveSidebarUI({
    collapsedChatRows: [...state.collapsedChatRows],
    activeTab: state.activeTab,
    hostFilter: state.hostFilter,
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
      persist({ ...s, collapsedChatRows: next })
      return { collapsedChatRows: next }
    }),
  setActiveTab: (tab) =>
    set((s) => {
      if (s.activeTab === tab) return s
      persist({ ...s, activeTab: tab })
      return { activeTab: tab }
    }),
  setHostFilter: (hostId) =>
    set((s) => {
      if (s.hostFilter === hostId) return s
      persist({ ...s, hostFilter: hostId })
      return { hostFilter: hostId }
    }),
})
