import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'
import type { AgentChat } from '@/features/agent/api/agent-api'
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

function makeChat(overrides: Partial<AgentChat> = {}): AgentChat {
  return {
    id: 'chat-1',
    workspaceId: 'w1',
    title: 'My Chat',
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: '',
    createdAt: new Date().toISOString(),
    parentId: '',
    ...overrides,
  } as AgentChat
}

/** Seeds a single pane (ROOT_PANE_ID) with the given editor tabs and, when
 *  `chatId` is set, a matching `agentChats.chats` fixture so ChatHead's title
 *  lookup resolves — mirrors how `recents-band.tsx` reads a chat's title. */
function setupPaneStore({
  buffers = [],
  chatId = null,
  activeEditorTabId = buffers[0]?.id ?? null,
}: {
  buffers?: EditorContent[]
  chatId?: string | null
  activeEditorTabId?: string | null
}) {
  const store = createWorkspaceStore('w1')
  store.setState((s) => ({
    ...s,
    buffers,
    agentChats: {
      ...s.agentChats,
      chats: chatId ? [makeChat({ id: chatId })] : s.agentChats.chats,
    },
    panes: {
      ...s.panes,
      [ROOT_PANE_ID]: {
        ...s.panes[ROOT_PANE_ID],
        chatId,
        editorTabIds: buffers.map((b) => b.id),
        activeEditorTabId,
      },
    },
  }))
  return store
}

function setupStore(buffers: EditorContent[]) {
  return setupPaneStore({ buffers })
}

function renderTabBar(
  store: ReturnType<typeof createWorkspaceStore>,
  props: Partial<React.ComponentProps<typeof TabBar>> = {},
) {
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(TabBar, { paneId: ROOT_PANE_ID, ...props }),
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
    const scrollContainer = screen.getByTestId('editor-tab-scroller')
    expect(scrollContainer).not.toBeNull()

    // Directly inside the scroll container (same element the tabs live in) —
    // not merely a descendant somewhere, and not a sibling rendered after it.
    expect(addButton.parentElement).toBe(scrollContainer)
    // ...and specifically AFTER every tab: the last child of that container.
    expect(scrollContainer.lastElementChild).toBe(addButton)

    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(3)
    for (const tab of tabs) {
      expect(scrollContainer.contains(tab)).toBe(true)
    }
    void container
  })

  it('flows to stay the last child as tabs are added', () => {
    const buffers = [makeEditorBuffer(0)]
    const store = setupStore(buffers)

    act(() => {
      renderTabBar(store)
    })

    act(() => {
      store.setState((s) => ({
        ...s,
        buffers: [...s.buffers, makeEditorBuffer(1)],
        panes: {
          ...s.panes,
          [ROOT_PANE_ID]: {
            ...s.panes[ROOT_PANE_ID],
            editorTabIds: [...s.panes[ROOT_PANE_ID].editorTabIds, 'buf-1'],
          },
        },
      }))
    })

    const addButton = screen.getByRole('button', { name: 'New tab' })
    const scrollContainer = screen.getByTestId('editor-tab-scroller')
    expect(scrollContainer.lastElementChild).toBe(addButton)
  })
})

describe('TabBar "+" click behaviour', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('calls onAddTab with this pane and focuses it (no built-in New Tab placeholder any more)', () => {
    const buffers = [makeEditorBuffer(0)]
    const store = setupStore(buffers)
    const onAddTab = vi.fn()
    act(() => {
      renderTabBar(store, { onAddTab })
    })

    act(() => {
      store.getState().paneActions.setActivePane('some-other-pane-first')
    })

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'New tab' }))
    })

    expect(onAddTab).toHaveBeenCalledWith(ROOT_PANE_ID)
    expect(store.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('does not render a dropdown menu on click', () => {
    const buffers = [makeEditorBuffer(0)]
    const store = setupStore(buffers)
    act(() => {
      renderTabBar(store, { onAddTab: vi.fn() })
    })

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: 'New tab' }))
    })

    expect(screen.queryByRole('menu')).toBeNull()
    expect(screen.queryByText('New Terminal')).toBeNull()
  })
})

// Spec §7.1 / task 17: tab-bar.tsx becomes the WHOLE pane-top row — split
// toggle, then the chat head (outside the tab scroller, no close, no
// reordering), then the editor tab strip in its own scroller.
describe('TabBar pane-top-row anatomy', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('the split toggle leads, before the chat name, outside the tab scroller', () => {
    const store = setupPaneStore({
      chatId: 'chat-1',
      buffers: [makeEditorBuffer(0)],
    })
    act(() => {
      renderTabBar(store)
    })

    const row = screen.getByTestId('pane-top-row')
    const children = Array.from(row.children).map((c) => c.getAttribute('data-role'))
    expect(children[0]).toBe('split-toggle')
    expect(children[1]).toBe('chat-head')
  })

  it('a pane with only its chat draws no tab-strip scroller at all', () => {
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [] })
    act(() => {
      renderTabBar(store)
    })
    expect(screen.queryByTestId('editor-tab-scroller')).not.toBeInTheDocument()
    // The row itself — split toggle and chat head — still draws.
    expect(screen.getByTestId('pane-top-row')).toBeInTheDocument()
    expect(screen.getByTestId('chat-head')).toBeInTheDocument()
  })

  it('the chat head has no close affordance', () => {
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [makeEditorBuffer(0)] })
    act(() => {
      renderTabBar(store)
    })
    expect(
      within(screen.getByTestId('chat-head')).queryByRole('button', { name: /close/i }),
    ).not.toBeInTheDocument()
  })

  it('the chat head shows the chat\'s own title', () => {
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [] })
    act(() => {
      renderTabBar(store)
    })
    expect(within(screen.getByTestId('chat-head')).getByText('My Chat')).toBeInTheDocument()
  })

  it('a pane holding no chat draws no chat head', () => {
    const store = setupPaneStore({ chatId: null, buffers: [makeEditorBuffer(0)] })
    act(() => {
      renderTabBar(store)
    })
    expect(screen.queryByTestId('chat-head')).not.toBeInTheDocument()
  })

  it('the split toggle flips PaneGroup.editorOpen', () => {
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [] })
    act(() => {
      renderTabBar(store)
    })
    expect(store.getState().panes[ROOT_PANE_ID]?.editorOpen).toBe(false)
    act(() => {
      fireEvent.click(screen.getByTestId('split-toggle'))
    })
    expect(store.getState().panes[ROOT_PANE_ID]?.editorOpen).toBe(true)
  })
})
