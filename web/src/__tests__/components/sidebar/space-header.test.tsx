import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SpaceHeader } from '@/components/sidebar/space-header'
import * as rowActions from '@/components/sidebar/lib/row-actions'
import type { Project } from '@/lib/types'

vi.mock('@/components/sidebar/lib/row-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof rowActions>()),
  performRenameProject: vi.fn().mockResolvedValue(undefined),
}))

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch,
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

  // Task 5 (icon personalization) turned the resting-state mark into a real
  // click target (EditableProjectIcon), which the OLD hover behaviour here
  // broke: hovering the row swapped the mark for the chevron before a click
  // could ever land on it, making the icon reachable in principle but
  // unclickable in practice — the exact conflict this codebase's own
  // history (project-home-row.tsx, deleted in the tree retirement, git
  // history cf422bc5) already hit and fixed by decoupling the fold-chevron
  // from hover entirely. Hover now only reveals the overflow control; the
  // mark stays put (and clickable) regardless of hover.
  it('on hover the overflow button appears; the mark stays, not a chevron', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onOverflow={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    expect(screen.queryByTestId('chevron')).not.toBeInTheDocument()
    expect(screen.getByTestId('overflow')).toBeInTheDocument()
  })

  it('mouse leave reverts the overflow button away again', () => {
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
    expect(screen.queryByTestId('overflow')).not.toBeInTheDocument()
  })

  it('the chevron shows only once folded, never merely from hover', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={true}
        onToggleFold={vi.fn()}
        onOverflow={vi.fn()}
      />,
    )
    expect(screen.getByTestId('chevron')).toBeInTheDocument()
    fireEvent.mouseLeave(screen.getByTestId('space-header-row'))
    expect(screen.getByTestId('chevron')).toBeInTheDocument()
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

  // Task 5 (icon personalization): the leading mark is EditableProjectIcon,
  // wired to the SAME icon-popover primitive the repo home row's own mark
  // uses (repo-icon-mark.tsx's EditableRepoIcon) — see that file's own test
  // suite for the parallel coverage.
  describe('click-to-edit icon', () => {
    beforeEach(() => {
      vi.clearAllMocks()
      apiFetch.mockResolvedValue(undefined)
    })

    it('clicking the mark opens the icon picker, not onToggleFold', async () => {
      const user = userEvent.setup()
      const onToggle = vi.fn()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={onToggle}
          onOverflow={vi.fn()}
        />,
      )
      await user.click(screen.getByRole('button', { name: /edit p1 icon/i }))
      expect(await screen.findByText('Icon')).toBeInTheDocument()
      expect(onToggle).not.toHaveBeenCalled()
    })

    it('setting an emoji persists it to this project’s own REST base', async () => {
      const user = userEvent.setup()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onOverflow={vi.fn()}
        />,
      )
      await user.click(screen.getByRole('button', { name: /edit p1 icon/i }))
      await user.click(await screen.findByRole('button', { name: /emoji/i }))
      await user.type(screen.getByPlaceholderText('Type an emoji…'), '🛰️')
      await user.keyboard('{Enter}')
      expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p1/icon/emoji', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emoji: '🛰️' }),
      })
    })
  })
})
