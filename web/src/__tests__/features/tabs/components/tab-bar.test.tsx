import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createElement } from 'react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'
import type { AgentChat } from '@/features/agent/api/agent-api'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'

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
    tokens: [],
    workspaceId: 'w1',
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
 *  `chatId` is set, a matching `agentChats.chats` fixture. */
// Task 26: panes/buffers moved off the per-workspace store onto the
// window-level singleton — seed `windowPaneStore` for those; `agentChats`
// stays on the workspace store returned here.
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
    agentChats: {
      ...s.agentChats,
      chats: chatId ? [makeChat({ id: chatId })] : s.agentChats.chats,
    },
  }))
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers = buffers
    s.panes[ROOT_PANE_ID] = {
      ...s.panes[ROOT_PANE_ID],
      chatId,
      editorTabIds: buffers.map((b) => b.id),
      activeEditorTabId,
    }
    return s
  })
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
      windowPaneStore.setState((s) => {
        s.buffers.push(makeEditorBuffer(1))
        s.panes[ROOT_PANE_ID].editorTabIds.push('buf-1')
        return s
      })
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

  it('opens a dropdown offering New File and New Terminal', async () => {
    const user = userEvent.setup()
    const buffers = [makeEditorBuffer(0)]
    const store = setupStore(buffers)
    act(() => {
      renderTabBar(store, { wsId: 'w1' })
    })

    await user.click(screen.getByRole('button', { name: 'New tab' }))

    expect(await screen.findByRole('menuitem', { name: /New File/ })).toBeDefined()
    expect(screen.getByRole('menuitem', { name: /New Terminal/ })).toBeDefined()
  })

  it('"New File" focuses this pane and opens a new blank editor tab in it', async () => {
    const user = userEvent.setup()
    const buffers = [makeEditorBuffer(0)]
    const store = setupPaneStore({ buffers, chatId: 'chat-1' })
    act(() => {
      renderTabBar(store, { wsId: 'w1' })
    })
    act(() => {
      windowPaneStore.getState().paneActions.setActivePane('some-other-pane-first')
    })

    await user.click(screen.getByRole('button', { name: 'New tab' }))
    await user.click(await screen.findByRole('menuitem', { name: /New File/ }))

    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
    const editorTabIds = windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds ?? []
    expect(editorTabIds).toHaveLength(2)
    const newBuf = windowPaneStore.getState().buffers.find((b) => b.id === editorTabIds[1])
    expect(newBuf?.type).toBe('editor')
    expect((newBuf as EditorContent)?.isVirtual).toBe(true)
  })

  it('"New Terminal" opens a new terminal tab in this pane', async () => {
    const user = userEvent.setup()
    const buffers = [makeEditorBuffer(0)]
    const store = setupPaneStore({ buffers, chatId: 'chat-1' })
    act(() => {
      renderTabBar(store, { wsId: 'w1' })
    })

    await user.click(screen.getByRole('button', { name: 'New tab' }))
    await user.click(await screen.findByRole('menuitem', { name: /New Terminal/ }))

    const editorTabIds = windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorTabIds ?? []
    const newBuf = windowPaneStore.getState().buffers.find((b) => b.id === editorTabIds[1])
    expect(newBuf?.type).toBe('terminal')
  })
})

// Spec §7.1 (revised by the chats/pane redesign): tab-bar.tsx is now the
// IDE SECTOR's own row only — split toggle, the editor tab strip in its own
// scroller, then the branch-review shortcut. The chat is no
// longer part of this row at all: it draws its own identity header
// (`ChatBranchHeader`) at the top of the chat view instead
// (pane-container.test.tsx / chat-branch-header.test.tsx cover that).
describe('TabBar pane-top-row anatomy', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('the split toggle leads the row, outside the tab scroller', () => {
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
  })

  // Chats/pane redesign: the shared pane box no longer paints a background
  // of its own (it can't, without blocking the chat view's real vibrancy),
  // so the IDE sector's own opacity now rides on TabBar's row directly.
  it('paints its own opaque pane-background — the shared box no longer does', () => {
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [makeEditorBuffer(0)] })
    act(() => {
      renderTabBar(store)
    })

    expect(screen.getByTestId('pane-top-row')).toHaveClass('bg-pane-background')
  })

  it('draws no chat head of its own for a pane with only its chat', () => {
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [] })
    act(() => {
      renderTabBar(store)
    })
    expect(screen.queryByTestId('editor-tab-scroller')).not.toBeInTheDocument()
    expect(screen.queryByTestId('chat-head')).not.toBeInTheDocument()
    // The row itself — split toggle — still draws.
    expect(screen.getByTestId('pane-top-row')).toBeInTheDocument()
    expect(screen.getByTestId('split-toggle')).toBeInTheDocument()
  })

  it('the split toggle flips PaneGroup.editorOpen', () => {
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [] })
    act(() => {
      renderTabBar(store)
    })
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorOpen).toBe(false)
    act(() => {
      fireEvent.click(screen.getByTestId('split-toggle'))
    })
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.editorOpen).toBe(true)
  })
})

