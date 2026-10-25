import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createElement } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import type { EditorContent } from '@/features/panes/types/pane-content'
import type { AgentChat } from '@/features/agent/api/agent-api'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useSettingsStore } from '@/features/settings/store'
import type { Chat, Repo } from '@/lib/store/sidebar'

// useSidebar() only feeds the reopen toggle; stubbed so no SidebarProvider is needed.
const sidebar = vi.hoisted(() => ({ open: true }))
vi.mock('@/components/ui/sidebar', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/ui/sidebar')>()
  return {
    ...actual,
    useSidebar: () => ({ open: sidebar.open, toggleSidebar: () => {} }),
  }
})

vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: () => createElement('span', { 'data-testid': 'file-icon' }),
}))

const menu = vi.hoisted(() => ({
  onRevealInFinder: undefined as undefined | ((path: string) => void),
}))
vi.mock('@/features/tabs/components/tab-context-menu', () => ({
  default: (props: { onRevealInFinder?: (path: string) => void }) => {
    menu.onRevealInFinder = props.onRevealInFinder
    return null
  },
}))

import TabBar from '@/features/tabs/components/tab-bar'
import * as fileTreeHandlers from '@/features/files/lib/file-tree-handlers'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

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

/** Seeds ROOT_PANE_ID on the window pane store with the given tabs and, when `chatId`
 *  is set, a matching chat on the returned workspace store. */
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
      editorTabIds: buffers.map((b) => b.id),
      activeEditorTabId,
    }
    return s
  })
  if (chatId) windowPaneStore.getState().paneActions.openChat(chatId)
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

    act(() => {
      renderTabBar(store)
    })

    const addButton = screen.getByRole('button', { name: 'New tab' })
    const scrollContainer = screen.getByTestId('editor-tab-scroller')
    expect(scrollContainer).not.toBeNull()

    // Inside the tabs' own scroller, after every tab.
    expect(addButton.parentElement).toBe(scrollContainer)
    expect(scrollContainer.lastElementChild).toBe(addButton)

    const tabs = screen.getAllByRole('tab')
    expect(tabs).toHaveLength(3)
    for (const tab of tabs) {
      expect(scrollContainer.contains(tab)).toBe(true)
    }
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
  // A different workspace is in view: what the "+" opens belongs to the pane's.
  beforeEach(() => {
    setActiveWorkspaceId('w-in-view')
  })
  afterEach(() => {
    vi.clearAllMocks()
    setActiveWorkspaceId(null)
  })

  it('focuses this pane and opens a New Tab page in it, with no menu', async () => {
    const user = userEvent.setup()
    const buffers = [makeEditorBuffer(0)]
    const store = setupPaneStore({ buffers, chatId: 'chat-1' })
    act(() => {
      renderTabBar(store, { wsId: 'w-pane' })
    })
    act(() => {
      windowPaneStore.getState().paneActions.setActivePane('some-other-pane-first')
    })

    await user.click(screen.getByRole('button', { name: 'New tab' }))

    expect(screen.queryByRole('menuitem')).toBeNull()
    expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
    const pane = windowPaneStore.getState().panes[ROOT_PANE_ID]
    expect(pane?.editorTabIds).toHaveLength(2)
    const page = windowPaneStore.getState().buffers.find((b) => b.id === pane?.editorTabIds[1])
    // The pane's own workspace, not the one in view.
    expect(page).toMatchObject({ type: 'newTabPage', name: 'New Tab', workspaceId: 'w-pane' })
    expect(pane?.activeEditorTabId).toBe(page?.id)
    expect(screen.getByRole('tab', { name: 'New Tab' })).toHaveAttribute('aria-selected', 'true')
  })

  it('a second + focuses the open New Tab instead of stacking another', async () => {
    const user = userEvent.setup()
    const store = setupPaneStore({ buffers: [makeEditorBuffer(0)], chatId: 'chat-1' })
    act(() => {
      renderTabBar(store, { wsId: 'w-pane' })
    })

    await user.click(screen.getByRole('button', { name: 'New tab' }))
    await user.click(screen.getByRole('tab', { name: 'file-0.ts' }))
    await user.click(screen.getByRole('button', { name: 'New tab' }))

    expect(screen.getAllByRole('tab', { name: 'New Tab' })).toHaveLength(1)
    const pane = windowPaneStore.getState().panes[ROOT_PANE_ID]
    expect(pane?.editorTabIds).toHaveLength(2)
    expect(pane?.activeEditorTabId).toBe(pane?.editorTabIds[1])
  })
})

// The IDE sector's own row: split toggle, the tab scroller, the branch-review shortcut.
// The chat draws its own header in the chat view.
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

  // The shared pane box paints none (it would block the chat view's vibrancy).
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

