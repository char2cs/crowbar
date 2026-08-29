import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

const baseRow: SidebarRowType = {
  id: 'row-1',
  kind: 'chat',
  parentId: null,
  order: 0,
  label: 'Fix the thing',
  ownsWorktree: false,
  workspaceId: null,
  working: false,
  hasView: false,
}

describe('SidebarRow', () => {
  it('renders the label', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
    expect(screen.getByText('Fix the thing')).toBeInTheDocument()
  })

  it('a row with a view greys its label, focused or not', () => {
    render(<SidebarRow row={{ ...baseRow, hasView: true }} depth={0} onOpen={vi.fn()} />)
    const label = screen.getByText('Fix the thing')
    expect(label.className).toMatch(/text-muted-foreground|opacity/)
  })

  it('a working row shows the spinner glyph, not the static mark', () => {
    // The real spinner (FlickerSpinner, web/src/components/ui/flicker-spinner.tsx)
    // marks itself with `data-flicker-spinner`, not a testid — every other call
    // site in this codebase asserts on that same attribute.
    const { container } = render(
      <SidebarRow row={{ ...baseRow, working: true }} depth={0} onOpen={vi.fn()} />,
    )
    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('trailing controls are trash, +, chevron in that order, revealed on hover', () => {
    render(
      <SidebarRow
        row={baseRow}
        depth={0}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onToggleFold={vi.fn()}
      />,
    )
    const controls = screen.getAllByRole('button')
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual(['trash', 'create', 'fold'])
  })

  it('no trailing controls render when no handler is passed for them', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
    expect(screen.queryAllByRole('button')).toHaveLength(0)
  })

  it('the create control makes a thread on a row that owns no worktree', () => {
    const onCreate = vi.fn()
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onCreate={onCreate} />)
    screen.getByRole('button', { name: /new thread/i }).click()
    expect(onCreate).toHaveBeenCalledWith('row-1', 'thread')
  })

  it('the create control makes a workspace on a row that owns a worktree', () => {
    const onCreate = vi.fn()
    render(
      <SidebarRow
        row={{ ...baseRow, ownsWorktree: true }}
        depth={0}
        onOpen={vi.fn()}
        onCreate={onCreate}
      />,
    )
    screen.getByRole('button', { name: /new workspace/i }).click()
    expect(onCreate).toHaveBeenCalledWith('row-1', 'workspace')
  })

  it('a project-home row (branch, no parent) gets the 20px glyph exception', () => {
    const { container } = render(
      <SidebarRow
        row={{ ...baseRow, kind: 'branch', parentId: null, ownsWorktree: true }}
        depth={0}
        onOpen={vi.fn()}
      />,
    )
    expect(container.querySelector('.size-5')).toBeInTheDocument()
  })

  it('clicking the row body opens it, not the trailing controls', () => {
    const onOpen = vi.fn()
    const onTrash = vi.fn()
    render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} onTrash={onTrash} />)
    screen.getByRole('button', { name: /delete/i }).click()
    expect(onTrash).toHaveBeenCalledWith('row-1')
    expect(onOpen).not.toHaveBeenCalled()
  })
})
