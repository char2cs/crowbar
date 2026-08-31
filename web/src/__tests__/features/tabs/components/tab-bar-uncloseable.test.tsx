import { act, fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

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
    workspaceId: 'w1',
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
    workspaceId: 'w1',
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
 *  the pane's only editor tab). Task 26: panes/buffers are window-level now —
 *  seed `windowPaneStore`, not the per-workspace store returned here (kept
 *  for WorkspaceStoreContext). */
function setupSoleTabStore() {
  const store = createWorkspaceStore('w1')
  const nt = makeSoleTab('nt-1', true)
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers = [nt]
    s.panes[ROOT_PANE_ID] = { ...s.panes[ROOT_PANE_ID], editorTabIds: ['nt-1'], activeEditorTabId: 'nt-1' }
    return s
  })
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
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers = [...editors, nt]
    s.panes[ROOT_PANE_ID] = { ...s.panes[ROOT_PANE_ID], editorTabIds: ['nt-1'], activeEditorTabId: 'nt-1' }
    s.panes[BOTTOM_PANE_ID] = {
      ...s.panes[BOTTOM_PANE_ID],
      editorTabIds: editors.map((b) => b.id),
      activeEditorTabId: editors[0].id,
    }
    return s
  })
  return store
}

function expectSoleTabSurvived(_store: ReturnType<typeof createWorkspaceStore>) {
  expect(windowPaneStore.getState().buffers.some((b) => b.id === 'nt-1')).toBe(true)
  expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toContain('nt-1')
}

/** Two real (closeable — neither pinned nor the sole tab of its pane) editor
 *  tabs in EACH of ROOT_PANE_ID and BOTTOM_PANE_ID — the harness I4's fix
 *  needs: a tab that the OLD window-wide bug would have swept up (it is not
 *  pinned and not uncloseable) but that correctly-scoped Close All/Others/to
 *  Right must never touch because it belongs to a DIFFERENT pane. */
function setupTwoPaneStore(bufferOrder: 'root-first' | 'bottom-first' = 'root-first') {
  const store = createWorkspaceStore('w1')
  const rootBufs = [makeEditorBuffer(0), makeEditorBuffer(1)]
  const bottomBufs = [makeEditorBuffer(2), makeEditorBuffer(3)]
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    // Order matters for "Close to Right": it must key off THIS PANE's own
    // tab order (pane.editorTabIds), never the flat window-wide array's —
    // bottom-first here specifically defeats an unscoped `slice(idx + 1)`
    // over the flat array, which would otherwise coincidentally look correct
    // whenever the active pane's own tabs already come last in that array.
    s.buffers = bufferOrder === 'bottom-first' ? [...bottomBufs, ...rootBufs] : [...rootBufs, ...bottomBufs]
    s.panes[ROOT_PANE_ID] = {
      ...s.panes[ROOT_PANE_ID],
      editorTabIds: rootBufs.map((b) => b.id),
      activeEditorTabId: rootBufs[0].id,
    }
    s.panes[BOTTOM_PANE_ID] = {
      ...s.panes[BOTTOM_PANE_ID],
      editorTabIds: bottomBufs.map((b) => b.id),
      activeEditorTabId: bottomBufs[0].id,
    }
    return s
  })
  return { store, rootBufs, bottomBufs }
}

describe('TabBar Close Others/All/to Right stay scoped to their own pane (I4)', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('"Close All" in one pane never touches a closeable tab belonging to a DIFFERENT pane', () => {
    const { store, rootBufs, bottomBufs } = setupTwoPaneStore()
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

    const remaining = windowPaneStore.getState().buffers.map((b) => b.id)
    for (const b of rootBufs) expect(remaining).toContain(b.id)
    for (const b of bottomBufs) expect(remaining).not.toContain(b.id)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds).toEqual(
      rootBufs.map((b) => b.id),
    )
  })

  it('"Close Others" in one pane keeps every tab in a DIFFERENT pane', () => {
    const { store, rootBufs, bottomBufs } = setupTwoPaneStore()
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

    const remaining = windowPaneStore.getState().buffers.map((b) => b.id)
    for (const b of rootBufs) expect(remaining).toContain(b.id)
    // The kept tab (bottomBufs[0], right-clicked) survives; the other
    // BOTTOM_PANE_ID tab is closed — but ROOT_PANE_ID's are untouched either way.
    expect(remaining).toContain(bottomBufs[0].id)
  })

  it('"Close to Right" orders by THIS PANE\'s own tabs, not the flat window-wide buffer array', () => {
    // bottom-first: BOTTOM_PANE_ID's own tabs sit BEFORE ROOT_PANE_ID's in
    // the flat buffers array, so an unscoped `slice(idx + 1)` over that array
    // would incorrectly reach past bottomBufs[1] into rootBufs — exactly the
    // bug this test exists to catch.
    const { store, rootBufs, bottomBufs } = setupTwoPaneStore('bottom-first')
    act(() => {
      renderTabBar(store, BOTTOM_PANE_ID)
    })

    // Right-click BOTTOM_PANE_ID's FIRST tab — "to the right" must mean
    // bottomBufs[1] only, never anything from ROOT_PANE_ID.
    const tabs = screen.getAllByRole('tab')
    act(() => {
      fireEvent.contextMenu(tabs[0])
    })
    act(() => {
      fireEvent.click(screen.getByText('close-to-right'))
    })

    const remaining = windowPaneStore.getState().buffers.map((b) => b.id)
    for (const b of rootBufs) expect(remaining).toContain(b.id)
    expect(remaining).toContain(bottomBufs[0].id)
    expect(remaining).not.toContain(bottomBufs[1].id)
  })
})

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