/**
 * "The empty case is treated as a normal view. It shouldn't be treated like
 * that — it should only appear when NO VIEW is opened. It's just a fallback
 * when nothing is found, not a normal view."
 *
 * An emptied pane in a split now leaves the layout outright
 * (`dropEmptiedPanes`, pane-slice.ts), so the only pane that reaches TabBar
 * holding nothing is the last one in the window — the "nothing is open here"
 * screen. It has no name to show, nothing to close, and no second view to
 * toggle beside a chat that isn't there.
 */
describe('TabBar — a pane holding nothing draws no chrome for it', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('draws no split toggle and no tab strip', () => {
    const store = setupPaneStore({ chatId: null, buffers: [] })
    act(() => {
      renderTabBar(store)
    })

    expect(screen.queryByTestId('split-toggle')).not.toBeInTheDocument()
    expect(screen.queryByTestId('editor-tab-scroller')).not.toBeInTheDocument()
  })

  it('keeps the row itself — it carries the window drag region and the traffic-light inset', () => {
    const store = setupPaneStore({ chatId: null, buffers: [] })
    act(() => {
      renderTabBar(store)
    })

    const row = screen.getByTestId('pane-top-row')
    expect(row).toBeInTheDocument()
    expect(row).toHaveAttribute('data-tauri-drag-region')
    expect(row.children).toHaveLength(0)
  })

  it('the same pane with a chat in it draws the full row again', () => {
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [] })
    act(() => {
      renderTabBar(store)
    })

    expect(screen.getByTestId('split-toggle')).toBeInTheDocument()
  })
})

// Chats/pane redesign: in the collapsed presentation, the chat is "just
// another tab" in the strip — `showChatTab` is pane-container.tsx's own
// signal for exactly that state (chat + real editor tabs + narrow/toggled
// off). TabBar itself doesn't know about `presentation`; it only draws
// what it's told to.
describe('TabBar — chat as a tab (showChatTab)', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('renders the chat as the FIRST entry in the scroller, before real tabs, when showChatTab is set', () => {
    const store = setupPaneStore({
      chatId: 'chat-1',
      buffers: [makeEditorBuffer(0)],
      activeEditorTabId: null,
    })
    act(() => {
      renderTabBar(store, { showChatTab: true })
    })

    const scroller = screen.getByTestId('editor-tab-scroller')
    const chatTab = screen.getByTestId('chat-tab-item')
    expect(scroller.contains(chatTab)).toBe(true)
    expect(scroller.firstElementChild).toBe(chatTab)
  })

  it('never renders the chat tab when showChatTab is not set (default)', () => {
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [makeEditorBuffer(0)] })
    act(() => {
      renderTabBar(store)
    })

    expect(screen.queryByTestId('chat-tab-item')).not.toBeInTheDocument()
  })

  it('is active exactly when the pane has chatSelected (defaults true — unset reads as selected)', () => {
    const store = setupPaneStore({
      chatId: 'chat-1',
      buffers: [makeEditorBuffer(0)],
    })
    act(() => {
      renderTabBar(store, { showChatTab: true })
    })

    expect(screen.getByTestId('chat-tab-item')).toHaveAttribute('aria-selected', 'true')
    const fileTab = screen.getByRole('tab', { name: /file-0\.ts/ })
    expect(fileTab).toHaveAttribute('aria-selected', 'false')
  })

  it('clicking it selects the chat and activates this pane', () => {
    const store = setupPaneStore({
      chatId: 'chat-1',
      buffers: [makeEditorBuffer(0)],
      activeEditorTabId: 'buf-0',
    })
    act(() => {
      renderTabBar(store, { showChatTab: true })
    })

    act(() => {
      windowPaneStore.getState().paneActions.setActivePane('some-other-pane-first')
    })

    act(() => {
      fireEvent.click(screen.getByTestId('chat-tab-item'))
    })

    // Selects the chat WITHOUT clearing activeEditorTabId — that field must
    // keep naming buf-0 so the editor view doesn't unmount it on switch
    // (PaneGroup.chatSelected's own doc).
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatSelected).toBe(true)
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.activeEditorTabId).toBe('buf-0')
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
  })

  it('clicking a real tab while the chat is selected switches away from the chat', () => {
    const store = setupPaneStore({
      chatId: 'chat-1',
      buffers: [makeEditorBuffer(0)],
      activeEditorTabId: null,
    })
    act(() => {
      renderTabBar(store, { showChatTab: true })
    })

    act(() => {
      fireEvent.click(screen.getByRole('tab', { name: /file-0\.ts/ }))
    })

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.activeEditorTabId).toBe('buf-0')
  })
})
