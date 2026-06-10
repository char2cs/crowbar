// Stub
export interface UIStateStore {
  leftSidebarOpen: boolean
  rightSidebarOpen: boolean
  bottomPaneOpen: boolean
  activeLeftTab: string | null
  activeRightTab: string | null
  activeBottomTab: string | null
}

// Re-export types from main store for backward compat
export type { SettingsTab, BottomPaneTab } from '@/features/window/stores/ui-state-store'
