import { act, fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

// I2: isUncloseable currently only hides the tab-bar-item's × button. Every
// OTHER close affordance — middle-click, and the tab context menu's Close /
// Close Others / Close All / Close to Right — must honour it too, or a pane's
// sole editor tab (the one state the design calls unreachable: editorTabIds:
// []) can still be closed out from under it.

vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: () => createElement('span', { 'data-testid': 'file-icon' }),
}))

// TabBar reaches useSidebar() only for the fallback sidebar-reopen toggle,
// irrelevant here — stub it so the suite needn't stand up a SidebarProvider
// (same stub tab-bar-rerender.test.tsx uses).
vi.mock('@/components/ui/sidebar', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/ui/sidebar')>()
  return {
    ...actual,
    useSidebar: () => ({ open: true, toggleSidebar: () => {} }),
  }
})

// Stub TabContextMenu down to one plain button per close affordance it wires,
// each invoking the prop it was handed with the open buffer's id. This
// exercises TabBar's REAL handleCloseOtherTabs / handleCloseAllTabs /
// handleCloseTabsToRight / handleContextMenuCloseTab — the exact functions I2
// found unguarded — without depending on the real base-ui menu's
// portal/positioning machinery (irrelevant to this suite, and already
// exercised by other tests).
vi.mock('@/features/tabs/components/tab-context-menu', () => ({
  default: ({
    isOpen,
    buffer,
    onCloseTab,
    onCloseOthers,
    onCloseAll,
    onCloseToRight,
  }: {
    isOpen: boolean
    buffer: { id: string } | null
    onCloseTab: (id: string) => void
    onCloseOthers: (id: string) => void
    onCloseAll: () => void
    onCloseToRight: (id: string) => void
  }) =>
    isOpen && buffer
      ? createElement(
          'div',
          { 'data-testid': 'ctx-menu' },
          createElement('button', { onClick: () => onCloseTab(buffer.id) }, 'close'),
          createElement('button', { onClick: () => onCloseOthers(buffer.id) }, 'close-others'),
          createElement('button', { onClick: () => onCloseAll() }, 'close-all'),
          createElement('button', { onClick: () => onCloseToRight(buffer.id) }, 'close-to-right'),
        )
      : null,
}))

import TabBar from '@/features/tabs/components/tab-bar'

function makeEditorBuffer(i: number): EditorContent {
  return {
    id: `buf-${i}`,
    type: 'editor',
    path: `/project/file-${i}.ts`,
    name: `file-${i}.ts`,
    content: '',
    savedContent: '',
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    isActive: false,
    tokens: [],
  }
}

/** A real editor tab, marked `isUncloseable` the way `syncSoleEditorTabCloseability`
 *  would for the sole tab a pane holds. There is no "Editor"/New Tab placeholder
 *  type any more (spec §7.1) — the invariant under test applies to any editor
 *  tab, so a plain file buffer stands in for it. */
function makeSoleTab(id: string, isUncloseable: boolean): EditorContent {
  return {
    id,
    type: 'editor',
    path: `/project/${id}.ts`,
    name: `${id}.ts`,
    content: '',
    savedContent: '',
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    isActive: false,
    tokens: [],
    isUncloseable,
  }
}

function renderTabBar(store: ReturnType<typeof createWorkspaceStore>, paneId: string) {
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(TabBar, { paneId }),
    ),
  )
}

/** A single sole-editor-tab pane, mirroring pane-slice's
 *  syncSoleEditorTabCloseability (isUncloseable is true exactly when it is
 *  the pane's only editor tab). */
function setupSoleTabStore() {
  const store = createWorkspaceStore('w1')
  const nt = makeSoleTab('nt-1', true)
  store.setState((s) => ({
    ...s,
    buffers: [nt],
    panes: {
      ...s.panes,
      [ROOT_PANE_ID]: { ...s.panes[ROOT_PANE_ID], editorTabIds: ['nt-1'], activeEditorTabId: 'nt-1' },
    },
  }))
  return store
}

