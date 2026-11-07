import { act, useRef } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { RecentsViewRow } from '@/components/sidebar/view-row'
import { useSidebarDrag } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { Repo } from '@/core/entities/repo-types'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { resetChatStore, setChatWorking, writeChat } from '@/__tests__/__fixtures__/agent-chat'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
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
  resetChatStore()
  writeChat({ id: 'chat-1', workspaceId: 'ws-1', title: 'Chat One' })
  writeChat({ id: 'chat-2', workspaceId: 'ws-1', title: 'Chat Two' })
  writeChat({ id: 'chat-3', workspaceId: 'ws-1', title: 'Chat Three' })
  seedRepos([repo(SIDEBAR_CHATS)])
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

  it('rows render flat, no indent', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    renderBand()
    expect(screen.getByTestId('recents-row-chat-1')).not.toHaveAttribute('data-depth')
  })

  it('clicking a row calls onFocus with its view', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    const { props } = renderBand()
    screen.getByRole('tab').click()
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
    setChatWorking('chat-1', true)
    renderBand()
    expect(screen.getByRole('button', { name: 'Close view' })).toBeInTheDocument()
    expect(document.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('switching views moves the lit row without any other state change', () => {
    groupAndSolo()
    renderBand()
    act(() => actions().activateView(viewOf('chat-1')))
    const group = screen.getByTestId(`recents-set-${viewOf('chat-1')}`)
    expect(group.getAttribute('data-view-showing')).toBe('true')
    expect(
      screen.getByTestId('recents-row-chat-3').parentElement!.getAttribute('data-view-showing'),
    ).toBeNull()
  })

  it('an untitled chat renders the UNTITLED_CHAT_LABEL fallback', () => {
    writeChat({ id: 'chat-1', title: '' })
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    renderBand()
    expect(screen.getByText(UNTITLED_CHAT_LABEL)).toBeInTheDocument()
  })

  it('a draft not yet sent is named as the new thread it is, never as loading', () => {
    actions().openChat('draft-1', { workspaceId: 'ws-1' })
    renderBand()
    expect(screen.getByTestId('recents-row-draft-1')).toHaveTextContent('New thread')
  })

  it("draws a workspace-owning chat's real icon, not the generic bubble", () => {
    seedRepos([
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
    ])
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    actions().openChat('chat-2', { workspaceId: 'ws-1' })
    renderBand()
    expect(within(screen.getByTestId('recents-row-chat-1')).queryByRole('img')).toBeInTheDocument()
    expect(
      within(screen.getByTestId('recents-row-chat-2')).queryByRole('img'),
    ).not.toBeInTheDocument()
  })

  // A record whose chat no cache knows still draws (loading) — a null row could never be closed.
  // Nothing is fetched for it; the sidebar's tree names the chat in time.
  describe('a row whose chat data has not resolved', () => {
    const unknownChat = () => {
      seedRepos([{ ...repo(SIDEBAR_CHATS), defaultWorkspaceId: 'ws-1' }])
      actions().adoptBackgroundChat('chat-9', 'p1', 'ws-1')
    }

    it('still draws with a loading label, and its × closes it', () => {
      unknownChat()
      const { props } = renderBand()
      expect(screen.getByTestId('recents-row-chat-9')).toBeInTheDocument()
      screen.getByRole('button', { name: 'Close view' }).click()
      expect(props.onClose).toHaveBeenCalledWith(viewOf('chat-9'))
    })
  })
})
