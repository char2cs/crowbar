import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SpaceHeader } from '@/components/sidebar/space-header'
import type { Project } from '@/lib/types'

function makeProject(id: string): Project {
  return {
    id,
    name: id,
    path: `/repos/${id}`,
    lastActivity: new Date('2026-08-28T00:00:00Z'),
  }
}

describe('SpaceHeader', () => {
  it('at rest shows the project mark and name, no controls', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onOverflow={vi.fn()}
      />,
    )
    expect(screen.queryByTestId('chevron')).not.toBeInTheDocument()
    expect(screen.queryByTestId('overflow')).not.toBeInTheDocument()
    expect(screen.getByText('p1')).toBeInTheDocument()
  })

  it('on hover the mark slot becomes a chevron, overflow appears', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onOverflow={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    expect(screen.getByTestId('chevron')).toBeInTheDocument()
    expect(screen.getByTestId('overflow')).toBeInTheDocument()
  })

  it('mouse leave reverts the chevron and overflow away again', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onOverflow={vi.fn()}
      />,
    )
    const row = screen.getByTestId('space-header-row')
    fireEvent.mouseEnter(row)
    fireEvent.mouseLeave(row)
    expect(screen.queryByTestId('chevron')).not.toBeInTheDocument()
    expect(screen.queryByTestId('overflow')).not.toBeInTheDocument()
  })

  it('clicking folds: chevron stays, rotated', () => {
    const onToggle = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={true}
        onToggleFold={onToggle}
        onOverflow={vi.fn()}
      />,
    )
    expect(screen.getByTestId('chevron')).toHaveClass('rotate-180')
  })

  it('clicking the header calls onToggleFold', () => {
    const onToggle = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onOverflow={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByTestId('space-header-row'))
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  it('clicking overflow calls onOverflow, not onToggleFold', () => {
    const onToggle = vi.fn()
    const onOverflow = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onOverflow={onOverflow}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    fireEvent.click(screen.getByTestId('overflow'))
    expect(onOverflow).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('keyboard-activating the overflow button fires onOverflow, not onToggleFold', async () => {
    // Regression: a keydown on the nested overflow button bubbles to the row's
    // own onKeyDown. Without SidebarRow's `e.target !== e.currentTarget`
    // guard, Enter/Space on the button fired onToggleFold instead of the
    // button's own click. fireEvent.keyDown does not exercise this — jsdom
    // does not synthesize a button's default click-on-Enter/Space action from
    // a raw keydown event — so this uses userEvent, which does.
    const onToggle = vi.fn()
    const onOverflow = vi.fn()
    const user = userEvent.setup()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onOverflow={onOverflow}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    screen.getByTestId('overflow').focus()
    await user.keyboard('{Enter}')
    expect(onOverflow).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('no background at rest, the same --accent hover every other row takes', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onOverflow={vi.fn()}
      />,
    )
    const row = screen.getByTestId('space-header-row')
    expect(row.className).toMatch(/border-transparent/)
    expect(row.className).toMatch(/hover:bg-accent/)
  })
})
