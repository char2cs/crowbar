import { create } from 'zustand'

/**
 * Which row is mid inline-rename (double-click-to-rename), across the whole
 * sidebar. A store rather than lifted React state, for the same reason
 * `sidebar-selection.ts` is one: the trigger (the delegated `dblclick`
 * listener in `sidebar-tree-chrome.tsx`) and the row that has to draw the
 * input for it (`SidebarRow`, several components deeper, under a sibling
 * subtree) share no closer common ancestor worth threading five components
 * of props through.
 *
 * Entirely separate from `sidebar-tree-chrome.tsx`'s own `renamingRowId`
 * local state, which still drives the right-click menu's modal `RenameDialog`
 * — that path is untouched by this one.
 */

interface SidebarInlineRenameState {
  renamingRowId: string | null
  startRenaming: (rowId: string) => void
  stopRenaming: () => void
}

export function getInitialInlineRenameState() {
  return { renamingRowId: null }
}

export const useSidebarInlineRenameStore = create<SidebarInlineRenameState>()((set) => ({
  ...getInitialInlineRenameState(),
  startRenaming: (rowId) => set({ renamingRowId: rowId }),
  stopRenaming: () => set({ renamingRowId: null }),
}))
