import { act, fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { BOTTOM_PANE_ID, ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'

vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: () => createElement('span', { 'data-testid': 'file-icon' }),
}))

vi.mock('@/components/ui/sidebar', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/ui/sidebar')>()
  return {
    ...actual,
    useSidebar: () => ({ open: true, toggleSidebar: () => {} }),
  }
})

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
    authority: 'workspace',
    path: `/project/file-${i}.ts`,
    name: `file-${i}.ts`,
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    workspaceId: 'w1',
  }
}

function makeSoleTab(id: string, isUncloseable: boolean): EditorContent {
  return {
    id,
    type: 'editor',
    authority: 'workspace',
    path: `/project/${id}.ts`,
    name: `${id}.ts`,
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
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

function setupSoleTabStore() {
  const store = createWorkspaceStore('w1')
  const nt = makeSoleTab('nt-1', true)
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers = [nt]
    s.panes[ROOT_PANE_ID] = {
      ...s.panes[ROOT_PANE_ID],
      editorTabIds: ['nt-1'],
      activeEditorTabId: 'nt-1',
    }
    return s
  })
  return store
}

function setupMultiPaneStore() {
  const store = createWorkspaceStore('w1')
  const editors = [0, 1, 2].map(makeEditorBuffer)
  const nt = makeSoleTab('nt-1', true)
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers = [...editors, nt]
    s.panes[ROOT_PANE_ID] = {
      ...s.panes[ROOT_PANE_ID],
      editorTabIds: ['nt-1'],
      activeEditorTabId: 'nt-1',
    }
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

function setupTwoPaneStore(bufferOrder: 'root-first' | 'bottom-first' = 'root-first') {
  const store = createWorkspaceStore('w1')
  const rootBufs = [makeEditorBuffer(0), makeEditorBuffer(1)]
  const bottomBufs = [makeEditorBuffer(2), makeEditorBuffer(3)]
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers =
      bufferOrder === 'bottom-first' ? [...bottomBufs, ...rootBufs] : [...rootBufs, ...bottomBufs]
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

describe('TabBar Close Others/All/to Right stay scoped to their own pane', () => {
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
    expect(remaining).toContain(bottomBufs[0].id)
  })

  it('"Close to Right" orders by THIS PANE\'s own tabs, not the flat window-wide buffer array', () => {
    const { store, rootBufs, bottomBufs } = setupTwoPaneStore('bottom-first')
    act(() => {
      renderTabBar(store, BOTTOM_PANE_ID)
    })

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

describe('TabBar honours isUncloseable', () => {
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
    act(() => {
      fireEvent.contextMenu(tabs[0])
    })
    act(() => {
      fireEvent.click(screen.getByText('close-to-right'))
    })

    expectSoleTabSurvived(store)
  })
})
