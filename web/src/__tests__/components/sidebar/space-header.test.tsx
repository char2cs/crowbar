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

beforeEach(() => {
  vi.clearAllMocks()
})

describe('SpaceHeader', () => {
  it('at rest shows the project mark and name, no controls', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    expect(screen.queryByTestId('chevron')).not.toBeInTheDocument()
    expect(screen.queryByTestId('new-thread')).not.toBeInTheDocument()
    expect(screen.queryByTestId('add-menu')).not.toBeInTheDocument()
    expect(screen.queryByTestId('delete-menu')).not.toBeInTheDocument()
    expect(screen.getByText('p1')).toBeInTheDocument()
  })

  // Spec §4: "On hover — the mark's slot becomes a chevron, and an overflow
  // (…) appears." Task 5 (icon personalization) turned the resting-state
  // mark into a real click target (EditableProjectIcon) too, which an
  // EARLIER attempt at this hover swap broke: hovering the row swapped the
  // mark for the chevron before a click could ever land on it, making the
  // icon reachable in principle but unclickable in practice. The fix is not
  // to drop the swap (that traded a real bug for a spec violation) but to
  // scope it: hovering the ROW swaps the mark for the chevron, per spec;
  // hovering the mark's OWN hit-target does not (see the next test).
  it('on hover (off the glyph) the chevron, thread button and add-menu button all appear', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    expect(screen.getByTestId('chevron')).toBeInTheDocument()
    expect(screen.getByTestId('new-thread')).toBeInTheDocument()
    expect(screen.getByTestId('add-menu')).toBeInTheDocument()
    expect(screen.getByTestId('delete-menu')).toBeInTheDocument()
  })

  // The narrower half of the fix above: a pointer sitting exactly on the
  // glyph's own hit-target keeps it as the icon, so Task 5's click-to-edit
  // affordance (icon-popover.tsx's hover-reveals-pencil) stays reachable
  // even while the row around it is hovered.
  it('hovering the glyph itself keeps the icon, not the chevron, even while the row is hovered', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    const row = screen.getByTestId('space-header-row')
    fireEvent.mouseEnter(row)
    fireEvent.mouseEnter(screen.getByTestId('space-glyph'))
    expect(screen.queryByTestId('chevron')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /edit p1 icon/i })).toBeInTheDocument()
    // Leaving the glyph for elsewhere on the row (still over the row overall,
    // per the `relatedTarget`) reverts it to the chevron.
    fireEvent.mouseLeave(screen.getByTestId('space-glyph'), { relatedTarget: row })
    expect(screen.getByTestId('chevron')).toBeInTheDocument()
  })

  it('mouse leave reverts the thread and add-menu buttons away again', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    const row = screen.getByTestId('space-header-row')
    fireEvent.mouseEnter(row)
    fireEvent.mouseLeave(row)
    expect(screen.queryByTestId('new-thread')).not.toBeInTheDocument()
    expect(screen.queryByTestId('add-menu')).not.toBeInTheDocument()
  })

  // Folded reports a state (spec §4), so it does not depend on hover at
  // all — the chevron stays whether or not the pointer is over the row,
  // unlike the unfolded case above, where hover is what puts it there.
  it('the chevron stays once folded regardless of hover', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={true}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={vi.fn()}
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
        onCreateThread={vi.fn()}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={vi.fn()}
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
        onCreateThread={vi.fn()}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByTestId('space-header-row'))
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  it('clicking the thread button calls onCreateThread, not onToggleFold', () => {
    const onToggle = vi.fn()
    const onCreateThread = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onCreateThread={onCreateThread}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    fireEvent.click(screen.getByTestId('new-thread'))
    expect(onCreateThread).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('clicking the add-menu button calls onOpenAddMenu, not onToggleFold', () => {
    const onToggle = vi.fn()
    const onOpenAddMenu = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onCreateThread={vi.fn()}
        onOpenAddMenu={onOpenAddMenu}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    fireEvent.click(screen.getByTestId('add-menu'))
    expect(onOpenAddMenu).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  // Spec §9's "the space header for the project" clause: the FIRST item in
  // the trailing cluster (the row's own fold toggle is its leading glyph, so
  // nothing else here competes for "last"), a plain overflow — no anchored
  // position math, unlike the Add menu — opening straight onto "Delete
  // Space", which calls the already-threaded `onTrashProject` pipe
  // (space-scroller.tsx), not `onToggleFold`.
  it('clicking Delete Space in the overflow calls onDeleteSpace, not onToggleFold', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    const onDeleteSpace = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onCreateThread={vi.fn()}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={onDeleteSpace}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    await user.click(screen.getByTestId('delete-menu'))
    await user.click(await screen.findByText('Delete Space'))

    expect(onDeleteSpace).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('keyboard-activating the thread or add-menu button fires its own handler, not onToggleFold', async () => {
    // Regression: a keydown on a nested button bubbles to the row's own
    // onKeyDown. Without SidebarRow's `e.target !== e.currentTarget` guard,
    // Enter/Space on the button fired onToggleFold instead of the button's
    // own click. fireEvent.keyDown does not exercise this — jsdom does not
    // synthesize a button's default click-on-Enter/Space action from a raw
    // keydown event — so this uses userEvent, which does.
    const onToggle = vi.fn()
    const onCreateThread = vi.fn()
    const onOpenAddMenu = vi.fn()
    const user = userEvent.setup()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onCreateThread={onCreateThread}
        onOpenAddMenu={onOpenAddMenu}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    screen.getByTestId('new-thread').focus()
    await user.keyboard('{Enter}')
    expect(onCreateThread).toHaveBeenCalledTimes(1)

    screen.getByTestId('add-menu').focus()
    await user.keyboard('{Enter}')
    expect(onOpenAddMenu).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('no background at rest, the same --accent hover every other row takes', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onOpenAddMenu={vi.fn()}
        onDeleteSpace={vi.fn()}
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
  // not a duplicate of the row rename path.
  //
  // Task 11: this is a REAL inline `<input>` replacing the name in place —
  // develop's actual behavior — not the modal `RenameDialog` Task 4 wrongly
  // built. `RenameDialog` itself is untouched; this row just no longer opens
  // it on double-click.
  describe('double-click-to-rename', () => {
    it('double-clicking the project name replaces it with a focused input, not a dialog', () => {
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onOpenAddMenu={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.doubleClick(screen.getByText('p1'))
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveValue('p1')
      expect(input).toHaveFocus()
    })

    it('Enter confirms and calls performRenameProject with the project id and new name', () => {
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onOpenAddMenu={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.doubleClick(screen.getByText('p1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'New Name' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(rowActions.performRenameProject).toHaveBeenCalledWith('p1', 'New Name')
      // No optimistic write (performRenameRow's own documented pattern): the
      // label only actually updates once the renamed DTO arrives over the
      // projects WS stream and the parent re-supplies a new `project` prop —
      // this component just closes the editor and falls back to whatever it
      // was handed, unchanged here since the mocked action does nothing.
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
      expect(screen.getByText('p1')).toBeInTheDocument()
    })

    it('Escape cancels with no call, restoring the plain label', () => {
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onOpenAddMenu={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.doubleClick(screen.getByText('p1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'New Name' } })
      fireEvent.keyDown(input, { key: 'Escape' })
      expect(rowActions.performRenameProject).not.toHaveBeenCalled()
      expect(screen.getByText('p1')).toBeInTheDocument()
    })

    it('blur without Enter/Escape commits the rename, matching develop', () => {
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onOpenAddMenu={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.doubleClick(screen.getByText('p1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'Blurred Name' } })
      fireEvent.blur(input)
      expect(rowActions.performRenameProject).toHaveBeenCalledWith('p1', 'Blurred Name')
    })

    it('a single click on the name still folds the space, not just double-click', () => {
      const onToggle = vi.fn()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={onToggle}
          onCreateThread={vi.fn()}
          onOpenAddMenu={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.click(screen.getByText('p1'))
      expect(onToggle).toHaveBeenCalledTimes(1)
    })

    it('clicking inside the input while renaming does not toggle the fold', () => {
      const onToggle = vi.fn()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={onToggle}
          onCreateThread={vi.fn()}
          onOpenAddMenu={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.doubleClick(screen.getByText('p1'))
      fireEvent.click(screen.getByRole('textbox'))
      expect(onToggle).not.toHaveBeenCalled()
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
          onCreateThread={vi.fn()}
          onOpenAddMenu={vi.fn()}
          onDeleteSpace={vi.fn()}
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
          onCreateThread={vi.fn()}
          onOpenAddMenu={vi.fn()}
          onDeleteSpace={vi.fn()}
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
