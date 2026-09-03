/**
 * The Chats panel's search field.
 *
 * It is the FILE EXPLORER'S field — the shared `@/components/ui/input` at
 * `size="sm"` with a magnifier laid over it — rather than a second one spelled
 * out in border classes here, so the two fields one swipe apart in the same
 * sidebar cannot drift. The tests below pin the parts of that which are
 * behaviour: what the field is called, that it reports keystrokes, and that
 * Escape clears it without reaching anything above it.
 *
 * There is deliberately nothing under it. The count line that used to live there
 * is gone, and so is the `esc to clear` hint beside it.
 */
import { cleanup, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { AgentChatsSearch } from '@/features/agent/tree/agent-chats-search'

afterEach(cleanup)

const field = () => screen.getByRole('textbox', { name: /search chats/i })

describe('AgentChatsSearch', () => {
  it('reports each keystroke', async () => {
    const onChange = vi.fn()
    render(<AgentChatsSearch value="" onChange={onChange} />)
    await userEvent.type(field(), 'd')
    expect(onChange).toHaveBeenCalledWith('d')
  })

  it('renders the value it is given', () => {
    render(<AgentChatsSearch value="drag" onChange={vi.fn()} />)
    expect(field()).toHaveValue('drag')
  })

  it('is the shared Input control, not a hand-rolled field', () => {
    // The whole point of the rebuild: focus rings, autofill, the dark surface
    // and the disabled state are decided once, in @/components/ui/input, and
    // this panel does not get its own opinion about any of them.
    render(<AgentChatsSearch value="" onChange={vi.fn()} />)
    expect(field()).toHaveAttribute('data-slot', 'input')
    expect(field().closest('[data-slot="input-control"]')).not.toBeNull()
    expect(field().closest('[data-size="sm"]')).not.toBeNull()
  })

  it('sits in the shared SidebarHeader, like the file explorer’s field', () => {
    // Both fields are one swipe apart in the same sidebar. The header is where
    // the horizontal padding and the gap under the tab switcher are decided, so
    // a panel that spelled its own wrapper out drifted from the one beside it —
    // which is exactly what happened (6px under the switcher here against 14px
    // there).
    const { container } = render(<AgentChatsSearch value="" onChange={vi.fn()} />)
    const header = container.querySelector('[data-slot="sidebar-header"]')
    expect(header).not.toBeNull()
    expect(field().closest('[data-slot="sidebar-header"]')).toBe(header)
    // The same inner row the explorer uses, so the field measures the same way
    // with or without a button beside it.
    expect(header!.querySelector('.flex.items-stretch.gap-1\\.5')).not.toBeNull()
  })

  it('hides the magnifier from assistive tech — the field’s label names it', () => {
    // Phosphor emits no aria-hidden of its own, so an icon that is not told to
    // hide is announced as an unlabelled graphic.
    const { container } = render(<AgentChatsSearch value="" onChange={vi.fn()} />)
    expect(container.querySelector('svg')?.getAttribute('aria-hidden')).toBe('true')
  })

  it('points at the list it filters', () => {
    render(<AgentChatsSearch value="" onChange={vi.fn()} />)
    expect(field()).toHaveAttribute('aria-controls', 'chat-tree-results')
  })

  it('says what it searches', () => {
    render(<AgentChatsSearch value="" onChange={vi.fn()} />)
    expect(field()).toHaveAttribute('placeholder', 'Search chats')
  })

  it('draws nothing under the field', async () => {
    // The count line and its `esc to clear` hint were deleted: they cost a row
    // of height for the whole life of a query to advertise a shortcut the field
    // honours anyway.
    const { container } = render(<AgentChatsSearch value="" onChange={vi.fn()} />)
    expect(screen.queryByTestId('chat-search-meta')).toBeNull()
    expect(container.textContent).toBe('')

    await userEvent.type(field(), 'a')
    expect(container.textContent).toBe('')
  })

  it('offers no filter menu — this panel has no filters to open one on', () => {
    render(<AgentChatsSearch value="" onChange={vi.fn()} />)
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('escape clears the query', async () => {
    const onChange = vi.fn()
    render(<AgentChatsSearch value="drag" onChange={onChange} />)
    field().focus()
    await userEvent.keyboard('{Escape}')
    expect(onChange).toHaveBeenCalledWith('')
  })

  it('escape does not reach handlers above the field', async () => {
    // Inside a search box Escape means "clear"; letting it bubble would also
    // close whatever pane or dialog is listening for it.
    const onChange = vi.fn()
    const outer = vi.fn()
    render(
      <div onKeyDown={outer}>
        <AgentChatsSearch value="drag" onChange={onChange} />
      </div>,
    )
    field().focus()
    await userEvent.keyboard('{Escape}')
    expect(onChange).toHaveBeenCalledWith('')
    expect(outer).not.toHaveBeenCalled()
  })

  it('other keys are left alone', async () => {
    const onChange = vi.fn()
    const outer = vi.fn()
    render(
      <div onKeyDown={outer}>
        <AgentChatsSearch value="" onChange={onChange} />
      </div>,
    )
    field().focus()
    await userEvent.keyboard('{Enter}')
    expect(outer).toHaveBeenCalled()
  })
})
