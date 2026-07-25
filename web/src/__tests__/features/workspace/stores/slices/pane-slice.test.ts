// web/src/__tests__/features/workspace/stores/slices/pane-slice.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createStore } from 'zustand'
import { immer } from 'zustand/middleware/immer'
import { createPaneSlice, type PaneSlice } from '@/features/workspace/stores/slices/pane-slice'
import {
  createBufferSlice,
  type BufferSlice,
} from '@/features/workspace/stores/slices/buffer-slice'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { fileUri } from '@/features/editor/lib/editor-uri'

function makeStore() {
  return createStore<PaneSlice & { bufferActions: { openNewTab: ReturnType<typeof vi.fn> } }>()(
    immer((set, get) => ({
      ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
      bufferActions: { openNewTab: vi.fn() },
    })),
  )
}

function makeStoreWithBuffers(buffers: Array<Record<string, unknown>>) {
  return createStore<PaneSlice & { buffers: Array<Record<string, unknown>> }>()(
    immer((set, get) => ({
      ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
      buffers,
      bufferActions: { openNewTab: vi.fn() },
    })),
  )
}

describe('pane-slice', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => {
    store = makeStore()
  })

  it('initialises with a single empty root group', () => {
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup).not.toBeNull()
    expect(rootGroup?.id).toBe(ROOT_PANE_ID)
    expect(rootGroup?.bufferIds).toEqual([])
    expect(rootGroup?.activeBufferId).toBeNull()
  })

  it('splitPane returns a new pane ID and updates rootLayout to a split', () => {
    const newPaneId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')
    expect(newPaneId).not.toBeNull()
    const rootLayout = store.getState().rootLayout
    expect(rootLayout.type).toBe('split')
    const panes = store.getState().panes
    expect(Object.keys(panes)).toContain(newPaneId!)
  })

  it('addBufferToPane adds bufferId to the correct group', () => {
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.bufferIds).toContain('buf-1')
    expect(rootGroup?.activeBufferId).toBe('buf-1')
  })

  it('removeBufferFromPane removes bufferId from the group', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-2', false)
    actions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1', true)
    const rootGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootGroup?.bufferIds).not.toContain('buf-1')
    expect(rootGroup?.bufferIds).toContain('buf-2')
  })

  it('closing the active tab activates the ADJACENT tab (right neighbor, else left when last)', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-2', false)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-3', false)
    const activeOf = () => store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.activeBufferId

    // Activate the MIDDLE tab, then close it -> the right neighbor activates
    // (not the first tab, which is what dropped users onto a far-away tab).
    store.getState().paneActions.activatePaneBuffer(ROOT_PANE_ID, 'buf-2')
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-2')
    expect(activeOf()).toBe('buf-3')

    // buf-3 is now the last + active; closing it falls back to the left neighbor.
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-3')
    expect(activeOf()).toBe('buf-1')

    // Closing the only remaining tab leaves the pane empty.
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1')
    expect(activeOf()).toBeNull()
  })

  // A pane can hold bufferIds whose buffer no longer exists — any caller that drops
  // a buffer without going through removeBufferFromPane strands its id here (and the
  // layout is persisted, so a stranded id outlives a reload). Activating one of those
  // ghosts renders NOTHING: the pane falls back to its empty state and the user reads
  // it as "the app lost my tabs". Observed live after deleting an agent chat.
  it('never activates a tab whose buffer no longer exists', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-real', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-ghost', false)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-active', false)

    // Only buf-real and buf-active have buffers behind them; buf-ghost is stranded.
    store.setState((st) => ({
      ...st,
      buffers: [
        { id: 'buf-real', type: 'terminal' },
        { id: 'buf-active', type: 'terminal' },
      ],
    }))

    store.getState().paneActions.activatePaneBuffer(ROOT_PANE_ID, 'buf-active')
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-active')

    // The right neighbour is gone, so it falls left — but must SKIP the ghost.
    expect(store.getState().paneActions.getPaneById(ROOT_PANE_ID)?.activeBufferId).toBe('buf-real')
  })

  it('getAllPaneGroups returns all leaf groups from paneRoot and bottomRoot', () => {
    const actions = store.getState().paneActions
    actions.splitPane(ROOT_PANE_ID, 'horizontal')
    const groups = actions.getAllPaneGroups()
    // 2 from the paneRoot split + 1 from bottomRoot
    expect(groups).toHaveLength(3)
    const ids = groups.map((g) => g.id)
    expect(ids).toContain(ROOT_PANE_ID)
    expect(ids).toContain(BOTTOM_PANE_ID)
  })

  it('togglePaneFullscreen sets fullscreenPaneId', () => {
    store.getState().paneActions.togglePaneFullscreen(ROOT_PANE_ID)
    expect(store.getState().fullscreenPaneId).toBe(ROOT_PANE_ID)
  })

  it('exitPaneFullscreen clears fullscreenPaneId', () => {
    store.getState().paneActions.togglePaneFullscreen(ROOT_PANE_ID)
    store.getState().paneActions.exitPaneFullscreen()
    expect(store.getState().fullscreenPaneId).toBeNull()
  })
})

