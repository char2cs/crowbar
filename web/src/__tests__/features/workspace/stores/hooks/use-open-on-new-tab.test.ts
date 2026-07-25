import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useOpenOnNewTab } from '@/features/workspace/stores/hooks/use-open-on-new-tab'
import { ROOT_PANE_ID, BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { EDITOR_CONSTANTS } from '@/features/editor/config/constants'

const openNewTab = vi.fn()

type FakePane = { id: string; bufferIds: string[] }

/** Mirrors production's real shape: `panes` always holds BOTH ROOT_PANE_ID and
 *  BOTTOM_PANE_ID (see pane-slice's initial state), each independently either
 *  empty or not — the gate this hook applies must be per-pane, not derived
 *  from the workspace-wide `buffers` count. */
function makeStore(panes: Record<string, FakePane>, totalBufferCount: number) {
  return {
    getState: () => ({
      buffers: Array.from({ length: totalBufferCount }, (_, i) => ({ id: `b${i}` })),
      panes,
      bufferActions: { openNewTab },
    }),
  }
}

function soloWorkspace(rootBufferIds: string[]) {
  return {
    [ROOT_PANE_ID]: { id: ROOT_PANE_ID, bufferIds: rootBufferIds },
    [BOTTOM_PANE_ID]: { id: BOTTOM_PANE_ID, bufferIds: [] },
  }
}

describe('useOpenOnNewTab', () => {
  beforeEach(() => openNewTab.mockClear())

  it('opens a New Tab in every RENDERED pane that restored with zero tabs', () => {
    renderHook(() => useOpenOnNewTab(makeStore(soloWorkspace([]), 0) as never, true))
    expect(openNewTab).toHaveBeenCalledWith(ROOT_PANE_ID)
    expect(openNewTab).toHaveBeenCalledTimes(1)
  })

  // The bottom pane is a layout that nothing renders: `bottomLayout` is never
  // handed to PaneNodeRenderer (SplitViewRoot draws `rootLayout` only), and the
  // one drop path that could put a buffer there keys off a
  // `data-bottom-pane-drop-target` attribute no component emits. A New Tab
  // seeded into it can therefore never be seen, closed, or consumed — and
  // because `newTab` is AUTO_EVICTION_PROTECTED it can never be reclaimed
  // either. It just permanently occupies a slot of the tab budget.
  it('does not seed the bottom pane, which nothing renders', () => {
    renderHook(() => useOpenOnNewTab(makeStore(soloWorkspace([]), 0) as never, true))
    expect(openNewTab).not.toHaveBeenCalledWith(BOTTOM_PANE_ID)
  })

  it('does nothing for a pane that already has tabs', () => {
    renderHook(() => useOpenOnNewTab(makeStore(soloWorkspace(['buf-1']), 3) as never, true))
    expect(openNewTab).not.toHaveBeenCalled()
  })

  it('waits for hydration — never races the restore', () => {
    renderHook(() => useOpenOnNewTab(makeStore(soloWorkspace([]), 0) as never, false))
    expect(openNewTab).not.toHaveBeenCalled()
  })

  // I3 regression: root holds a file, split right, close the file in the LEFT
  // (root) pane — root survives with `bufferIds: []` and the split pane keeps
  // the real file. The WORKSPACE restored one buffer overall (non-zero), but
  // root itself restored nothing. Gating on the workspace-wide buffer count
  // alone (the old implementation) skips root entirely: it never gets seeded
  // and comes back with a blank, permanently tab-less strip.
  it('seeds a pane left empty by a split even when the workspace restored other buffers', () => {
    const panes: Record<string, FakePane> = {
      [ROOT_PANE_ID]: { id: ROOT_PANE_ID, bufferIds: [] },
      'split-pane-2': { id: 'split-pane-2', bufferIds: ['buf-real'] },
      [BOTTOM_PANE_ID]: { id: BOTTOM_PANE_ID, bufferIds: [] },
    }
    renderHook(() => useOpenOnNewTab(makeStore(panes, 1) as never, true))
    expect(openNewTab).toHaveBeenCalledWith(ROOT_PANE_ID)
    expect(openNewTab).not.toHaveBeenCalledWith('split-pane-2')
    expect(openNewTab).toHaveBeenCalledTimes(1)
  })

  // The user-visible cost of seeding an unrendered pane: a buffer nobody can
  // see, that nothing can evict, spending one of MAX_OPEN_TABS. Run against the
  // REAL slices so the eviction policy under test is production's.
  it('does not spend a tab-cap slot on a pane the user can never see', () => {
    const store = createWorkspaceStore('w1')
    renderHook(() => useOpenOnNewTab(store, true))

    const max = EDITOR_CONSTANTS.MAX_OPEN_TABS
    for (let i = 0; i < max; i++) {
      store.getState().bufferActions.openContent({
        type: 'editor',
        path: `/src/f${i}.ts`,
        name: `f${i}.ts`,
        content: '',
      })
    }

    const paths = store.getState().buffers.map((b) => b.path)
    // All MAX_OPEN_TABS files the user opened are still open — the budget was
    // theirs to spend, not the invisible pane's.
    for (let i = 0; i < max; i++) expect(paths).toContain(`/src/f${i}.ts`)
  })
})
