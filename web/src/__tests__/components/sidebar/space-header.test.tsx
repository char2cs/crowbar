import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SpaceHeader } from '@/components/sidebar/space-header'
import * as rowActions from '@/components/sidebar/lib/row-actions'
import type { Project } from '@/lib/types'

vi.mock('@/components/sidebar/lib/row-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof rowActions>()),
  performRenameProject: vi.fn().mockResolvedValue(undefined),
}))

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

  // Double-click-to-rename the project itself — restored from the deleted
  // tree's project-home-row.tsx, which called the same `renameProject` API
  // this reaches through the new `performRenameProject` wrapper. A project
  // has no id in the row-based `SidebarRow[]`/`performRenameRow` space (a
  // project is not a row at all), so this is a second, LOCAL rename target,
  // not a duplicate of the row rename path — it reuses the same shared
  // `RenameDialog` component, per that component's own doc: "the sidebar's
  // one rename gesture."
  describe('double-click-to-rename', () => {
    it('double-clicking the project name opens the rename dialog prefilled with it', () => {
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onOverflow={vi.fn()}
        />,
      )
      fireEvent.doubleClick(screen.getByText('p1'))
      expect(screen.getByRole('textbox')).toHaveValue('p1')
    })

    it('confirming the dialog calls performRenameProject with the project id and new name', () => {
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onOverflow={vi.fn()}
        />,
      )
      fireEvent.doubleClick(screen.getByText('p1'))
      fireEvent.change(screen.getByRole('textbox'), { target: { value: 'New Name' } })
      fireEvent.click(screen.getByRole('button', { name: /rename/i }))
      expect(rowActions.performRenameProject).toHaveBeenCalledWith('p1', 'New Name')
    })

    it('a single click on the name still folds the space, not just double-click', () => {
      const onToggle = vi.fn()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={onToggle}
          onOverflow={vi.fn()}
        />,
      )
      fireEvent.click(screen.getByText('p1'))
      expect(onToggle).toHaveBeenCalledTimes(1)
    })
  })
})