describe('pane-slice bottomRoot routing', () => {
  let store: ReturnType<typeof makeStore>

  beforeEach(() => {
    store = makeStore()
  })

  it('addBufferToPane adds to bottomRoot when paneId is BOTTOM_PANE_ID', () => {
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-1', true)
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.bufferIds).toContain('buf-1')
  })

  it('getAllPaneGroups includes groups from both paneRoot and bottomRoot', () => {
    const groups = store.getState().paneActions.getAllPaneGroups()
    const ids = groups.map((g) => g.id)
    expect(ids).toContain(ROOT_PANE_ID)
    expect(ids).toContain(BOTTOM_PANE_ID)
  })

  it('getPaneById finds pane in bottomRoot', () => {
    const pane = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(pane).not.toBeNull()
    expect(pane?.id).toBe(BOTTOM_PANE_ID)
  })

  it('activatePaneBuffer works for bottomRoot pane', () => {
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-bottom', false)
    store.getState().paneActions.activatePaneBuffer(BOTTOM_PANE_ID, 'buf-bottom')
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.activeBufferId).toBe('buf-bottom')
  })

  it('removeBufferFromPane removes buffer from bottomRoot pane', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-2', false)
    actions.removeBufferFromPane(BOTTOM_PANE_ID, 'buf-1', true)
    const bottomGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomGroup?.bufferIds).not.toContain('buf-1')
    expect(bottomGroup?.bufferIds).toContain('buf-2')
  })

  it('getPaneByBufferId finds buffer in bottomRoot', () => {
    store.getState().paneActions.addBufferToPane(BOTTOM_PANE_ID, 'buf-bottom', true)
    const pane = store.getState().paneActions.getPaneByBufferId('buf-bottom')
    expect(pane).not.toBeNull()
    expect(pane?.id).toBe(BOTTOM_PANE_ID)
  })

  it('moveBufferToPane moves buffer across trees (bottomRoot -> paneRoot)', () => {
    const actions = store.getState().paneActions
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-x', true)
    actions.moveBufferToPane('buf-x', BOTTOM_PANE_ID, ROOT_PANE_ID)
    const rootPaneGroup = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(rootPaneGroup?.bufferIds).toContain('buf-x')
    const bottomPaneGroup = store.getState().paneActions.getPaneById(BOTTOM_PANE_ID)
    expect(bottomPaneGroup?.bufferIds).not.toContain('buf-x')
  })
})

