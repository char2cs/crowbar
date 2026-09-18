import { create } from 'zustand'

/**
 * Which row is mid inline-rename — double-click, or the right-click menu's
 * Rename item, both start the same edit — across the whole sidebar. A store
 * rather than lifted React state, for the same reason `sidebar-selection.ts`
 * is one: the triggers (`sidebar-tree-chrome.tsx`'s delegated `dblclick`
 * listener, `row-context-menu.tsx`'s Rename item) and the row that has to
 * draw the input for it (`SidebarRow`, several components deeper, under a
 * sibling subtree) share no closer common ancestor worth threading five
 * components of props through.
 *
 * `renamingRole` is the resolution to the id-collision `inlineRenameDisabled`
 * (sidebar-row.tsx) documents: a live-paned chat, or a workspace branch that
 * happens to be both in Recents AND under an expanded repo, renders through
 * TWO `SidebarRow` instances sharing one id. `startRenaming` decides, once,
 * which of them actually draws the editor, by checking the DOM for a
 * tree-rendered instance at the moment the rename starts — the same
 * `[attr="id"]` lookup `drop-dom.ts`'s `elementFor` already uses. A tree
 * instance wins when one is mounted (develop's own rule: the tree is the one
 * place a name is edited). When the row's ONLY mounted instance is its
 * Recents mirror — its own tree row is folded away under a collapsed repo or
 * project, not merely off-screen — the Recents instance is the one that has
 * to draw it, or the edit has nowhere to render at all. Live-reproduced: a
 * Recents entry for a branch whose repo section is collapsed had no tree
 * copy in the DOM, so `inlineRenameDisabled` (a static per-caller flag, with
 * no notion of what else is actually mounted) always refused it — this is
 * the actual reason `RenameDialog` used to exist as a fallback for that one
 * case, instead of this being fixed.
 */

interface SidebarInlineRenameState {
  renamingRowId: string | null
  renamingRole: 'tree' | 'recents' | null
  startRenaming: (rowId: string) => void
  stopRenaming: () => void
}

export function getInitialInlineRenameState() {
  return { renamingRowId: null, renamingRole: null }
}

export const useSidebarInlineRenameStore = create<SidebarInlineRenameState>()((set) => ({
  ...getInitialInlineRenameState(),
  startRenaming: (rowId) => {
    const hasTreeInstance =
      typeof document !== 'undefined' &&
      document.querySelector(
        `[role="treeitem"][data-sidebar-row-id="${rowId}"]:not([data-sidebar-recents-row])`,
      ) != null
    set({ renamingRowId: rowId, renamingRole: hasTreeInstance ? 'tree' : 'recents' })
  },
  stopRenaming: () => set({ renamingRowId: null, renamingRole: null }),
}))
