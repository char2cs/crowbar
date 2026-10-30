import { act, useRef } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { RecentsViewRow } from '@/components/sidebar/view-row'
import { useSidebarDrag } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { newDraftChatId } from '@/features/agent/lib/draft-chat'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'

vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

interface FakeChat {
  id: string
  workspaceId: string
  title: string
}

const { stores } = vi.hoisted(() => ({
  stores: {
    current: new Map<string, { chats: FakeChat[]; working: Record<string, boolean> }>(),
  },
}))

// Each member row resolves its chat through its OWN workspace's registered
// store; a workspace absent here has no store, exactly like an unmounted one.
vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@/features/workspace/stores/workspace-store-registry')
  >()),
  getWorkspaceStore: (wsId: string) => {
    const agentChats = stores.current.get(wsId)
    if (!agentChats) return undefined
    return { subscribe: () => () => {}, getState: () => ({ agentChats }) }
  },
}))

const DRAG_PROPS = {
  scrollRef: { current: null } as React.RefObject<HTMLElement | null>,
  onDrop: vi.fn(),
  onPaneDrop: vi.fn(),
}

function repo(chats: Repo['chats'], workspaces: Repo['workspaces'] = []): Repo {
  return {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces,
    chats,
  }
}

const SIDEBAR_CHATS: Repo['chats'] = [
  { id: 'chat-1', repoId: 'r1', title: 'Chat One', order: 0, workspaceId: 'ws-1' },
  { id: 'chat-2', repoId: 'r1', title: 'Chat Two', order: 1, workspaceId: 'ws-1' },
  { id: 'chat-3', repoId: 'r1', title: 'Chat Three', order: 2, workspaceId: 'ws-1' },
]

beforeEach(() => {
  stores.current = new Map([
    [
      'ws-1',
      {
        chats: [
          { id: 'chat-1', workspaceId: 'ws-1', title: 'Chat One' },
          { id: 'chat-2', workspaceId: 'ws-1', title: 'Chat Two' },
          { id: 'chat-3', workspaceId: 'ws-1', title: 'Chat Three' },
        ],
        working: {},
      },
    ],
  ])
  useSidebarStore.setState({ ...getInitialState(), repos: [repo(SIDEBAR_CHATS)] })
  resetWindowPaneStoreForTests()
  windowPaneStore.getState().paneActions.setActiveProject('p1')
})

const actions = () => windowPaneStore.getState().paneActions
const paneOf = (chatId: string) => chatPaneIndex(windowPaneStore.getState().panes).get(chatId)!
const viewOf = (chatId: string) => windowPaneStore.getState().panes[paneOf(chatId)].viewId!

interface BandProps {
  viewIds: readonly string[]
  onFocus: (viewId: string) => void
  onClose: (viewId: string) => void
}

function Band({ viewIds, onFocus, onClose }: BandProps) {
  const rows = useRef(new Map<string, SidebarRowType>())
  const drag = useSidebarDrag({
    ...DRAG_PROPS,
    subjectsFor: (rowId) => {
      const row = rows.current.get(rowId)
      return row ? [row] : []
    },
  })
  return (
    <div>
      {viewIds.map((viewId) => (
        <RecentsViewRow
          key={viewId}
          viewId={viewId}
          onFocus={onFocus}
          onClose={onClose}
          drag={drag}
          registerRow={(row) => rows.current.set(row.id, row)}
        />
      ))}
    </div>
  )
}

function renderBand(overrides: Partial<BandProps> = {}) {
  const props = {
    viewIds: windowPaneStore.getState().viewOrder,
    onFocus: vi.fn(),
    onClose: vi.fn(),
    ...overrides,
  }
  return { ...render(<Band {...props} />), props }
}

function classesOf(el: Element): string[] {
  return el.className.split(/\s+/).filter(Boolean)
}

