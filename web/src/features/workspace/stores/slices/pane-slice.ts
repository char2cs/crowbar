import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { WorkspaceStore } from '../workspace-store'
import { isEditorContent, type PaneContent } from '@/features/panes/types/pane-content'
import { fileUri } from '@/features/editor/lib/editor-uri'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import type {
  PaneGroup,
  LayoutNode,
  SplitDirection,
  SplitPlacement,
} from '@/features/panes/types/pane'
import {
  createLeaf,
  splitLayout,
  closeLayout,
  findLeaf,
  findSplit,
  getAllLeafIds,
  distributeSplit,
  resizeFlattenedLayout,
  normalizeLayout,
  getAdjacentLeafId,
} from '@/features/panes/utils/pane-layout'

export interface PaneActions {
  splitPane(
    paneId: string,
    direction: SplitDirection,
    bufferId?: string,
    placement?: SplitPlacement,
  ): string | null
  closePane(paneId: string): void
  setActivePane(paneId: string): void
  activatePaneBuffer(paneId: string, bufferId: string | null): void
  addBufferToPane(paneId: string, bufferId: string, setActive?: boolean): void
  removeBufferFromPane(paneId: string, bufferId: string, preserveEmptyPane?: boolean): void
  moveBufferToPane(bufferId: string, fromPaneId: string, toPaneId: string): void
  setPanePreviewBuffer(paneId: string, bufferId: string | null): void
  setPaneBufferPinned(paneId: string, bufferId: string, pinned: boolean): void
  setPaneLocked(paneId: string, locked: boolean): void
  reorderPaneBuffers(paneId: string, startIndex: number, endIndex: number): void
  resizePaneSplit(splitId: string, index: number, sizes: [number, number]): void
  distributePaneSplit(splitId: string): void
  togglePaneFullscreen(paneId: string): void
  exitPaneFullscreen(): void
  getAllPaneGroups(): PaneGroup[]
  getPaneById(paneId: string): PaneGroup | null
  getPaneByBufferId(bufferId: string): PaneGroup | null
  getActivePane(): PaneGroup | null
  clearPreviewBufferEverywhere(bufferId: string): void
  switchToNextBufferInPane(): void
  switchToPreviousBufferInPane(): void
  navigateToPane(direction: 'left' | 'right' | 'up' | 'down'): void
}

export interface PaneSlice {
  panes: Record<string, PaneGroup>
  rootLayout: LayoutNode
  bottomLayout: LayoutNode
  activePaneId: string
  mostRecentActivePaneIds: string[]
  fullscreenPaneId: string | null
  paneActions: PaneActions
}