// A thread runs on its parent's worktree (`Chat.workspaceId` is inherited, so it can't
// gate this): only the worktree's owning chat gets branch chrome.
describe('TabBar — branch-review shortcut is for worktree-OWNING chats only', () => {
  afterEach(() => {
    useSidebarStore.setState({ repos: [] })
    vi.clearAllMocks()
  })

  function seedRepoChats(chatOwnsWorktree: boolean) {
    const owner: Chat = {
      id: chatOwnsWorktree ? 'chat-1' : 'chat-main',
      repoId: 'repo-1',
      title: 'main',
      order: 0,
      workspaceId: 'w1',
      ownsWorktree: true,
    }
    useSidebarStore.setState({
      repos: [
        {
          id: 'repo-1',
          projectId: 'proj-1',
          name: 'crowbar',
          avatarLabel: 'C',
          avatarColor: '#000',
          workspaces: [],
          defaultWorkspaceId: 'w1',
          defaultOwningChatId: owner.id,
          chats: chatOwnsWorktree
            ? [owner]
            : [
                owner,
                {
                  id: 'chat-1',
                  repoId: 'repo-1',
                  title: 'My Chat',
                  order: 1,
                  workspaceId: 'w1',
                  ownsWorktree: false,
                  parentId: 'chat-main',
                },
              ],
        } as Repo,
      ],
    })
  }

  it('hides it for a pane whose chat is a thread', () => {
    seedRepoChats(false)
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [makeEditorBuffer(0)] })
    act(() => {
      renderTabBar(store, { wsId: 'w1' })
    })
    expect(screen.queryByTestId('branch-review-shortcut')).not.toBeInTheDocument()
    // The rest of the IDE sector's row is untouched.
    expect(screen.getByTestId('editor-tab-scroller')).toBeInTheDocument()
  })

  it('keeps it for a pane whose chat owns its worktree', () => {
    seedRepoChats(true)
    const store = setupPaneStore({ chatId: 'chat-1', buffers: [makeEditorBuffer(0)] })
    act(() => {
      renderTabBar(store, { wsId: 'w1' })
    })
    expect(screen.getByTestId('branch-review-shortcut')).toBeInTheDocument()
  })

  // An editor-only split names no chat, so there is no thread to suppress.
  it('keeps it for an editor-only pane that holds no chat', () => {
    seedRepoChats(false)
    const store = setupPaneStore({ chatId: null, buffers: [makeEditorBuffer(0)] })
    act(() => {
      renderTabBar(store, { wsId: 'w1' })
    })
    expect(screen.getByTestId('branch-review-shortcut')).toBeInTheDocument()
  })
})

// An emptied split pane leaves the layout, so an empty pane here is the window's last:
// the "nothing is open" fallback, with no name, nothing to close and nothing to toggle.
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

// Collapsed, the chat is just another tab in the strip; pane-container decides when
// (`showChatTab`), TabBar only draws it.
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

    // activeEditorTabId keeps naming buf-0, so the editor view doesn't unmount it.
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

describe('TabBar context menu', () => {
  beforeEach(() => {
    menu.onRevealInFinder = undefined
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('reveals a tab’s file in the workspace the tab belongs to, not the pane’s', () => {
    const reveal = vi.spyOn(fileTreeHandlers, 'revealInFolder').mockImplementation(() => {})
    const store = setupStore([{ ...makeEditorBuffer(0), workspaceId: 'w-tab' }])
    act(() => {
      renderTabBar(store, { wsId: 'w-pane' })
    })

    act(() => {
      fireEvent.contextMenu(screen.getByRole('tab', { name: /file-0\.ts/ }))
    })
    menu.onRevealInFinder?.('src/a.ts')

    expect(reveal).toHaveBeenCalledExactlyOnceWith('w-tab', 'src/a.ts')
  })

  it('offers no Reveal while no tab’s menu is open, rather than guessing a workspace', () => {
    const store = setupStore([makeEditorBuffer(0)])
    act(() => {
      renderTabBar(store, { wsId: 'w-pane' })
    })

    expect(menu.onRevealInFinder).toBeUndefined()
  })

  // A remote workspace's path means nothing to this machine's Finder.
  it('offers no Reveal for a tab whose workspace lives on a remote host', () => {
    resetHostWorld()
    pairRemoteHost('host-1')
    loadProjects(projectOn('p-remote', 'host-1'))
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r1', wsId: 'w-remote' })
    const store = setupStore([{ ...makeEditorBuffer(0), workspaceId: 'w-remote' }])
    act(() => {
      renderTabBar(store, { wsId: 'w-remote' })
    })

    act(() => {
      fireEvent.contextMenu(screen.getByRole('tab', { name: /file-0\.ts/ }))
    })

    expect(menu.onRevealInFinder).toBeUndefined()
    resetHostWorld()
  })
})

// With the sidebar hidden, whoever draws the reopen toggle also carries the
// build readout: this row when no view strip exists, the strip otherwise.
describe('TabBar — build readout beside the reopen toggle', () => {
  afterEach(() => {
    sidebar.open = true
    vi.restoreAllMocks()
  })

  function renderHiddenAtEdge(side: 'left' | 'right') {
    sidebar.open = false
    useSettingsStore.setState((state) => {
      state.settings.buildBadgeOverride = 'beta'
      state.settings.sidebarPosition = side
    })
    // jsdom has no layout: report a row flush against the window's top-left and right edges.
    const rect = { left: 0, top: 0, width: window.innerWidth, height: 32, right: window.innerWidth }
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue(rect as DOMRect)
    const store = setupPaneStore({ buffers: [makeEditorBuffer(0)] })
    act(() => {
      renderTabBar(store)
    })
  }

  it.each(['left', 'right'] as const)(
    'shows the readout with the toggle when no view strip exists (sidebar %s)',
    (side) => {
      resetWindowPaneStoreForTests()
      renderHiddenAtEdge(side)
      expect(screen.getByRole('button', { name: 'Show sidebar' })).toBeInTheDocument()
      expect(screen.getAllByText(/beta 0\.0\.0-beta\.1/)).toHaveLength(1)
    },
  )

  it('leaves the readout to the view strip once a view exists', () => {
    renderHiddenAtEdge('left')
    expect(screen.getAllByText(/beta 0\.0\.0-beta\.1/)).toHaveLength(1)
    act(() => {
      const { paneActions } = windowPaneStore.getState()
      paneActions.setActiveProject('p1')
      paneActions.openChat('chat-1', { workspaceId: 'w1' })
    })
    expect(screen.queryByText(/beta 0\.0\.0-beta\.1/)).toBeNull()
  })
})