// C1 regression: removing/closing an editor buffer must release its retained
// Monaco model via the editorManager (keyed by FILE URI), so closing a tab frees
// the model and a reopen reads fresh content. Wires a fake editorManager onto the
// store object (the same place `createWorkspaceStore` Object.assign's it).
describe('pane-slice → editorManager model release (C1)', () => {
  function makeStoreWithManager() {
    const closeBuffer = vi.fn()
    // The pane slice reads `get().buffers` (path lookup) and `api.editorManager`.
    type S = PaneSlice & {
      buffers: Array<{ id: string; type: string; path: string }>
      bufferActions: { openNewTab: ReturnType<typeof vi.fn> }
    }
    let api: { editorManager: { closeBuffer: typeof closeBuffer } }
    const store = createStore<S>()(
      immer((set, get, rawApi) => {
        api = rawApi as unknown as typeof api
        api.editorManager = { closeBuffer }
        return {
          ...createPaneSlice(
            ...([set, get, rawApi] as unknown as Parameters<typeof createPaneSlice>),
          ),
          buffers: [{ id: 'buf-ed', type: 'editor', path: '/src/a.ts' }],
          bufferActions: { openNewTab: vi.fn() },
        }
      }),
    )
    return { store, closeBuffer }
  }

  it('removeBufferFromPane releases the model for that pane (paneId + fileUri)', () => {
    const { store, closeBuffer } = makeStoreWithManager()
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-ed', true)
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-ed', true)
    expect(closeBuffer).toHaveBeenCalledWith(ROOT_PANE_ID, fileUri('/src/a.ts'))
  })

  it('does not release for a pane that never held the buffer', () => {
    const { store, closeBuffer } = makeStoreWithManager()
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-ed', true)
    expect(closeBuffer).not.toHaveBeenCalled()
  })
})

describe('pane-slice — a pane is never tab-less', () => {
  it('spawns a New Tab when the last tab in the root pane closes', () => {
    const openNewTab = vi.fn()
    const store = createStore<PaneSlice & { bufferActions: { openNewTab: typeof openNewTab } }>()(
      immer((set, get) => ({
        ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
        bufferActions: { openNewTab },
      })),
    )
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1')
    expect(openNewTab).toHaveBeenCalledWith(ROOT_PANE_ID)
  })

  it('does NOT spawn one when the removed buffer was itself a New Tab', () => {
    const openNewTab = vi.fn()
    const store = createStore<
      PaneSlice & { bufferActions: { openNewTab: typeof openNewTab }; buffers: unknown[] }
    >()(
      immer((set, get) => ({
        ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
        bufferActions: { openNewTab },
        buffers: [{ id: 'nt-1', type: 'newTab' }],
      })),
    )
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'nt-1', true)
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, 'nt-1')
    // Otherwise removing a New Tab mints a New Tab, forever.
    expect(openNewTab).not.toHaveBeenCalled()
  })

  it('marks a sole New Tab uncloseable, and a New Tab beside others closeable', () => {
    const store = makeStoreWithBuffers([
      { id: 'nt-1', type: 'newTab', isUncloseable: false },
      { id: 'buf-1', type: 'editor', isUncloseable: false },
    ])
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'nt-1', true)
    expect(store.getState().buffers.find((b) => b.id === 'nt-1')?.isUncloseable).toBe(true)

    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    expect(store.getState().buffers.find((b) => b.id === 'nt-1')?.isUncloseable).toBe(false)
  })

  it('removeBufferFromPane re-syncs isUncloseable: a New Tab becomes sole again once its sibling closes', () => {
    const store = makeStoreWithBuffers([
      { id: 'nt-1', type: 'newTab', isUncloseable: false },
      { id: 'buf-1', type: 'editor', isUncloseable: false },
    ])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'nt-1', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    // Sanity: two tabs in the pane, so nt-1 is not (yet) sole.
    expect(store.getState().buffers.find((b) => b.id === 'nt-1')?.isUncloseable).toBe(false)

    actions.removeBufferFromPane(ROOT_PANE_ID, 'buf-1')

    // nt-1 is once again the pane's only tab: it must flip back to uncloseable.
    expect(store.getState().buffers.find((b) => b.id === 'nt-1')?.isUncloseable).toBe(true)
  })

  it('moveBufferToPane re-syncs isUncloseable on both the source and destination pane', () => {
    const store = makeStoreWithBuffers([
      { id: 'nt-root', type: 'newTab', isUncloseable: false },
      { id: 'nt-bottom', type: 'newTab', isUncloseable: false },
      { id: 'buf-1', type: 'editor', isUncloseable: false },
    ])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'nt-root', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(BOTTOM_PANE_ID, 'nt-bottom', true)
    const uncloseable = (id: string) =>
      store.getState().buffers.find((b) => b.id === id)?.isUncloseable

    // Before the move: root holds 2 tabs (nt-root not sole); bottom holds 1 (nt-bottom sole).
    expect(uncloseable('nt-root')).toBe(false)
    expect(uncloseable('nt-bottom')).toBe(true)

    actions.moveBufferToPane('buf-1', ROOT_PANE_ID, BOTTOM_PANE_ID)

    // Root is left with only nt-root -> now sole -> uncloseable flips true.
    expect(uncloseable('nt-root')).toBe(true)
    // Bottom now holds nt-bottom + buf-1 -> nt-bottom is no longer sole -> flips false.
    expect(uncloseable('nt-bottom')).toBe(false)
  })
})

