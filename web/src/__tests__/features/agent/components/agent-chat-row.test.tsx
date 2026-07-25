import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentChatRow } from '@/features/agent/components/agent-chat-row'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'

// The row reads its OWN turn state from the workspace store (like
// AgentChatTabIcon) rather than a `working` prop, so drive "is working" through
// the store, not props. Keep the store out of IndexedDB.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

const base = {
  wsId: 'w1',
  chatId: 'c1',
  title: 'My chat',
  providerIcon: '<svg data-icon="claude"/>',
  active: false,
  renaming: false,
  onSelect: vi.fn(),
  onStartRename: vi.fn(),
  onConfirmRename: vi.fn(),
  onCancelRename: vi.fn(),
  onPointerDownDrag: vi.fn(),
}

const setWorking = (chatId: string, working: boolean) =>
  act(() => getOrCreateWorkspaceStore('w1').getState().setAgentChatWorking(chatId, working))

beforeEach(() => {
  getOrCreateWorkspaceStore('w1')
})

afterEach(() => {
  cleanup()
  destroyWorkspaceStore('w1')
  vi.clearAllMocks()
})

describe('AgentChatRow', () => {
  it('renders the provider icon when idle and the spinner when its own chat is working', () => {
    const { container } = render(<AgentChatRow {...base} />)
    expect(container.querySelector('[data-icon="claude"]')).not.toBeNull()
    expect(screen.queryByRole('status')).toBeNull()

    // Working state comes from the store; the row self-subscribes and swaps in
    // the spinner without any prop change.
    setWorking('c1', true)
    expect(screen.getByRole('status')).toBeTruthy()
    expect(container.querySelector('[data-icon="claude"]')).toBeNull()
  })

  it('re-renders only for its OWN chat’s working state, not a sibling’s', () => {
    const { container } = render(<AgentChatRow {...base} />)
    // A different chat going busy must not flip THIS row to the spinner.
    setWorking('c2', true)
    expect(screen.queryByRole('status')).toBeNull()
    expect(container.querySelector('[data-icon="claude"]')).not.toBeNull()
  })

  it('single-click selects with the chat id, double-click starts rename', () => {
    const onSelect = vi.fn()
    const onStartRename = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} onStartRename={onStartRename} />)
    const row = screen.getByText('My chat')
    fireEvent.click(row)
    expect(onSelect).toHaveBeenCalledWith('c1')
    fireEvent.doubleClick(row)
    expect(onStartRename).toHaveBeenCalledWith('c1')
  })

  it('renaming renders the inline input seeded with the title', () => {
    render(<AgentChatRow {...base} renaming />)
    expect(screen.getByDisplayValue('My chat')).toBeTruthy()
    expect(screen.queryByText('My chat')).toBeNull()
  })

  it('confirming the inline input calls onConfirmRename with the chat id and new title', () => {
    const onConfirmRename = vi.fn()
    render(<AgentChatRow {...base} renaming onConfirmRename={onConfirmRename} />)
    const input = screen.getByDisplayValue('My chat')
    fireEvent.change(input, { target: { value: 'Renamed chat' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirmRename).toHaveBeenCalledWith('c1', 'Renamed chat')
  })

  it('renames in the row’s own face, not monospace — a chat title is prose, not a branch name', () => {
    render(<AgentChatRow {...base} renaming />)
    expect(screen.getByDisplayValue('My chat').className).not.toContain('font-mono')
  })

  it('cancelling the inline input (Escape) calls onCancelRename', () => {
    const onCancelRename = vi.fn()
    render(<AgentChatRow {...base} renaming onCancelRename={onCancelRename} />)
    const input = screen.getByDisplayValue('My chat')
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(onCancelRename).toHaveBeenCalledTimes(1)
  })

  it('while renaming, clicking/double-clicking the row does not select or re-trigger rename', () => {
    const onSelect = vi.fn()
    const onStartRename = vi.fn()
    render(<AgentChatRow {...base} renaming onSelect={onSelect} onStartRename={onStartRename} />)
    const row = screen.getByRole('button')
    fireEvent.click(row)
    fireEvent.doubleClick(row)
    expect(onSelect).not.toHaveBeenCalled()
    expect(onStartRename).not.toHaveBeenCalled()
  })

  it('while renaming, pointer-down on the row does not start a drag', () => {
    const onPointerDownDrag = vi.fn()
    render(<AgentChatRow {...base} renaming onPointerDownDrag={onPointerDownDrag} />)
    const row = screen.getByRole('button')
    fireEvent.pointerDown(row)
    expect(onPointerDownDrag).not.toHaveBeenCalled()
  })

  it('while renaming, Enter/Space on the row do not select', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} renaming onSelect={onSelect} />)
    const row = screen.getByRole('button')
    fireEvent.keyDown(row, { key: 'Enter' })
    fireEvent.keyDown(row, { key: ' ' })
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('Enter and Space on the focused row call onSelect with the chat id', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} />)
    const row = screen.getByRole('button')
    fireEvent.keyDown(row, { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledTimes(1)
    fireEvent.keyDown(row, { key: ' ' })
    expect(onSelect).toHaveBeenCalledTimes(2)
    expect(onSelect).toHaveBeenLastCalledWith('c1')
  })

  it('ignores other keys on the row', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} />)
    const row = screen.getByRole('button')
    fireEvent.keyDown(row, { key: 'Tab' })
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('applies the active row class when active, inactive class otherwise', () => {
    const { rerender } = render(<AgentChatRow {...base} active={false} />)
    let row = screen.getByRole('button')
    expect(row.className).toContain('hover:bg-accent')

    rerender(<AgentChatRow {...base} active />)
    row = screen.getByRole('button')
    expect(row.className).toContain('bg-background')
  })

  it('exposes a drag drop-target attribute and forwards pointer-down (with the chat id) to the drag handler', () => {
    const onPointerDownDrag = vi.fn()
    render(<AgentChatRow {...base} onPointerDownDrag={onPointerDownDrag} />)
    const row = screen.getByRole('button')
    expect(row.getAttribute('data-agent-chat-drop')).toBe('c1')
    fireEvent.pointerDown(row)
    expect(onPointerDownDrag).toHaveBeenCalledTimes(1)
    expect(onPointerDownDrag.mock.calls[0][0]).toBe('c1')
  })

  it('never renders a kebab menu or a delete (×) button', () => {
    render(<AgentChatRow {...base} />)
    expect(screen.queryByRole('button', { name: /more/i })).toBeNull()
    expect(screen.queryByRole('button', { name: /delete/i })).toBeNull()
    expect(screen.queryByText('×')).toBeNull()
    // Only the row itself is a button — no nested action buttons.
    expect(screen.getAllByRole('button')).toHaveLength(1)
  })
})
