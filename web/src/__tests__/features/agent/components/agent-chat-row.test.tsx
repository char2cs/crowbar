import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentChatRow } from '@/features/agent/components/agent-chat-row'

const base = {
  chatId: 'c1',
  title: 'My chat',
  providerIcon: '<svg data-icon="claude"/>',
  working: false,
  active: false,
  renaming: false,
  onSelect: vi.fn(),
  onStartRename: vi.fn(),
  onConfirmRename: vi.fn(),
  onCancelRename: vi.fn(),
  onPointerDownDrag: vi.fn(),
}

describe('AgentChatRow', () => {
  it('renders the provider icon when idle and the spinner when working', () => {
    const { rerender, container } = render(<AgentChatRow {...base} />)
    expect(container.querySelector('[data-icon="claude"]')).not.toBeNull()
    expect(screen.queryByRole('status')).toBeNull()

    rerender(<AgentChatRow {...base} working />)
    expect(screen.getByRole('status')).toBeTruthy()
    expect(container.querySelector('[data-icon="claude"]')).toBeNull()
  })

  it('single-click selects, double-click starts rename', () => {
    const onSelect = vi.fn()
    const onStartRename = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} onStartRename={onStartRename} />)
    const row = screen.getByText('My chat')
    fireEvent.click(row)
    expect(onSelect).toHaveBeenCalledTimes(1)
    fireEvent.doubleClick(row)
    expect(onStartRename).toHaveBeenCalledTimes(1)
  })

  it('renaming renders the inline input seeded with the title', () => {
    render(<AgentChatRow {...base} renaming />)
    expect(screen.getByDisplayValue('My chat')).toBeTruthy()
    expect(screen.queryByText('My chat')).toBeNull()
  })

  it('confirming the inline input calls onConfirmRename with the new title', () => {
    const onConfirmRename = vi.fn()
    render(<AgentChatRow {...base} renaming onConfirmRename={onConfirmRename} />)
    const input = screen.getByDisplayValue('My chat')
    fireEvent.change(input, { target: { value: 'Renamed chat' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirmRename).toHaveBeenCalledWith('Renamed chat')
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

  it('applies the active row class when active, inactive class otherwise', () => {
    const { rerender } = render(<AgentChatRow {...base} active={false} />)
    let row = screen.getByRole('button')
    expect(row.className).toContain('hover:bg-accent')

    rerender(<AgentChatRow {...base} active />)
    row = screen.getByRole('button')
    expect(row.className).toContain('bg-background')
  })

  it('exposes a drag drop-target attribute and forwards pointer-down to the drag handler', () => {
    const onPointerDownDrag = vi.fn()
    render(<AgentChatRow {...base} onPointerDownDrag={onPointerDownDrag} />)
    const row = screen.getByRole('button')
    expect(row.getAttribute('data-agent-chat-drop')).toBe('c1')
    fireEvent.pointerDown(row)
    expect(onPointerDownDrag).toHaveBeenCalledTimes(1)
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
