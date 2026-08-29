import { describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { RecentsBand, type RecentsEntry } from '@/components/sidebar/recents-band'

interface FakeChat {
  id: string
  workspaceId: string
  title: string
}

const { chats, working } = vi.hoisted(() => ({
  chats: {
    current: [
      { id: 'chat-1', workspaceId: 'ws-1', title: 'Chat One' },
      { id: 'chat-2', workspaceId: 'ws-1', title: 'Chat Two' },
    ] as FakeChat[],
  },
  working: { current: {} as Record<string, boolean> },
}))

vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStoreContext: (sel: (s: unknown) => unknown) =>
    sel({ agentChats: { chats: chats.current, working: working.current } }),
}))

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
    working.current = {}
    const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set' }]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(within(shell).getAllByTestId(/^recents-row-/)).toHaveLength(2)
  })

  it('entries render flat, no indent', () => {
    working.current = {}
    const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1'], state: 'dormant' }]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    expect(screen.getByTestId('recents-row-chat-1')).not.toHaveAttribute('data-depth')
  })

  it('clicking a row calls onFocus with its entry', () => {
    working.current = {}
    const onFocus = vi.fn()
    const entry: RecentsEntry = { id: 'e1', chatIds: ['chat-1'], state: 'dormant' }
    render(<RecentsBand entries={[entry]} onFocus={onFocus} onClose={vi.fn()} />)
    screen.getByRole('treeitem').click()
    expect(onFocus).toHaveBeenCalledWith(entry)
  })

  it('clicking close calls onClose with the entry, not the chat', () => {
    working.current = {}
    const onClose = vi.fn()
    const entry: RecentsEntry = { id: 'e1', chatIds: ['chat-1'], state: 'live' }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={onClose} />)
    screen.getByTestId('recents-close-e1').click()
    expect(onClose).toHaveBeenCalledWith(entry)
  })

  it('the close control is never labelled as a delete', () => {
    working.current = {}
    const entry: RecentsEntry = { id: 'e1', chatIds: ['chat-1'], state: 'dormant' }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} />)
    const close = screen.getByTestId('recents-close-e1')
    expect(close.getAttribute('aria-label')).not.toMatch(/delete/i)
    // The tree's trash control is what this must NOT render for a Recents row.
    expect(screen.queryByRole('button', { name: /delete/i })).not.toBeInTheDocument()
  })

  it('a live set lights the shell, not the members', () => {
    working.current = {}
    const entries: RecentsEntry[] = [{ id: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'live' }]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(shell.className).toMatch(/bg-background/)
  })
})
