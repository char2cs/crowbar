// Stub: window/ui-state feature is out of scope for this session.
import { create } from 'zustand'
import type { SidebarView } from "@/features/layout/utils/sidebar-pane-utils"

export type SettingsTab =
  | "general"
  | "editor"
  | "appearance"
  | "keyboard"
  | "terminal"
  | "git"
  | "account"
  | "ai"
  | "extensions"
  | "features"
  | "advanced"
  | "collaboration"
  | "enterprise"
  | "databases"
  | "file-explorer"
  | "language"

export type BottomPaneTab = "terminal" | "problems" | "output" | "debug" | "console" | "debugger"
export type SidebarActivityItem = "file-explorer" | "git" | "search" | "extensions" | "outline" | "collaboration"

export interface UIState {
  sidebarWidth: number
  bottomPaneHeight: number
  isSidebarVisible: boolean
  setIsSidebarVisible: (v: boolean) => void
  isBottomPaneVisible: boolean
  setIsBottomPaneVisible: (v: boolean) => void
  activeSidebarView: SidebarView | null
  setActiveSidebarView: (view: SidebarView | null) => void
  isGitViewActive: boolean
  isGitHubPRsViewActive: boolean
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
}

export const useUIState = create<UIState>((set) => ({
  sidebarWidth: 260,
  bottomPaneHeight: 240,
  isSidebarVisible: true,
  setIsSidebarVisible: (v) => set({ isSidebarVisible: v }),
  isBottomPaneVisible: false,
  setIsBottomPaneVisible: (v) => set({ isBottomPaneVisible: v }),
  activeSidebarView: null,
  setActiveSidebarView: (view) => set({ activeSidebarView: view }),
  isGitViewActive: false,
  isGitHubPRsViewActive: false,
  settingsInitialTab: "general" as SettingsTab,
  setSettingsInitialTab: (tab) => set({ settingsInitialTab: tab }),
  isSettingsOpen: false,
  setIsSettingsOpen: (open) => set({ isSettingsOpen: open }),
  isFindVisible: false,
  setIsFindVisible: (v) => set({ isFindVisible: v }),
  activeBottomTab: "terminal" as BottomPaneTab,
  setActiveBottomTab: (tab) => set({ activeBottomTab: tab, bottomPaneActiveTab: tab }),
  bottomPaneActiveTab: "terminal" as BottomPaneTab,
  setBottomPaneActiveTab: (tab) => set({ bottomPaneActiveTab: tab, activeBottomTab: tab }),
  requestTerminalFocus: () => {},
  registerTerminalFocus: () => {},
  clearTerminalFocus: () => {},
  sidebarActivityItem: null,
  setSidebarActivityItem: (item) => set({ sidebarActivityItem: item }),
  openSettingsDialog: (tab) => set({ isSettingsOpen: true, settingsInitialTab: tab ?? "general" }),
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
}))
