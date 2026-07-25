import { useEffect } from 'react'
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

/**
 * A pane never lands tab-less. Runs once hydration has settled — every pane
 * that restored with zero tabs (a fresh worktree, Project Home with no files
 * of its own, or a pane whose only tab was a New Tab that stripNewTabs
 * correctly refused to persist) opens on a New Tab exactly like any other.
 *
 * Gated PER PANE, not on the workspace's total buffer count: a workspace can
 * restore buffers overall while one of its panes (e.g. the survivor of a
 * split whose OTHER pane held all the real files) restores empty. Gating on
 * `buffers.length` alone skips that pane — it never gets seeded and comes back
 * with a blank tab strip.
 *
 * The BOTTOM pane is excluded: nothing renders it. `bottomLayout` is never
 * handed to PaneNodeRenderer (SplitViewRoot draws `rootLayout` only), and the
 * one drop path that could route a buffer there keys off a
 * `data-bottom-pane-drop-target` attribute no component emits. A New Tab seeded
 * into it is invisible AND unreclaimable — `newTab` is auto-eviction protected —
 * so it does nothing but spend one of MAX_OPEN_TABS for the life of the
 * workspace, which then evicts one of the user's real files a tab early. The
 * "never tab-less" invariant exists so the user always has something to look at;
 * it has no meaning for a pane the user can never look at.
 *
 * Gated on `hydrated` rather than firing on mount: opening a New Tab before the
 * restore lands would race it and leave a blank tab beside the restored files.
 */
export function useOpenOnNewTab(store: WorkspaceStore, hydrated: boolean): void {
  useEffect(() => {
    if (!hydrated) return
    const { panes, bufferActions } = store.getState()
    for (const pane of Object.values(panes)) {
      if (pane.id === BOTTOM_PANE_ID) continue
      if (pane.bufferIds.length === 0) bufferActions.openNewTab(pane.id)
    }
  }, [store, hydrated])
}