// I1 regression: dismissing a split must not resurrect a stale New Tab in the
// surviving pane. Design call (see the report for the full rationale): a New
// Tab merged into a pane that already has (or will have) content has outlived
// its purpose — it is dropped rather than merged, and closeability is re-synced
// either way so a New Tab that IS kept (because the survivor was genuinely
// empty) gets a correct isUncloseable flag rather than a stale one.
describe('pane-slice — closePane and the New Tab invariants (I1)', () => {
  it("drops the closing split's New Tab rather than resurrecting it in a pane that already has content", () => {
    const store = makeStoreWithBuffers([
      { id: 'buf-1', type: 'editor', isUncloseable: false },
      { id: 'nt-1', type: 'newTab', isUncloseable: true },
    ])
    const actions = store.getState().paneActions
    // Root holds a real file.
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    // The split holds ONLY a New Tab.
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addBufferToPane(splitId, 'nt-1', true)

    actions.closePane(splitId)

    const root = actions.getPaneById(ROOT_PANE_ID)
    // The New Tab must NOT have been merged into root — root already had
    // content, so the placeholder had nothing left to do.
    expect(root?.bufferIds).toEqual(['buf-1'])
    expect(root?.activeBufferId).toBe('buf-1')
  })

  it('does not leave two identical New Tabs when the survivor already had its own', () => {
    const store = makeStoreWithBuffers([
      { id: 'nt-root', type: 'newTab', isUncloseable: true },
      { id: 'nt-split', type: 'newTab', isUncloseable: true },
    ])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'nt-root', true)
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addBufferToPane(splitId, 'nt-split', true)

    actions.closePane(splitId)

    const root = actions.getPaneById(ROOT_PANE_ID)
    // Root keeps its OWN New Tab only — never two blanks.
    expect(root?.bufferIds).toEqual(['nt-root'])
  })

  it('re-syncs isUncloseable after the merge: a merged-in New Tab that is now sole becomes uncloseable', () => {
    const store = makeStoreWithBuffers([{ id: 'nt-1', type: 'newTab', isUncloseable: false }])
    const actions = store.getState().paneActions
    // The split holds ONLY a New Tab; root is completely empty (no tabs at all).
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addBufferToPane(splitId, 'nt-1', true)
    // Root has nothing of its own, so its New Tab from the split must be KEPT
    // (dropping it would leave root tab-less) — and its isUncloseable flag
    // must be synced to `true` now that it is root's sole tab.
    actions.closePane(splitId)

    const root = actions.getPaneById(ROOT_PANE_ID)
    expect(root?.bufferIds).toEqual(['nt-1'])
    expect(store.getState().buffers.find((b) => b.id === 'nt-1')?.isUncloseable).toBe(true)
  })

  // C-leak regression: a New Tab the merge loop SKIPS is dropped from the
  // surviving pane's `bufferIds` but was never dropped from `state.buffers` —
  // leaving a buffer no pane references and nothing can reclaim. `newTab` is in
  // AUTO_EVICTION_PROTECTED, so every one of these permanently consumes a slot
  // of the MAX_OPEN_TABS budget and the evictee search skips it; once the budget
  // fills, `openContent` evicts the user's REAL FILE instead. The two sibling
  // paths that also discard a New Tab (consumeNewTabInPane, moveBufferToPane's
  // duplicate branch) both filter `state.buffers`; closePane must too.
  it('deletes the dropped New Tab from state.buffers instead of orphaning it', () => {
    const store = makeStoreWithBuffers([
      { id: 'buf-1', type: 'editor', isUncloseable: false },
      { id: 'nt-1', type: 'newTab', isUncloseable: true },
    ])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addBufferToPane(splitId, 'nt-1', true)

    actions.closePane(splitId)

    expect(store.getState().buffers.find((b) => b.id === 'nt-1')).toBeUndefined()
  })

  it('deletes the duplicate New Tab from state.buffers when the survivor already had its own', () => {
    const store = makeStoreWithBuffers([
      { id: 'nt-root', type: 'newTab', isUncloseable: true },
      { id: 'nt-split', type: 'newTab', isUncloseable: true },
    ])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'nt-root', true)
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addBufferToPane(splitId, 'nt-split', true)

    actions.closePane(splitId)

    expect(store.getState().buffers.find((b) => b.id === 'nt-split')).toBeUndefined()
    expect(store.getState().buffers.find((b) => b.id === 'nt-root')).toBeDefined()
  })

  it('keeps a New Tab that the merge actually KEPT (survivor was empty)', () => {
    const store = makeStoreWithBuffers([{ id: 'nt-1', type: 'newTab', isUncloseable: false }])
    const actions = store.getState().paneActions
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addBufferToPane(splitId, 'nt-1', true)

    actions.closePane(splitId)

    expect(store.getState().buffers.find((b) => b.id === 'nt-1')).toBeDefined()
  })

  it('does not point the survivor at a dropped New Tab as its active buffer', () => {
    const store = makeStoreWithBuffers([
      { id: 'buf-1', type: 'editor', isUncloseable: false },
      { id: 'nt-1', type: 'newTab', isUncloseable: true },
    ])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    const splitId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    // The split's New Tab is ACTIVE in the split when it closes (splitPane
    // already made the new split the active pane).
    actions.addBufferToPane(splitId, 'nt-1', true)
    expect(store.getState().activePaneId).toBe(splitId)

    actions.closePane(splitId)

    const root = actions.getPaneById(ROOT_PANE_ID)
    // root must keep pointing at its own real tab, not the dropped New Tab id.
    expect(root?.activeBufferId).toBe('buf-1')
    expect(root?.bufferIds).not.toContain('nt-1')
  })
})

