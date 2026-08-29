import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { RecentsBand, type RecentsBandEntry } from '@/components/sidebar/recents-band'

interface FakeChat {
  id: string
  workspaceId: string
  title: string
}

const DEFAULT_CHATS: FakeChat[] = [
  { id: 'chat-1', workspaceId: 'ws-1', title: 'Chat One' },
  { id: 'chat-2', workspaceId: 'ws-1', title: 'Chat Two' },
]

const { stores } = vi.hoisted(() => ({
  stores: {
    current: new Map<string, { chats: FakeChat[]; working: Record<string, boolean> }>(),
  },
}))

// `RecentsBand` now resolves each chat via `useWorkspaceStoreById(workspaceId,
// ...)` — no ambient `WorkspaceStoreContext.Provider` needed — since a
// project's Recents can span more than one workspace's store (spec §4). The
// mock is keyed by workspaceId, so a test can prove an entry is resolved
// against ITS OWN workspace's data rather than one shared fixture.
vi.mock('@/features/workspace/stores/hooks/use-workspace-store-by-id', () => ({
  useWorkspaceStoreById: (wsId: string, sel: (s: unknown) => unknown) =>
    sel({ agentChats: stores.current.get(wsId) ?? { chats: [], working: {} } }),
}))

beforeEach(() => {
  stores.current = new Map([['ws-1', { chats: DEFAULT_CHATS, working: {} }]])
})

/** Convenience: point 'ws-1' at a fresh chat/working fixture pair. */
function setWs1(chats: FakeChat[], working: Record<string, boolean> = {}) {
  stores.current.set('ws-1', { chats, working })
}

describe('RecentsBand', () => {
  it('renders nothing when there are no entries', () => {
    const { container } = render(<RecentsBand entries={[]} onFocus={vi.fn()} onClose={vi.fn()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('a working entry has no close control', () => {
    setWs1(DEFAULT_CHATS, { 'chat-1': true })
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1'], state: 'working', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    expect(screen.queryByTestId('recents-close-e1')).not.toBeInTheDocument()
    // §5.6: the spinner still rides the member — its absence isn't what hid the close control.
    expect(document.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('a set draws as one shell around its member rows', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(within(shell).getAllByTestId(/^recents-row-/)).toHaveLength(2)
  })

  it('entries render flat, no indent', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1'], state: 'dormant', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    expect(screen.getByTestId('recents-row-chat-1')).not.toHaveAttribute('data-depth')
  })

  it('clicking a row calls onFocus with its entry', () => {
    const onFocus = vi.fn()
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={onFocus} onClose={vi.fn()} />)
    screen.getByRole('treeitem').click()
    expect(onFocus).toHaveBeenCalledWith(entry)
  })

  it('clicking close calls onClose with the entry, not the chat', () => {
    const onClose = vi.fn()
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={onClose} />)
    screen.getByTestId('recents-close-e1').click()
    expect(onClose).toHaveBeenCalledWith(entry)
  })

  it('the close control is never labelled as a delete', () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} />)
    const close = screen.getByTestId('recents-close-e1')
    expect(close.getAttribute('aria-label')).not.toMatch(/delete/i)
    // The tree's trash control is what this must NOT render for a Recents row.
    expect(screen.queryByRole('button', { name: /delete/i })).not.toBeInTheDocument()
  })

  it('a live set lights the shell, not the members', () => {
    const entries: RecentsBandEntry[] = [
      {
        id: 'e1',
        localId: 'e1',
        chatIds: ['chat-1', 'chat-2'],
        state: 'live',
        workspaceId: 'ws-1',
      },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(shell.className).toMatch(/bg-background/)
  })

  it('a dormant (at-rest) set does not light the shell', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(shell.className).not.toMatch(/bg-background/)
    expect(shell.className).toMatch(/bg-sidebar-element-idle/)
  })

  it('reserves room for the close control so a long title truncates before it, not under it', () => {
    // A title long enough to truncate right at the row's edge is the realistic
    // case where an unreserved close button would draw over the last
    // characters (tab-bar-item.tsx's own `pr-8` exists for the identical
    // reason). We can't measure real layout in jsdom, so assert the reserved
    // class directly, on the same element SidebarRow renders into.
    setWs1([{ id: 'chat-1', workspaceId: 'ws-1', title: 'A'.repeat(120) }])
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} />)
    const rowWrapper = screen.getByTestId('recents-row-chat-1')
    expect(rowWrapper.className).toMatch(/\bpr-10\b/)
  })

  it('does not reserve close-button room on a working row, which has no close control', () => {
    setWs1(DEFAULT_CHATS, { 'chat-1': true })
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'working',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} />)
    const rowWrapper = screen.getByTestId('recents-row-chat-1')
    expect(rowWrapper.className).not.toMatch(/\bpr-10\b/)
  })

  it('resolves each entry through its OWN workspace store, not one shared assumption', () => {
    // The whole reason for the workspaceId tag: a project's Recents can mix
    // chats from more than one workspace's store (spec §4). 'chat-2' exists
    // ONLY in 'ws-other's fixture, not in 'ws-1's (DEFAULT_CHATS) — if the
    // component ever fell back to a single ambient store, this entry would
    // silently render nothing (RecentsMemberRow returns null when `chat` is
    // undefined).
    stores.current.set('ws-other', {
      chats: [{ id: 'chat-2', workspaceId: 'ws-other', title: 'Other space chat' }],
      working: {},
    })
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1'], state: 'dormant', workspaceId: 'ws-1' },
      { id: 'e2', localId: 'e2', chatIds: ['chat-2'], state: 'dormant', workspaceId: 'ws-other' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    expect(screen.getByTestId('recents-row-chat-1')).toHaveTextContent('Chat One')
    expect(screen.getByTestId('recents-row-chat-2')).toHaveTextContent('Other space chat')
  })
})