/** A second pane (BOTTOM_PANE_ID) with several editor buffers, alongside
 *  ROOT_PANE_ID's sole (uncloseable) tab — reproduces "Close Others/All/to
 *  Right operate over EVERY workspace buffer, not just this tab bar's own
 *  pane" sweeping up a tab it has no business touching. */
function setupMultiPaneStore() {
  const store = createWorkspaceStore('w1')
  const editors = [0, 1, 2].map(makeEditorBuffer)
  const nt = makeSoleTab('nt-1', true)
  store.setState((s) => ({
    ...s,
    buffers: [...editors, nt],
    panes: {
      ...s.panes,
      [ROOT_PANE_ID]: { ...s.panes[ROOT_PANE_ID], editorTabIds: ['nt-1'], activeEditorTabId: 'nt-1' },
      [BOTTOM_PANE_ID]: {
        ...s.panes[BOTTOM_PANE_ID],
        editorTabIds: editors.map((b) => b.id),
        activeEditorTabId: editors[0].id,
      },
    },
  }))
  return store
}

function expectSoleTabSurvived(store: ReturnType<typeof createWorkspaceStore>) {
  expect(store.getState().buffers.some((b) => b.id === 'nt-1')).toBe(true)
  expect(store.getState().panes[ROOT_PANE_ID]?.editorTabIds).toContain('nt-1')
}

describe('TabBar honours isUncloseable (I2)', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('middle-click (auxclick) on the sole editor tab does not close it', () => {
    const store = setupSoleTabStore()
    act(() => {
      renderTabBar(store, ROOT_PANE_ID)
    })

    const tab = screen.getByRole('tab')
    act(() => {
      fireEvent(tab, new MouseEvent('auxclick', { bubbles: true, cancelable: true, button: 1 }))
    })

    expectSoleTabSurvived(store)
  })

  it('the context menu\'s "Close" on the sole editor tab does not close it', () => {
    const store = setupSoleTabStore()
    act(() => {
      renderTabBar(store, ROOT_PANE_ID)
    })

    const tab = screen.getByRole('tab')
    act(() => {
      fireEvent.contextMenu(tab)
    })
    act(() => {
      fireEvent.click(screen.getByText('close'))
    })

    expectSoleTabSurvived(store)
  })

  it('"Close Others" does not sweep up an uncloseable editor tab elsewhere in the workspace', () => {
    const store = setupMultiPaneStore()
    act(() => {
      renderTabBar(store, BOTTOM_PANE_ID)
    })

    const tabs = screen.getAllByRole('tab')
    act(() => {
      fireEvent.contextMenu(tabs[0])
    })
    act(() => {
      fireEvent.click(screen.getByText('close-others'))
    })

    expectSoleTabSurvived(store)
  })

  it('"Close All" does not sweep up an uncloseable editor tab elsewhere in the workspace', () => {
    const store = setupMultiPaneStore()
    act(() => {
      renderTabBar(store, BOTTOM_PANE_ID)
    })

    const tabs = screen.getAllByRole('tab')
    act(() => {
      fireEvent.contextMenu(tabs[0])
    })
    act(() => {
      fireEvent.click(screen.getByText('close-all'))
    })

    expectSoleTabSurvived(store)
  })

  it('"Close to Right" does not sweep up an uncloseable editor tab elsewhere in the workspace', () => {
    const store = setupMultiPaneStore()
    act(() => {
      renderTabBar(store, BOTTOM_PANE_ID)
    })

    const tabs = screen.getAllByRole('tab')
    // buf-0 (index 0) — everything "to the right" in the global buffers array
    // (buf-1, buf-2, nt-1) is a candidate, which is exactly what makes this
    // case reach the sole editor tab in the first place.
    act(() => {
      fireEvent.contextMenu(tabs[0])
    })
    act(() => {
      fireEvent.click(screen.getByText('close-to-right'))
    })

    expectSoleTabSurvived(store)
  })
})