// I6 regression: dragging a New Tab between panes must preserve both
// invariants — a pane is never tab-less, and a pane holds at most one New Tab.
describe('pane-slice — moveBufferToPane and the New Tab invariants (I6)', () => {
  it('reseeds the source pane with a fresh New Tab after its own New Tab is dragged away', () => {
    const openNewTab = vi.fn()
    const store = createStore<
      PaneSlice & {
        buffers: Array<Record<string, unknown>>
        bufferActions: { openNewTab: typeof openNewTab }
      }
    >()(
      immer((set, get) => ({
        ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
        buffers: [{ id: 'nt-left', type: 'newTab' }],
        bufferActions: { openNewTab },
      })),
    )
    const actions = store.getState().paneActions
    const rightId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addBufferToPane(ROOT_PANE_ID, 'nt-left', true)

    actions.moveBufferToPane('nt-left', ROOT_PANE_ID, rightId)

    // Root must not be left tab-less by the drag.
    const root = actions.getPaneById(ROOT_PANE_ID)
    expect(root?.bufferIds).toEqual([])
    expect(openNewTab).toHaveBeenCalledWith(ROOT_PANE_ID)
  })

  it('does not leave two New Tabs in the destination when it already had its own', () => {
    const store = makeStoreWithBuffers([
      { id: 'nt-left', type: 'newTab' },
      { id: 'nt-right', type: 'newTab' },
    ])
    const actions = store.getState().paneActions
    const rightId = actions.splitPane(ROOT_PANE_ID, 'horizontal')!
    actions.addBufferToPane(ROOT_PANE_ID, 'nt-left', true)
    actions.addBufferToPane(rightId, 'nt-right', true)

    actions.moveBufferToPane('nt-left', ROOT_PANE_ID, rightId)

    const right = actions.getPaneById(rightId)
    // Only ONE New Tab in the destination — the dragged duplicate is dropped,
    // and the destination's own New Tab becomes (and stays) active.
    expect(right?.bufferIds).toEqual(['nt-right'])
    expect(right?.activeBufferId).toBe('nt-right')
    // The dragged duplicate is gone from the buffer list entirely, not just
    // unreferenced — a New Tab carries no state worth keeping around.
    expect(store.getState().buffers.find((b) => b.id === 'nt-left')).toBeUndefined()
  })
})

