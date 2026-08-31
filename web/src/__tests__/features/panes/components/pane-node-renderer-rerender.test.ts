import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LayoutNode, PaneGroup } from '@/features/panes/types/pane'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

// ── Per-pane render probe ────────────────────────────────────────────
// PaneContainer is the leaf's only child; it is NOT independently memoized, so
// each render of it corresponds 1:1 to a render of the PaneNodeRenderer leaf
// that hosts it. Keyed by pane id, so we can assert that mutating pane A never
// re-renders pane B's leaf.
const paneRenderCounts = new Map<string, number>()
vi.mock('@/features/panes/components/pane-container', () => ({
  PaneContainer: ({ pane }: { pane: { id: string } }) => {
    paneRenderCounts.set(pane.id, (paneRenderCounts.get(pane.id) ?? 0) + 1)
    return createElement('div', { 'data-testid': `pane-${pane.id}` })
  },
}))

// Imported AFTER the mock (vi.mock is hoisted regardless; kept explicit).
import { PaneNodeRenderer } from '@/features/panes/components/pane-node-renderer'
import { SplitViewRoot } from '@/features/panes/components/split-view-root'

const SPLIT: LayoutNode = {
  type: 'split',
  id: 'split-root',
  direction: 'horizontal',
  sizes: [50, 50],
  first: { type: 'pane', id: 'pane-a' },
  second: { type: 'pane', id: 'pane-b' },
}

function paneGroup(id: string, editorTabIds: string[], activeEditorTabId: string): PaneGroup {
  return {
    id,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds,
    activeEditorTabId,
    editorOpen: true,
  }
}

// Task 26: panes/layout live on the window-level `windowPaneStore`, not the
// per-workspace store — seed pane-a/pane-b there. `WorkspaceStoreContext` is
// kept around purely because PaneContainer still reads `workspaceId` off it.
function setupStore(layoutInto: 'root' | 'none') {
  const store = createWorkspaceStore('w1')
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    if (layoutInto === 'root') s.rootLayout = SPLIT
    s.panes['pane-a'] = paneGroup('pane-a', ['a1', 'a2'], 'a1')
    s.panes['pane-b'] = paneGroup('pane-b', ['b1'], 'b1')
    return s
  })
  return store
}

describe('PaneNodeRenderer leaf render isolation', () => {
  beforeEach(() => paneRenderCounts.clear())
  afterEach(() => vi.clearAllMocks())

  it('does not re-render pane B when pane A activeEditorTabId changes', () => {
    const store = setupStore('none')

    act(() => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(PaneNodeRenderer, { node: SPLIT }),
        ),
      )
    })

    const beforeA = paneRenderCounts.get('pane-a') ?? 0
    const beforeB = paneRenderCounts.get('pane-b') ?? 0
    expect(beforeA).toBeGreaterThan(0)
    expect(beforeB).toBeGreaterThan(0)

    // Switch the ACTIVE tab of pane A only. immer's structural sharing keeps
    // panes['pane-b'] referentially identical, so B's leaf must stay put.
    act(() => {
      windowPaneStore.getState().paneActions.activateEditorTabInPane('pane-a', 'a2')
    })

    expect(paneRenderCounts.get('pane-a')).toBe(beforeA + 1)
    expect(paneRenderCounts.get('pane-b')).toBe(beforeB)
  })
})

describe('SplitViewRoot end-to-end leaf isolation', () => {
  beforeEach(() => paneRenderCounts.clear())
  afterEach(() => vi.clearAllMocks())

  it('a pane-local change does not re-render the sibling leaf through the whole tree', () => {
    const store = setupStore('root')

    act(() => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(SplitViewRoot),
        ),
      )
    })

    const beforeB = paneRenderCounts.get('pane-b') ?? 0
    expect(paneRenderCounts.get('pane-a') ?? 0).toBeGreaterThan(0)
    expect(beforeB).toBeGreaterThan(0)

    act(() => {
      windowPaneStore.getState().paneActions.activateEditorTabInPane('pane-a', 'a2')
    })

    // SplitViewRoot no longer subscribes to the whole `panes` record, so it does
    // not re-render the layout tree; only pane A's own leaf reacts.
    expect(paneRenderCounts.get('pane-b')).toBe(beforeB)
  })
})
