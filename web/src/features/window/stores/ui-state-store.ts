import { create } from 'zustand'
import type { SidebarView } from "@/features/layout/utils/sidebar-pane-utils"

// Module-level registry — not Zustand state (functions aren't serializable)
const _terminalFocusRegistry = new Map<string, () => void>()
const _registrationOrder: string[] = [] // tracks insertion order for recency

export function _resetTerminalFocusRegistryForTests(): void {
  _terminalFocusRegistry.clear()
  _registrationOrder.length = 0
}

export type SettingsTab =
  | "editor"
  | "appearance"
  | "terminal"
  | "git"
  | "account"
  | "ai"
  | "extensions"
  | "features"
  | "advanced"
  | "collaboration"
  | "enterprise"
  | "file-explorer"
  | "language"
  | "developer"

export type BottomPaneTab = "terminal" | "buffers"
export type SidebarActivityItem = "file-explorer" | "git" | "search" | "extensions"

export interface UIState {
  sidebarWidth: number
  bottomPaneHeight: number
  setBottomPaneHeight: (h: number) => void
  isSidebarVisible: boolean
  setIsSidebarVisible: (v: boolean) => void
  isBottomPaneVisible: boolean
  setIsBottomPaneVisible: (v: boolean) => void
  activeSidebarView: SidebarView | null
  setActiveSidebarView: (view: SidebarView | null) => void
  isGitViewActive: boolean
  settingsInitialTab: SettingsTab
  setSettingsInitialTab: (tab: SettingsTab) => void
  isSettingsOpen: boolean
  setIsSettingsOpen: (open: boolean) => void
  isFindVisible: boolean
  setIsFindVisible: (v: boolean) => void
  activeBottomTab: BottomPaneTab
  setActiveBottomTab: (tab: BottomPaneTab) => void
  bottomPaneActiveTab: BottomPaneTab
  setBottomPaneActiveTab: (tab: BottomPaneTab) => void
  requestTerminalFocus: () => void
  registerTerminalFocus: (id: string, fn: () => void) => void
  clearTerminalFocus: (id: string) => void
  sidebarActivityItem: SidebarActivityItem | null
  setSidebarActivityItem: (item: SidebarActivityItem | null) => void
  openSettingsDialog: (tab?: SettingsTab) => void
  openCommandPaletteView: (view?: string) => void
  // Command palette
  isCommandPaletteVisible: boolean
  commandPaletteInitialView: string | null
  setIsCommandPaletteVisible: (v: boolean) => void
  isSettingsDialogVisible: boolean
  setIsSettingsDialogVisible: (v: boolean) => void
  setActiveView: (view: string | null) => void
  setActiveRightSidebarView: (view: string | null) => void
  activeRightSidebarView: string | null
  setIsQuickOpenVisible: (v: boolean) => void
  setIsRightSidebarVisible: (v: boolean) => void
  activeView: string | null
  isRightSidebarVisible: boolean
  isQuickOpenVisible: boolean
  isAgentLauncherVisible: boolean
  setIsAgentLauncherVisible: (v: boolean) => void
  isGlobalSearchVisible: boolean
  setIsGlobalSearchVisible: (v: boolean) => void
  isProjectPickerVisible: boolean
  setIsProjectPickerVisible: (v: boolean) => void
}

export const useUIState = create<UIState>((set) => ({
  sidebarWidth: 260,
  bottomPaneHeight: 240,
  setBottomPaneHeight: (h: number) => set({ bottomPaneHeight: h }),
  isSidebarVisible: true,
  setIsSidebarVisible: (v) => set({ isSidebarVisible: v }),
  isBottomPaneVisible: false,
  setIsBottomPaneVisible: (v) => set({ isBottomPaneVisible: v }),
  activeSidebarView: null,
  setActiveSidebarView: (view) => set({ activeSidebarView: view }),
  isGitViewActive: false,
  settingsInitialTab: "appearance" as SettingsTab,
  setSettingsInitialTab: (tab) => set({ settingsInitialTab: tab }),
  isSettingsOpen: false,
  setIsSettingsOpen: (open) => set({ isSettingsOpen: open }),
  isFindVisible: false,
  setIsFindVisible: (v) => set({ isFindVisible: v }),
  activeBottomTab: "terminal" as BottomPaneTab,
  setActiveBottomTab: (tab) => set({ activeBottomTab: tab, bottomPaneActiveTab: tab }),
  bottomPaneActiveTab: "terminal" as BottomPaneTab,
  setBottomPaneActiveTab: (tab) => set({ bottomPaneActiveTab: tab, activeBottomTab: tab }),
  registerTerminalFocus: (id: string, fn: () => void) => {
    _terminalFocusRegistry.set(id, fn)
    // remove existing entry (if re-registering) then push to end
    const idx = _registrationOrder.indexOf(id)
    if (idx !== -1) _registrationOrder.splice(idx, 1)
    _registrationOrder.push(id)
  },
  clearTerminalFocus: (id: string) => {
    _terminalFocusRegistry.delete(id)
    const idx = _registrationOrder.indexOf(id)
    if (idx !== -1) _registrationOrder.splice(idx, 1)
  },
  requestTerminalFocus: () => {
    const lastId = _registrationOrder.at(-1)
    if (lastId) _terminalFocusRegistry.get(lastId)?.()
  },
  sidebarActivityItem: null,
  setSidebarActivityItem: (item) => set({ sidebarActivityItem: item }),
  openSettingsDialog: (tab) => set({ isSettingsOpen: true, settingsInitialTab: tab ?? "appearance" }),
  openCommandPaletteView: (view) => set({ isCommandPaletteVisible: true, commandPaletteInitialView: view ?? null }),
  // Command palette
  isCommandPaletteVisible: false,
  commandPaletteInitialView: null,
  setIsCommandPaletteVisible: (v) => set({ isCommandPaletteVisible: v }),
  isSettingsDialogVisible: false,
  setIsSettingsDialogVisible: (v) => set({ isSettingsOpen: v, isSettingsDialogVisible: v }),
  setActiveView: (view) => set({ activeView: view }),
  setActiveRightSidebarView: (view) => set({ activeRightSidebarView: view }),
  activeRightSidebarView: null,
  setIsQuickOpenVisible: (v) => set({ isQuickOpenVisible: v }),
  setIsRightSidebarVisible: (v) => set({ isRightSidebarVisible: v }),
  activeView: null,
  isRightSidebarVisible: false,
  isQuickOpenVisible: false,
  isAgentLauncherVisible: false,
  setIsAgentLauncherVisible: (v) => set({ isAgentLauncherVisible: v }),
  isGlobalSearchVisible: false,
  setIsGlobalSearchVisible: (v) => set({ isGlobalSearchVisible: v }),
  isProjectPickerVisible: false,
  setIsProjectPickerVisible: (v) => set({ isProjectPickerVisible: v }),
}))