// C-leak regression, end to end: the orphaned New Tabs closePane used to leave
// behind are AUTO_EVICTION_PROTECTED, so they silently eat the MAX_OPEN_TABS
// budget until `openContent`'s evictee search — which skips every protected
// buffer — starts closing the user's REAL FILES instead. Runs the pane slice
// against the REAL buffer slice so the eviction policy under test is production's.
describe('pane-slice + buffer-slice — split/close cycles must not evict real files', () => {
  function makeFullStore() {
    return createStore<PaneSlice & BufferSlice & { workspaceId: string }>()(
      immer((set, get, api) => ({
        ...createPaneSlice(...([set, get, api] as unknown as Parameters<typeof createPaneSlice>)),
        ...createBufferSlice(
          ...([set, get, api] as unknown as Parameters<typeof createBufferSlice>),
        ),
        workspaceId: 'ws-test',
      })),
    )
  }

  /** One "split the pane, then dismiss the split" round trip — the split gets
   *  its own New Tab (splitPane's I8 rule) and closePane then discards it. */
  function splitAndClose(store: ReturnType<typeof makeFullStore>) {
    const splitId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal')!
    store.getState().bufferActions.openNewTab(splitId)
    store.getState().paneActions.closePane(splitId)
  }

  it('does not grow state.buffers on every split/close round trip', () => {
    const store = makeFullStore()
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/src/a.ts',
      name: 'a.ts',
      content: '',
    })
    const before = store.getState().buffers.length

    for (let i = 0; i < 5; i++) splitAndClose(store)

    expect(store.getState().buffers.length).toBe(before)
  })

  it('still holds the user’s open file after enough split/close cycles to fill the tab cap', () => {
    const store = makeFullStore()
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/src/a.ts',
      name: 'a.ts',
      content: '',
    })

    // More cycles than MAX_OPEN_TABS, so a leak is guaranteed to fill the budget.
    for (let i = 0; i < 12; i++) splitAndClose(store)

    // Opening a second file must not close the first: nothing the user did
    // should have consumed the tab budget.
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/src/b.ts',
      name: 'b.ts',
      content: '',
    })

    const paths = store.getState().buffers.map((b) => b.path)
    expect(paths).toContain('/src/a.ts')
    expect(paths).toContain('/src/b.ts')
  })
})