function makeRootLeaf(): PaneGroup {
  return { id: ROOT_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
}

function makeBottomLeaf(): PaneGroup {
  return { id: BOTTOM_PANE_ID, type: 'group', bufferIds: [], activeBufferId: null }
}

function layoutContainsPane(layout: LayoutNode, paneId: string): boolean {
  return findLeaf(layout, paneId) !== null
}

function getLayoutKey(
  state: Pick<PaneSlice, 'rootLayout' | 'bottomLayout'>,
  paneId: string,
): 'rootLayout' | 'bottomLayout' {
  if (layoutContainsPane(state.rootLayout, paneId)) return 'rootLayout'
  if (layoutContainsPane(state.bottomLayout, paneId)) return 'bottomLayout'
  return paneId === BOTTOM_PANE_ID ? 'bottomLayout' : 'rootLayout'
}

/**
 * A New Tab that is a pane's ONLY tab has no close affordance: closing it would
 * immediately spawn another (see removeBufferFromPane), so the × would be a
 * button that visibly does nothing. Beside other tabs it closes normally.
 * Derived state, so it is re-synced wherever a pane's membership changes.
 */
function syncNewTabCloseability(
  state: { panes: Record<string, { bufferIds: string[] }>; buffers?: PaneContent[] },
  paneId: string,
): void {
  const pane = state.panes[paneId]
  if (!pane || !Array.isArray(state.buffers)) return
  const sole = pane.bufferIds.length === 1
  // Index once: these are immer drafts, so the map holds the same draft
  // objects and mutating through it still records the change.
  const byId = new Map(state.buffers.map((b) => [b.id, b]))
  for (const id of pane.bufferIds) {
    const buf = byId.get(id)
    if (buf?.type === 'newTab') buf.isUncloseable = sole
  }
}

/** Whether `bufferId` names a New Tab placeholder. Defensive against test
 *  doubles that exercise the slice without a `buffers` array (see
 *  syncNewTabCloseability's own guard for the same reason). */
function isNewTabBuffer(state: { buffers?: PaneContent[] }, bufferId: string): boolean {
  return (
    Array.isArray(state.buffers) &&
    state.buffers.some((b) => b.id === bufferId && b.type === 'newTab')
  )
}

/** The New Tab id `pane` is currently holding, if any. A pane holds at most one
 *  (mirrors buffer-slice's `findNewTabInPane`, kept local to avoid a slice ->
 *  slice import for one lookup). */
function findNewTabId(
  state: { buffers?: PaneContent[] },
  pane: { bufferIds: string[] } | null | undefined,
): string | undefined {
  if (!pane || !Array.isArray(state.buffers)) return undefined
  const byId = new Map(state.buffers.map((b) => [b.id, b]))
  for (const id of pane.bufferIds) {
    const buf = byId.get(id)
    if (buf?.type === 'newTab') return buf.id
  }
  return undefined
}

export const createPaneSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  PaneSlice
> = (set, get, api) => {
  // `api` is the same object onto which `createWorkspaceStore` later attaches the
  // non-reactive `editorManager` handle (via Object.assign). It exists by the time
  // any action runs, so the cast is safe and keeps the editor lib decoupled from
  // the store state (the slice never imports editor *components*).
  const editorManagerOf = () => (api as unknown as WorkspaceStore).editorManager

  /** Release the held Monaco model for `bufferId` in `paneId` (editor buffers
   *  only). A no-op when the buffer isn't an editor or the pane didn't hold it
   *  (the manager guards on `held`). Disposes the model when no pane still holds
   *  it, so closing a tab frees its model and a reopen reads fresh content. */
  const releaseBufferModel = (paneId: string, bufferId: string) => {
    const buf = get().buffers?.find((b) => b.id === bufferId)
    if (!buf || !isEditorContent(buf)) return
    editorManagerOf()?.closeBuffer(paneId, fileUri(buf.path))
  }

  return {
    panes: { [ROOT_PANE_ID]: makeRootLeaf(), [BOTTOM_PANE_ID]: makeBottomLeaf() },
    rootLayout: createLeaf(ROOT_PANE_ID),
    bottomLayout: createLeaf(BOTTOM_PANE_ID),
    activePaneId: ROOT_PANE_ID,
    mostRecentActivePaneIds: [ROOT_PANE_ID],
    fullscreenPaneId: null,

    paneActions: {
      splitPane(paneId, direction, bufferId?, placement = 'after') {
        let newPaneId: string | null = null
        // A New Tab is a placeholder — two panes can never share the SAME one:
        // its `isUncloseable` flag can't mean "sole in pane A" and "not sole in
        // pane B" at once for a single shared buffer id. Handing the new pane
        // the source's New Tab id (as splitActiveEditorGroup did before this
        // fix, since getShareableSplitBufferId only excluded terminals) left
        // the exact same id in TWO panes' bufferIds — and closing/consuming
        // one side's copy (e.g. "New Terminal" in the new pane) then stranded
        // the id in the other. Detected here, at the mutation site, so it holds
        // regardless of which caller reaches this action.
        let needsOwnNewTab = false
        set((state) => {
          const key = getLayoutKey(state, paneId)
          const result = splitLayout(state[key], paneId, direction, placement)
          if (!result) return
          state[key] = result.layout
          newPaneId = result.newPaneId
          needsOwnNewTab = Boolean(bufferId) && isNewTabBuffer(state, bufferId!)
          const sharedBufferId = needsOwnNewTab ? undefined : bufferId
          state.panes[newPaneId] = {
            id: newPaneId,
            type: 'group',
            bufferIds: sharedBufferId ? [sharedBufferId] : [],
            activeBufferId: sharedBufferId ?? null,
          }
          state.activePaneId = newPaneId
          state.mostRecentActivePaneIds = [newPaneId, ...state.mostRecentActivePaneIds]
          // The source pane's own membership is untouched by a split, so this is
          // a no-op today — kept so a future change to this action can't silently
          // leave the source's isUncloseable flag stale.
          syncNewTabCloseability(state, paneId)
        })
        if (newPaneId && needsOwnNewTab) {
          get().bufferActions.openNewTab(newPaneId)
        }
        return newPaneId
      },

      closePane(paneId) {
        set((state) => {
          const key = getLayoutKey(state, paneId)
          const closingPane = state.panes[paneId]
          const result = closeLayout(state[key], paneId)
          let syncTarget: string | null = null
          if (result !== null) {
            state[key] = normalizeLayout(result)
            const remainingIds = getAllLeafIds(state[key])
            const fallbackId =
              remainingIds[0] ?? (key === 'rootLayout' ? ROOT_PANE_ID : BOTTOM_PANE_ID)
            syncTarget = fallbackId
            if (closingPane) {
              const fp = state.panes[fallbackId]
              if (fp) {
                // A New Tab is a placeholder that has served its purpose the
                // moment it lands beside a pane that is (or ends up) non-empty:
                // merging it in would either duplicate one `fp` already holds,
                // or sit uselessly next to real content. Only keep it — so `fp`
                // isn't left tab-less — when `fp` was otherwise completely
                // empty before this merge.
                const fpWasEmpty = fp.bufferIds.length === 0
                const existingBufferIds = new Set(fp.bufferIds)
                // A New Tab skipped by the merge below is referenced by NO pane
                // once `paneId` is deleted — and `newTab` is auto-eviction
                // PROTECTED, so an orphan is unreclaimable: it holds a slot of
                // the MAX_OPEN_TABS budget for the life of the workspace, and
                // once the budget fills `openContent` evicts the user's real
                // file instead. Drop it from `buffers` too, exactly as the two
                // sibling paths that also discard a New Tab already do
                // (consumeNewTabInPane, moveBufferToPane's duplicate branch).
                const droppedNewTabIds: string[] = []
                for (const bufferId of closingPane.bufferIds) {
                  if (existingBufferIds.has(bufferId)) continue
                  if (!fpWasEmpty && isNewTabBuffer(state, bufferId)) {
                    droppedNewTabIds.push(bufferId)
                    continue
                  }
                  fp.bufferIds.push(bufferId)
                  existingBufferIds.add(bufferId)
                }
                if (droppedNewTabIds.length > 0 && Array.isArray(state.buffers)) {
                  const dropped = new Set(droppedNewTabIds)
                  state.buffers = state.buffers.filter((b) => !dropped.has(b.id))
                }
                // Only adopt the closing pane's active buffer if it actually
                // survived the merge above (it won't have if it was a dropped
                // New Tab) — otherwise `fp` keeps whichever tab it already had
                // active rather than pointing at an id it doesn't hold.
                if (
                  state.activePaneId === paneId &&
                  closingPane.activeBufferId &&
                  fp.bufferIds.includes(closingPane.activeBufferId)
                ) {
                  fp.activeBufferId = closingPane.activeBufferId
                }
              }
            }
            if (state.activePaneId === paneId) state.activePaneId = fallbackId
          } else {
            const fallbackId = key === 'rootLayout' ? ROOT_PANE_ID : BOTTOM_PANE_ID
            state.panes[fallbackId] = key === 'rootLayout' ? makeRootLeaf() : makeBottomLeaf()
            state[key] = createLeaf(fallbackId)
            if (state.activePaneId === paneId) state.activePaneId = ROOT_PANE_ID
          }
          delete state.panes[paneId]
          state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(
            (id) => id !== paneId,
          )
          if (state.fullscreenPaneId === paneId) state.fullscreenPaneId = null
          if (syncTarget) syncNewTabCloseability(state, syncTarget)
        })
      },

      setActivePane(paneId) {
        set((state) => {
          state.activePaneId = paneId
          state.mostRecentActivePaneIds = [
            paneId,
            ...state.mostRecentActivePaneIds.filter((id) => id !== paneId),
          ]
        })
      },

      activatePaneBuffer(paneId, bufferId) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          // Only a tab the pane actually HOLDS may be activated. A pane resolves
          // its content as `paneBuffers.find(b => b.id === activeBufferId)` where
          // paneBuffers comes from its own bufferIds (pane-container.tsx), so an
          // id outside that list draws the empty-pane fallback WITH a populated
          // tab strip above it: tabs visible, none selected, nothing rendered.
          // Callers reach that routinely by activating right after a move that
          // may have dropped the buffer — use-tab-drag's drop handler and
          // pane-container's split-drop both call this with the dragged id
          // immediately after `moveBufferToPane`, which DELETES that buffer when
          // it is a duplicate New Tab. The defence belongs here rather than at
          // every call site. (`null` is the explicit "nothing is active" request
          // and always applies; attaching first — addBufferToPane — is how a
          // caller legitimately activates something the pane does not yet hold.)
          if (bufferId !== null && !pane.bufferIds.includes(bufferId)) return
          pane.activeBufferId = bufferId
          state.activePaneId = paneId
          state.mostRecentActivePaneIds = [
            paneId,
            ...state.mostRecentActivePaneIds.filter((id) => id !== paneId),
          ]
        })
      },

      addBufferToPane(paneId, bufferId, setActive = true) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          if (!pane.bufferIds.includes(bufferId)) pane.bufferIds.push(bufferId)
          if (setActive) pane.activeBufferId = bufferId
          syncNewTabCloseability(state, paneId)
        })
      },

      removeBufferFromPane(paneId, bufferId, preserveEmptyPane = false) {
        // Release this pane's held Monaco model BEFORE mutating state (reads the
        // buffer path, which still exists). Guarded internally: a no-op if the pane
        // never held it.
        if (get().panes[paneId]?.bufferIds.includes(bufferId)) {
          releaseBufferModel(paneId, bufferId)
        }
        // Read the type BEFORE the mutation: after it, the buffer may be gone.
        // A New Tab must never spawn its own replacement, or the pane can never
        // be emptied and closeBuffer recurses.
        const removedWasNewTab =
          (get().buffers as PaneContent[] | undefined)?.find((b) => b.id === bufferId)?.type ===
          'newTab'
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          const closedIndex = pane.bufferIds.indexOf(bufferId)
          const wasActive = pane.activeBufferId === bufferId
          pane.bufferIds = pane.bufferIds.filter((id) => id !== bufferId)
          if (wasActive) {
            // Activate the ADJACENT tab so a close keeps you on a nearby tab
            // (VS Code-style): the buffer that shifted into the closed slot (the
            // right neighbor), else the new last tab (the left neighbor when the
            // closed tab was last), else null when the pane is now empty.
            //
            // Only a tab that still HAS a buffer may be activated. A bufferId whose
            // buffer is gone renders nothing — the pane falls back to its empty
            // "New Terminal" state, which reads as "the app lost my tabs". That is
            // not hypothetical: any caller that drops a buffer without going through
            // this action leaves its id stranded here, and activating one of those
            // ghosts blanked the pane in the running app.
            // (When the slice is exercised without a buffer list — pane-slice's own
            // unit tests — every id counts as alive: there is nothing to check against.)
            const known = state.buffers
            const isAlive = (id: string) => !Array.isArray(known) || known.some((b) => b.id === id)
            const alive = pane.bufferIds.filter(isAlive)
            const rightNeighbor = pane.bufferIds.slice(closedIndex).find(isAlive)
            pane.activeBufferId = rightNeighbor ?? alive[alive.length - 1] ?? null
          }
          if (pane.previewBufferId === bufferId) pane.previewBufferId = null
          if (
            !preserveEmptyPane &&
            pane.bufferIds.length === 0 &&
            paneId !== ROOT_PANE_ID &&
            paneId !== BOTTOM_PANE_ID
          ) {
            const key = getLayoutKey(state, paneId)
            const result = closeLayout(state[key], paneId)
            if (result !== null) {
              state[key] = normalizeLayout(result)
              const remaining = getAllLeafIds(state[key])
              if (state.activePaneId === paneId) state.activePaneId = remaining[0] ?? ROOT_PANE_ID
              delete state.panes[paneId]
              state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(
                (id) => id !== paneId,
              )
            }
          }
          syncNewTabCloseability(state, paneId)
        })
        // A pane is never tab-less: an empty pane that SURVIVED (root, bottom, or
        // an explicit preserveEmptyPane) gets a New Tab so there is always
        // something to look at and somewhere to start from.
        const survivor = get().panes[paneId]
        if (!removedWasNewTab && survivor && survivor.bufferIds.length === 0) {
          get().bufferActions.openNewTab(paneId)
        }
      },

      moveBufferToPane(bufferId, fromPaneId, toPaneId) {
        set((state) => {
          const fromPane = state.panes[fromPaneId]
          const toPane = state.panes[toPaneId]
          if (!fromPane || !toPane) return
          fromPane.bufferIds = fromPane.bufferIds.filter((id) => id !== bufferId)
          if (fromPane.activeBufferId === bufferId)
            fromPane.activeBufferId = fromPane.bufferIds[0] ?? null

          // A New Tab is a placeholder — at most one per pane. If the dragged
          // buffer IS one and the destination already has its own, the dragged
          // one is a duplicate rather than new content: drop it globally (it
          // carries no state worth keeping) and focus the pane's existing New
          // Tab instead of sitting a second blank tab beside it. Real content
          // is unaffected — it is fine (and intentional elsewhere in this
          // slice) for a pane to hold both real tabs and its own New Tab at
          // once, so landing real content beside an existing New Tab is a
          // plain merge, same as any other buffer.
          const destNewTabId = isNewTabBuffer(state, bufferId)
            ? findNewTabId(state, toPane)
            : undefined
          if (destNewTabId && destNewTabId !== bufferId) {
            state.buffers = state.buffers.filter((b) => b.id !== bufferId)
            toPane.activeBufferId = destNewTabId
          } else {
            if (!toPane.bufferIds.includes(bufferId)) toPane.bufferIds.push(bufferId)
            toPane.activeBufferId = bufferId
          }

          state.activePaneId = toPaneId
          state.mostRecentActivePaneIds = [
            toPaneId,
            ...state.mostRecentActivePaneIds.filter((id) => id !== toPaneId),
          ]
          if (
            fromPane.bufferIds.length === 0 &&
            fromPaneId !== ROOT_PANE_ID &&
            fromPaneId !== BOTTOM_PANE_ID
          ) {
            const key = getLayoutKey(state, fromPaneId)
            const result = closeLayout(state[key], fromPaneId)
            if (result !== null) {
              state[key] = normalizeLayout(result)
              delete state.panes[fromPaneId]
              state.mostRecentActivePaneIds = state.mostRecentActivePaneIds.filter(
                (id) => id !== fromPaneId,
              )
            }
          }
          syncNewTabCloseability(state, fromPaneId)
          syncNewTabCloseability(state, toPaneId)
        })
        // A pane is never tab-less: reseed the source pane if it survived (root,
        // bottom, or wasn't closed above) and the move left it with nothing —
        // mirrors removeBufferFromPane's own survivor check.
        const survivor = get().panes[fromPaneId]
        if (survivor && survivor.bufferIds.length === 0) {
          get().bufferActions.openNewTab(fromPaneId)
        }
      },

      setPanePreviewBuffer(paneId, bufferId) {
        set((state) => {
          const p = state.panes[paneId]
          if (p) p.previewBufferId = bufferId
        })
      },

      setPaneBufferPinned(paneId, bufferId, pinned) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          if (!pane.pinnedBufferIds) pane.pinnedBufferIds = []
          if (pinned) {
            if (!pane.pinnedBufferIds.includes(bufferId)) pane.pinnedBufferIds.push(bufferId)
          } else pane.pinnedBufferIds = pane.pinnedBufferIds.filter((id) => id !== bufferId)
        })
      },

      setPaneLocked(paneId, locked) {
        set((state) => {
          const pane = state.panes[paneId]
          if (pane) pane.locked = locked
        })
      },

      reorderPaneBuffers(paneId, startIndex, endIndex) {
        set((state) => {
          const pane = state.panes[paneId]
          if (!pane) return
          const ids = [...pane.bufferIds]
          const [moved] = ids.splice(startIndex, 1)
          ids.splice(endIndex, 0, moved)
          pane.bufferIds = ids
        })
      },

      resizePaneSplit(splitId, index, sizes) {
        set((state) => {
          if (findSplit(state.rootLayout, splitId)) {
            state.rootLayout = resizeFlattenedLayout(state.rootLayout, splitId, index, sizes)
          } else if (findSplit(state.bottomLayout, splitId)) {
            state.bottomLayout = resizeFlattenedLayout(state.bottomLayout, splitId, index, sizes)
          }
        })
      },

      distributePaneSplit(splitId) {
        set((state) => {
          if (findSplit(state.rootLayout, splitId)) {
            state.rootLayout = distributeSplit(state.rootLayout, splitId)
          } else {
            state.bottomLayout = distributeSplit(state.bottomLayout, splitId)
          }
        })
      },

      togglePaneFullscreen(paneId) {
        set((state) => {
          state.fullscreenPaneId = state.fullscreenPaneId === paneId ? null : paneId
        })
      },

      exitPaneFullscreen() {
        set((state) => {
          state.fullscreenPaneId = null
        })
      },

      getAllPaneGroups() {
        return Object.values(get().panes)
      },
      getPaneById(paneId) {
        return get().panes[paneId] ?? null
      },
      getPaneByBufferId(bufferId) {
        return Object.values(get().panes).find((p) => p.bufferIds.includes(bufferId)) ?? null
      },
      getActivePane() {
        return get().panes[get().activePaneId] ?? null
      },

      clearPreviewBufferEverywhere(bufferId) {
        set((state) => {
          for (const pane of Object.values(state.panes)) {
            if (pane.previewBufferId === bufferId) pane.previewBufferId = null
          }
        })
      },

      switchToNextBufferInPane() {
        const state = get()
        const pane = state.panes[state.activePaneId]
        if (!pane || pane.bufferIds.length <= 1) return
        const curr = pane.activeBufferId ? pane.bufferIds.indexOf(pane.activeBufferId) : -1
        get().paneActions.activatePaneBuffer(
          pane.id,
          pane.bufferIds[(curr + 1) % pane.bufferIds.length],
        )
      },

      switchToPreviousBufferInPane() {
        const state = get()
        const pane = state.panes[state.activePaneId]
        if (!pane || pane.bufferIds.length <= 1) return
        const curr = pane.activeBufferId ? pane.bufferIds.indexOf(pane.activeBufferId) : 0
        get().paneActions.activatePaneBuffer(
          pane.id,
          pane.bufferIds[(curr - 1 + pane.bufferIds.length) % pane.bufferIds.length],
        )
      },

      navigateToPane(direction) {
        const state = get()
        for (const layout of [state.rootLayout, state.bottomLayout]) {
          const adj = getAdjacentLeafId(layout, state.activePaneId, direction)
          if (adj && state.panes[adj]) {
            set((s) => {
              s.activePaneId = adj
              s.mostRecentActivePaneIds = [
                adj,
                ...s.mostRecentActivePaneIds.filter((id) => id !== adj),
              ]
            })
            return
          }
        }
      },
    },
  }
}
