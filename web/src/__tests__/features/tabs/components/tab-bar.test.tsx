import { act, fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

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

vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: () => createElement('span', { 'data-testid': 'file-icon' }),
}))

vi.mock('@/features/tabs/components/tab-context-menu', () => ({
  default: () => null,
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
    isActive: i === 0,
    tokens: [],
  }
}

function setupStore(buffers: EditorContent[]) {
  const store = createWorkspaceStore('w1')
  store.setState((s) => ({
    ...s,
    buffers,
    panes: {
      ...s.panes,
      [ROOT_PANE_ID]: {
        ...s.panes[ROOT_PANE_ID],
        bufferIds: buffers.map((b) => b.id),
        activeBufferId: buffers[0]?.id ?? null,
      },
    },
  }))
  return store
}

function renderTabBar(store: ReturnType<typeof createWorkspaceStore>) {
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(TabBar, { paneId: ROOT_PANE_ID }),
    ),
  )
}

describe('TabBar "+" placement', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders the + as the last child INSIDE the scrolling tab container, not as a sibling after it', () => {
    const buffers = [makeEditorBuffer(0), makeEditorBuffer(1), makeEditorBuffer(2)]
    const store = setupStore(buffers)

    let container!: HTMLElement
    act(() => {
      ;({ container } = renderTabBar(store))
    })

    const addButton = screen.getByRole('button', { name: 'New tab' })
    const scrollContainer = container.querySelector('.tab-scrollbar')
    expect(scrollContainer).not.toBeNull()

    // Directly inside the scroll container (same element the tabs live in) —
    // not merely a descendant somewhere, and not a sibling rendered after it.
    expect(addButton.parentElement).toBe(scrollContainer)
    // ...and specifically AFTER every tab: the last child of that container.
    expect(scrollContainer?.lastElementChild).toBe(addButton)

    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(3)
    for (const tab of tabs) {
      expect(scrollContainer?.contains(tab)).toBe(true)
    }
  })

  it('flows to stay the last child as tabs are added', () => {
    const buffers = [makeEditorBuffer(0)]
    const store = setupStore(buffers)

    let container!: HTMLElement
    act(() => {
      ;({ container } = renderTabBar(store))
    })

    act(() => {
      store.setState((s) => ({
        ...s,
        buffers: [...s.buffers, makeEditorBuffer(1)],
        panes: {
          ...s.panes,
          [ROOT_PANE_ID]: {
            ...s.panes[ROOT_PANE_ID],
            bufferIds: [...s.panes[ROOT_PANE_ID].bufferIds, 'buf-1'],
          },
        },
      }))
    })

    const addButton = screen.getByRole('button', { name: 'New tab' })
    const scrollContainer = container.querySelector('.tab-scrollbar')
    expect(scrollContainer?.lastElementChild).toBe(addButton)
  })
})

describe('TabBar "+" click behaviour', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('opens a New Tab (not a terminal) and focuses this pane', () => {
    const buffers = [makeEditorBuffer(0)]
    const store = setupStore(buffers)
    act(() => {
      renderTabBar(store)
    })

    const beforeIds = new Set(store.getState().buffers.map((b) => b.id))

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'New tab' }))
    })

    const afterBuffers = store.getState().buffers
    const added = afterBuffers.find((b) => !beforeIds.has(b.id))

    expect(added).toBeDefined()
    expect(added?.type).toBe('newTab')
    expect(afterBuffers.some((b) => b.type === 'terminal')).toBe(false)
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
    expect(store.getState().panes[ROOT_PANE_ID]?.bufferIds).toContain(added?.id)
  })

  it('does not render a dropdown menu on click', () => {
    const buffers = [makeEditorBuffer(0)]
    const store = setupStore(buffers)
    act(() => {
      renderTabBar(store)
    })

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'New tab' }))
    })

    expect(screen.queryByRole('menu')).toBeNull()
    expect(screen.queryByText('New Terminal')).toBeNull()
  })
})
