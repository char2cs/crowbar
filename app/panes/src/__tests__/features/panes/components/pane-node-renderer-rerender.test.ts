import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LayoutNode, PaneGroup } from '@/features/panes/types/pane'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'

const paneRenderCounts = new Map<string, number>()
vi.mock('@/features/panes/components/pane-container', () => ({
  PaneContainer: ({ pane }: { pane: { id: string } }) => {
    paneRenderCounts.set(pane.id, (paneRenderCounts.get(pane.id) ?? 0) + 1)
    return createElement('div', { 'data-testid': `pane-${pane.id}` })
  },
}))

import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
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
    viewId: null,
  }
}

function setupStore() {
  const store = createWorkspaceStore('w1')
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    delete s.panes[ROOT_PANE_ID]
    s.stage = SPLIT
    s.activePaneId = 'pane-a'
    s.mostRecentActivePaneIds = ['pane-a']
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
    const store = setupStore()

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
    const store = setupStore()

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

    expect(paneRenderCounts.get('pane-b')).toBe(beforeB)
  })
})