/** A group of chat-1 + chat-2 in one view, plus chat-3 as its own view. */
function groupAndSolo() {
  actions().openChat('chat-1', { workspaceId: 'ws-1' })
  actions().dropChatOnPane('chat-2', paneOf('chat-1'), 'right', 'ws-1')
  actions().openChat('chat-3', { workspaceId: 'ws-1' })
}

describe('RecentsViewRow', () => {
  it('one row per view: a group draws as one shell around its members', () => {
    groupAndSolo()
    renderBand()
    const shell = screen.getByTestId(`recents-set-${viewOf('chat-1')}`)
    expect(within(shell).getAllByTestId(/^recents-row-/)).toHaveLength(2)
    expect(screen.getAllByTestId(/^recents-row-/)).toHaveLength(3)
  })

  it('a pinned row is its glyph alone: no title text, no ×, the title kept as its label', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    actions().setViewPinned(viewOf('chat-1'), true)
    renderBand()
    expect(screen.queryByRole('button', { name: 'Close view' })).not.toBeInTheDocument()
    expect(screen.getByRole('treeitem', { name: 'Chat One' })).toHaveAttribute('title', 'Chat One')
  })

  it('a pinned group keeps every member and still has no ×', () => {
    groupAndSolo()
    actions().setViewPinned(viewOf('chat-1'), true)
    renderBand()
    const shell = screen.getByTestId(`recents-set-${viewOf('chat-1')}`)
    expect(within(shell).getAllByTestId(/^recents-row-/)).toHaveLength(2)
    expect(within(shell).queryByRole('button', { name: 'Close view' })).not.toBeInTheDocument()
  })

  it('rows render flat, no indent', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    renderBand()
    expect(screen.getByTestId('recents-row-chat-1')).not.toHaveAttribute('data-depth')
  })

  it('clicking a row calls onFocus with its view', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    const { props } = renderBand()
    screen.getByRole('treeitem').click()
    expect(props.onFocus).toHaveBeenCalledWith(viewOf('chat-1'))
  })

  it("a solo row's × closes its view, never labelled as a delete", () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    const { props } = renderBand()
    const close = screen.getByRole('button', { name: 'Close view' })
    expect(close.getAttribute('aria-label')).not.toMatch(/delete/i)
    close.click()
    expect(props.onClose).toHaveBeenCalledWith(viewOf('chat-1'))
  })

  it('a split has one ×, which closes the whole view', () => {
    groupAndSolo()
    const { props } = renderBand()
    expect(screen.queryByRole('button', { name: 'Close Chat Two' })).not.toBeInTheDocument()
    const shell = screen.getByTestId(`recents-set-${viewOf('chat-1')}`)
    within(shell).getByRole('button', { name: 'Close view' }).click()
    expect(props.onClose).toHaveBeenCalledWith(viewOf('chat-1'))
  })

  it('every row has a close control — a working one included', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    stores.current.get('ws-1')!.working = { 'chat-1': true }
    renderBand()
    expect(screen.getByRole('button', { name: 'Close view' })).toBeInTheDocument()
    expect(document.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('only the showing view is painted; an off-screen group takes no ground', () => {
    groupAndSolo()
    renderBand()
    const group = screen.getByTestId(`recents-set-${viewOf('chat-1')}`)
    expect(group.className).not.toMatch(/bg-background/)
    expect(group.getAttribute('data-view-showing')).toBeNull()
    const solo = screen.getByTestId('recents-row-chat-3').parentElement!
    expect(solo.getAttribute('data-view-showing')).toBe('true')
  })

  it('switching views moves the lit row without any other state change', () => {
    groupAndSolo()
    renderBand()
    act(() => actions().activateView(viewOf('chat-1')))
    const group = screen.getByTestId(`recents-set-${viewOf('chat-1')}`)
    expect(group.getAttribute('data-view-showing')).toBe('true')
    expect(group.className).toMatch(/bg-background/)
    expect(
      screen.getByTestId('recents-row-chat-3').parentElement!.getAttribute('data-view-showing'),
    ).toBeNull()
  })

  it('every tab is one h-8 shell with no margin of its own on the row', () => {
    groupAndSolo()
    renderBand()
    const solo = screen.getByTestId('recents-row-chat-3').parentElement!
    const group = screen.getByTestId(`recents-set-${viewOf('chat-1')}`)
    for (const shell of [solo, group]) {
      expect(classesOf(shell)).toEqual(expect.arrayContaining(['mx-0.5', 'h-8', 'rounded-full']))
    }
    for (const rowId of ['recents-row-chat-1', 'recents-row-chat-2', 'recents-row-chat-3']) {
      const treeitem = within(screen.getByTestId(rowId)).getByRole('treeitem')
      expect(classesOf(treeitem)).toEqual(expect.arrayContaining(['h-8', 'mx-0', 'my-0']))
    }
  })

  it('an untitled chat renders the UNTITLED_CHAT_LABEL fallback', () => {
    stores.current.get('ws-1')!.chats[0] = { id: 'chat-1', workspaceId: 'ws-1', title: '' }
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    renderBand()
    expect(screen.getByText(UNTITLED_CHAT_LABEL)).toBeInTheDocument()
  })

  it("draws a workspace-owning chat's real icon, not the generic bubble", () => {
    useSidebarStore.setState({
      repos: [
        repo(SIDEBAR_CHATS, [
          {
            id: 'ws-1',
            branch: 'feature/x',
            age: '',
            owningChatId: 'chat-1',
            status: 'locked',
            provisioning: 'placeholder',
          },
        ]),
      ],
    })
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    actions().openChat('chat-2', { workspaceId: 'ws-1' })
    renderBand()
    expect(within(screen.getByTestId('recents-row-chat-1')).queryByRole('img')).toBeInTheDocument()
    expect(
      within(screen.getByTestId('recents-row-chat-2')).queryByRole('img'),
    ).not.toBeInTheDocument()
  })

  // Regression: a working chat whose workspace store is not mounted keeps its
  // row — the row is its record, drawn from the sidebar's own chat list.
  it('a background row survives its workspace store unmounting', () => {
    actions().adoptBackgroundChat('chat-2', 'p1', 'ws-1')
    stores.current = new Map()
    renderBand()
    expect(screen.getByTestId('recents-row-chat-2')).toBeInTheDocument()
    expect(screen.getByText('Chat Two')).toBeInTheDocument()
  })

  // Regression: a record whose chat no cache knew rendered null — an
  // invisible row nobody could close. It draws from its record (loading)
  // until the sidebar's tree names the chat; nothing is fetched for it.
  describe('a row whose chat data has not resolved', () => {
    const unknownChat = () => {
      useSidebarStore.setState({
        repos: [{ ...repo(SIDEBAR_CHATS), defaultWorkspaceId: 'ws-1' }],
      })
      actions().adoptBackgroundChat('chat-9', 'p1', 'ws-1')
    }

    it('still draws with a loading label, and its × closes it', () => {
      unknownChat()
      const { props } = renderBand()
      expect(screen.getByTestId('recents-row-chat-9')).toBeInTheDocument()
      screen.getByRole('button', { name: 'Close view' }).click()
      expect(props.onClose).toHaveBeenCalledWith(viewOf('chat-9'))
    })

    // A new thread has no chat on the daemon until its first prompt: nothing is coming to name it.
    it('names an unsent new thread untitled, not loading', () => {
      const draft = newDraftChatId()
      actions().openChat(draft, { workspaceId: 'ws-1' })
      renderBand()
      expect(
        within(screen.getByTestId(`recents-row-${draft}`)).getByText(UNTITLED_CHAT_LABEL),
      ).toBeInTheDocument()
      expect(screen.queryByText('Loading…')).toBeNull()
    })
  })
})