// I4 regression: `activatePaneBuffer` wrote whatever id it was handed straight
// onto the pane. Its callers can hand it a DEAD one — use-tab-drag's drop
// handler calls it with the dragged buffer id immediately after
// `moveBufferToPane` may have DELETED that buffer (the duplicate-New-Tab
// branch) — and a pane pointed at a buffer it doesn't hold renders its empty
// fallback while the tab strip still shows tabs, none of them selected. Same
// class of defence `removeBufferFromPane` already applies when picking a
// neighbour to activate.
describe('pane-slice — activatePaneBuffer only activates something renderable (I4)', () => {
  it('ignores a buffer that no longer exists rather than blanking the pane', () => {
    const store = makeStoreWithBuffers([{ id: 'buf-1', type: 'editor' }])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)

    actions.activatePaneBuffer(ROOT_PANE_ID, 'buf-gone')

    expect(actions.getPaneById(ROOT_PANE_ID)?.activeBufferId).toBe('buf-1')
  })

  it('ignores a live buffer that this pane does not hold', () => {
    const store = makeStoreWithBuffers([
      { id: 'buf-1', type: 'editor' },
      { id: 'buf-elsewhere', type: 'editor' },
    ])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(BOTTOM_PANE_ID, 'buf-elsewhere', true)

    actions.activatePaneBuffer(ROOT_PANE_ID, 'buf-elsewhere')

    expect(actions.getPaneById(ROOT_PANE_ID)?.activeBufferId).toBe('buf-1')
  })

  it('still clears the active buffer when asked for null', () => {
    const store = makeStoreWithBuffers([{ id: 'buf-1', type: 'editor' }])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)

    actions.activatePaneBuffer(ROOT_PANE_ID, null)

    expect(actions.getPaneById(ROOT_PANE_ID)?.activeBufferId).toBeNull()
  })

  it('activates normally when the pane really holds the buffer', () => {
    const store = makeStoreWithBuffers([
      { id: 'buf-1', type: 'editor' },
      { id: 'buf-2', type: 'editor' },
    ])
    const actions = store.getState().paneActions
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)
    actions.addBufferToPane(ROOT_PANE_ID, 'buf-2', false)

    actions.activatePaneBuffer(ROOT_PANE_ID, 'buf-2')

    expect(actions.getPaneById(ROOT_PANE_ID)?.activeBufferId).toBe('buf-2')
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
  })
})

// I8 regression: splitting off an active New Tab must never leave the SAME
// buffer id in two panes' bufferIds — a New Tab is a placeholder, and sharing
// one across panes can't express "closeable in A, uncloseable in B" for a
// single shared id.
describe('pane-slice — splitPane and the New Tab invariants (I8)', () => {
  it("gives the new pane its OWN New Tab instead of sharing the source pane's", () => {
    const openNewTab = vi.fn()
    const store = createStore<
      PaneSlice & {
        buffers: Array<Record<string, unknown>>
        bufferActions: { openNewTab: typeof openNewTab }
      }
    >()(
      immer((set, get) => ({
        ...createPaneSlice(...([set, get, {}] as unknown as Parameters<typeof createPaneSlice>)),
        buffers: [{ id: 'nt-1', type: 'newTab' }],
        bufferActions: { openNewTab },
      })),
    )
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'nt-1', true)

    const newPaneId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal', 'nt-1')

    expect(newPaneId).not.toBeNull()
    const newPane = store.getState().paneActions.getPaneById(newPaneId!)
    // The bug: bufferIds included 'nt-1', the SAME id as root's copy.
    expect(newPane?.bufferIds).not.toContain('nt-1')
    expect(openNewTab).toHaveBeenCalledWith(newPaneId)

    // Root keeps its own copy, untouched.
    const root = store.getState().paneActions.getPaneById(ROOT_PANE_ID)
    expect(root?.bufferIds).toEqual(['nt-1'])
  })

  it('shares a REAL buffer id across panes as before (only New Tabs are exempt)', () => {
    const store = makeStoreWithBuffers([{ id: 'buf-1', type: 'editor' }])
    store.getState().paneActions.addBufferToPane(ROOT_PANE_ID, 'buf-1', true)

    const newPaneId = store.getState().paneActions.splitPane(ROOT_PANE_ID, 'horizontal', 'buf-1')

    const newPane = store.getState().paneActions.getPaneById(newPaneId!)
    expect(newPane?.bufferIds).toEqual(['buf-1'])
  })
})
