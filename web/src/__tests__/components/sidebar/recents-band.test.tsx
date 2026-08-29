import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { RecentsBand, type RecentsEntry } from '@/components/sidebar/recents-band'

interface FakeChat {
  id: string
  workspaceId: string
  title: string
}

const DEFAULT_CHATS: FakeChat[] = [
  { id: 'chat-1', workspaceId: 'ws-1', title: 'Chat One' },
  { id: 'chat-2', workspaceId: 'ws-1', title: 'Chat Two' },
]

const { chats, working } = vi.hoisted(() => ({
  chats: { current: [] as FakeChat[] },
  working: { current: {} as Record<string, boolean> },
}))

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStoreContext: (sel: (s: unknown) => unknown) =>
    sel({ agentChats: { chats: chats.current, working: working.current } }),
}))

beforeEach(() => {
  chats.current = DEFAULT_CHATS
  working.current = {}
})

describe('RecentsBand', () => {
  it('renders nothing when there are no entries', () => {
    const { container } = render(<RecentsBand entries={[]} onFocus={vi.fn()} onClose={vi.fn()} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('a working entry has no close control', () => {
    working.current = { 'chat-1': true }
    const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1'], state: 'working' }]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    expect(screen.queryByTestId('recents-close-e1')).not.toBeInTheDocument()
    // §5.6: the spinner still rides the member — its absence isn't what hid the close control.
    expect(document.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('a set draws as one shell around its member rows', () => {
    const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set' }]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(within(shell).getAllByTestId(/^recents-row-/)).toHaveLength(2)
  })

  it('entries render flat, no indent', () => {
    const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1'], state: 'dormant' }]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    expect(screen.getByTestId('recents-row-chat-1')).not.toHaveAttribute('data-depth')
  })

  it('clicking a row calls onFocus with its entry', () => {
    const onFocus = vi.fn()
    const entry: RecentsEntry = { id: 'e1', chatIds: ['chat-1'], state: 'dormant' }
    render(<RecentsBand entries={[entry]} onFocus={onFocus} onClose={vi.fn()} />)
    screen.getByRole('treeitem').click()
    expect(onFocus).toHaveBeenCalledWith(entry)
  })

  it('clicking close calls onClose with the entry, not the chat', () => {
    const onClose = vi.fn()
    const entry: RecentsEntry = { id: 'e1', chatIds: ['chat-1'], state: 'live' }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={onClose} />)
    screen.getByTestId('recents-close-e1').click()
    expect(onClose).toHaveBeenCalledWith(entry)
  })

  it('the close control is never labelled as a delete', () => {
    const entry: RecentsEntry = { id: 'e1', chatIds: ['chat-1'], state: 'dormant' }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} />)
    const close = screen.getByTestId('recents-close-e1')
    expect(close.getAttribute('aria-label')).not.toMatch(/delete/i)
    // The tree's trash control is what this must NOT render for a Recents row.
    expect(screen.queryByRole('button', { name: /delete/i })).not.toBeInTheDocument()
  })

  it('a live set lights the shell, not the members', () => {
    const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'live' }]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(shell.className).toMatch(/bg-background/)
  })

  it('a dormant (at-rest) set does not light the shell', () => {
    const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set' }]
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
    chats.current = [{ id: 'chat-1', workspaceId: 'ws-1', title: 'A'.repeat(120) }]
    const entry: RecentsEntry = { id: 'e1', chatIds: ['chat-1'], state: 'dormant' }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} />)
    const rowWrapper = screen.getByTestId('recents-row-chat-1')
    expect(rowWrapper.className).toMatch(/\bpr-10\b/)
  })

  it('does not reserve close-button room on a working row, which has no close control', () => {
    working.current = { 'chat-1': true }
    const entry: RecentsEntry = { id: 'e1', chatIds: ['chat-1'], state: 'working' }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} />)
    const rowWrapper = screen.getByTestId('recents-row-chat-1')
    expect(rowWrapper.className).not.toMatch(/\bpr-10\b/)
  })
})
