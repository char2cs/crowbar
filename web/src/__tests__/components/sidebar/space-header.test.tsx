import { afterEach, describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SpaceHeader } from '@/components/sidebar/space-header'
import * as rowActions from '@/components/sidebar/lib/row-actions'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
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
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    // The chevron stays mounted (icon-popover.tsx's own popover state lives
    // inside the icon it would otherwise unmount — see its own doc), so this
    // checks it is hidden rather than absent.
    expect(screen.getByTestId('chevron').parentElement?.className).toContain('hidden')
    expect(screen.queryByTestId('new-thread')).not.toBeInTheDocument()
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
  it('on hover (off the glyph) the chevron, thread button and overflow menu all appear', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    expect(screen.getByTestId('chevron')).toBeInTheDocument()
    expect(screen.getByTestId('new-thread')).toBeInTheDocument()
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
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    const row = screen.getByTestId('space-header-row')
    fireEvent.mouseEnter(row)
    fireEvent.mouseEnter(screen.getByTestId('space-glyph'))
    expect(screen.getByTestId('chevron').parentElement?.className).toContain('hidden')
    expect(screen.getByRole('button', { name: /edit p1 icon/i })).toBeInTheDocument()
    // Leaving the glyph for elsewhere on the row (still over the row overall,
    // per the `relatedTarget`) reverts it to the chevron.
    fireEvent.mouseLeave(screen.getByTestId('space-glyph'), { relatedTarget: row })
    expect(screen.getByTestId('chevron').parentElement?.className).not.toContain('hidden')
  })

  it('mouse leave reverts the thread button and overflow menu away again', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    const row = screen.getByTestId('space-header-row')
    fireEvent.mouseEnter(row)
    fireEvent.mouseLeave(row)
    expect(screen.queryByTestId('new-thread')).not.toBeInTheDocument()
    expect(screen.queryByTestId('delete-menu')).not.toBeInTheDocument()
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
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
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
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    expect(screen.getByTestId('chevron')).toHaveClass('rotate-180')
  })

  // Caught live: the fold button sized to its own text line-height inside
  // the h-9 row, leaving an 8px dead band top and bottom that shared the
  // row's hover ground but had no click handler — clicking near the row's
  // own border, still visibly inside it, silently did nothing.
  // `getBoundingClientRect` is meaningless in jsdom (no real layout), so
  // this pins the actual mechanism instead: the button must stretch across
  // the row's full cross-axis and re-center its own text, not merely occupy
  // whatever the row's default `align-items: center` would give a
  // block-level child.
  it('fold button stretches to the row’s full height, not just its own text', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    const foldButton = screen.getByRole('button', { name: 'Collapse p1' })
    expect(foldButton).toHaveClass('self-stretch')
    expect(foldButton).toHaveClass('items-center')
  })

  // The row's own container (`space-header-row`) is a plain, non-interactive
  // div now — no `role`, no `onClick` of its own — precisely so it cannot
  // present as one giant button swallowing the overflow/thread controls'
  // independent semantics (`html-no-nested-interactive`). The fold toggle is
  // a real, independently-focusable `<button>` (the label) instead, so this
  // asserts THAT control does the job, not a bare click on the container.
  it('clicking the header (its label button) calls onToggleFold', () => {
    const onToggle = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onCreateThread={vi.fn()}
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Collapse p1' }))
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  // The container itself must NOT carry its own click behavior — that would
  // be exactly the reintroduced bug (a clickable ancestor wrapping the
  // overflow/thread buttons). A bare click on the row's own box, missing
  // every actual control, does nothing.
  it('a bare click on the row container (missing every real control) does not toggle', () => {
    const onToggle = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onCreateThread={vi.fn()}
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByTestId('space-header-row'))
    expect(onToggle).not.toHaveBeenCalled()
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
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    fireEvent.click(screen.getByTestId('new-thread'))
    expect(onCreateThread).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  // The "add repository" plus button used to live as its own row control;
  // "Import a repo" and "Create a folder" now live as items inside the SAME
  // overflow menu Delete Space already opens (spec: fewer controls fighting
  // for the row's own hover cluster).
  it('clicking Import a repo in the overflow calls onImportRepo, not onToggleFold', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    const onImportRepo = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onCreateThread={vi.fn()}
        onImportRepo={onImportRepo}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    await user.click(screen.getByTestId('delete-menu'))
    await user.click(await screen.findByText('Import a repo'))

    expect(onImportRepo).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('clicking Create a folder in the overflow calls onCreateFolder, not onToggleFold', async () => {
    const user = userEvent.setup()
    const onToggle = vi.fn()
    const onCreateFolder = vi.fn()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onCreateThread={vi.fn()}
        onImportRepo={vi.fn()}
        onCreateFolder={onCreateFolder}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    await user.click(screen.getByTestId('delete-menu'))
    await user.click(await screen.findByText('Create a folder'))

    expect(onCreateFolder).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  // Spec §9's "the space header for the project" clause: the FIRST item in
  // the trailing cluster (the row's own fold toggle is its leading glyph, so
  // nothing else here competes for "last"), a plain overflow opening onto
  // Import a repo / Create a folder / Delete Space — the last of which calls
  // the already-threaded `onTrashProject` pipe (space-scroller.tsx), not
  // `onToggleFold`.
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
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={onDeleteSpace}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    await user.click(screen.getByTestId('delete-menu'))
    await user.click(await screen.findByText('Delete Space'))

    expect(onDeleteSpace).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  // THE BUG: "can't start chats directly on a CLI, it always obligates me to
  // use the native chat" — the header's own Thread button, like the row's,
  // had no way to land the new chat on Terminal without flipping
  // chatIsDefaultPresentation (Settings → Chat) globally. This is the
  // discoverable, visible affordance for it: a second item in the SAME
  // overflow menu Import a repo / Create a folder already live in.
  describe('New thread in Terminal (overflow menu)', () => {
    afterEach(() => {
      useAgentProvidersStore.setState({ status: 'idle', providers: [] })
    })

    it('clicking it calls onCreateThreadTerminal, not onToggleFold or plain onCreateThread', async () => {
      useAgentProvidersStore.setState({
        status: 'ready',
        providers: [
          { id: 'claude', enabled: true, hasTerminal: true, terminalStartHere: true },
        ] as never,
      })
      const user = userEvent.setup()
      const onToggle = vi.fn()
      const onCreateThread = vi.fn()
      const onCreateThreadTerminal = vi.fn()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={onToggle}
          onCreateThread={onCreateThread}
          onCreateThreadTerminal={onCreateThreadTerminal}
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
      await user.click(screen.getByTestId('delete-menu'))
      await user.click(await screen.findByText('New thread in Terminal'))

      expect(onCreateThreadTerminal).toHaveBeenCalledTimes(1)
      expect(onCreateThread).not.toHaveBeenCalled()
      expect(onToggle).not.toHaveBeenCalled()
    })

    // House rule: absence, not a disabled control.
    it('is ABSENT, not disabled, when the enabled provider declares no terminal', async () => {
      useAgentProvidersStore.setState({
        status: 'ready',
        providers: [{ id: 'claude', enabled: true, hasTerminal: false }] as never,
      })
      const user = userEvent.setup()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onCreateThreadTerminal={vi.fn()}
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
      await user.click(screen.getByTestId('delete-menu'))

      expect(await screen.findByText('Import a repo')).toBeInTheDocument()
      expect(screen.queryByText('New thread in Terminal')).not.toBeInTheDocument()
    })

    // THE surfaces: fix (design spec 2.5): hasTerminal alone used to gate
    // this. codex HAS a terminal (attach) that is only reachable by
    // switching to it after a turn — never a launch target for a chat that
    // does not exist yet.
    it('is ABSENT when the enabled provider has a terminal that is not a start_here surface', async () => {
      useAgentProvidersStore.setState({
        status: 'ready',
        providers: [{ id: 'codex', enabled: true, hasTerminal: true }] as never,
      })
      const user = userEvent.setup()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onCreateThreadTerminal={vi.fn()}
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
      await user.click(screen.getByTestId('delete-menu'))

      expect(await screen.findByText('Import a repo')).toBeInTheDocument()
      expect(screen.queryByText('New thread in Terminal')).not.toBeInTheDocument()
    })

    // No enabled provider resolved yet is not evidence of "no terminal" —
    // the plain Thread button offers itself unconditionally too and leaves
    // the refusal to click-time resolution; this matches it.
    it('is still offered when no provider is enabled yet', async () => {
      useAgentProvidersStore.setState({ status: 'idle', providers: [] })
      const user = userEvent.setup()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onCreateThreadTerminal={vi.fn()}
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
      await user.click(screen.getByTestId('delete-menu'))

      expect(await screen.findByText('New thread in Terminal')).toBeInTheDocument()
    })
  })

  it('keyboard-activating the thread button fires its own handler, not onToggleFold', async () => {
    // Regression: a keydown on a nested button bubbles to the row's own
    // onKeyDown. Without SidebarRow's `e.target !== e.currentTarget` guard,
    // Enter/Space on the button fired onToggleFold instead of the button's
    // own click. fireEvent.keyDown does not exercise this — jsdom does not
    // synthesize a button's default click-on-Enter/Space action from a raw
    // keydown event — so this uses userEvent, which does.
    const onToggle = vi.fn()
    const onCreateThread = vi.fn()
    const user = userEvent.setup()
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={onToggle}
        onCreateThread={onCreateThread}
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    screen.getByTestId('new-thread').focus()
    await user.keyboard('{Enter}')
    expect(onCreateThread).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('no background at rest, the same idle-ground hover every other row takes', () => {
    render(
      <SpaceHeader
        project={makeProject('p1')}
        folded={false}
        onToggleFold={vi.fn()}
        onCreateThread={vi.fn()}
        onImportRepo={vi.fn()}
        onCreateFolder={vi.fn()}
        onDeleteSpace={vi.fn()}
      />,
    )
    const row = screen.getByTestId('space-header-row')
    expect(row.className).toMatch(/border-transparent/)
    expect(row.className).toMatch(/hover:bg-sidebar-element-idle/)
  })

  // Double-click-to-rename the project itself — restored from the deleted
  // tree's project-home-row.tsx, which called the same `renameProject` API
  // this reaches through the new `performRenameProject` wrapper. A project
  // has no id in the row-based `SidebarRow[]`/`performRenameRow` space (a
  // project is not a row at all), so this is a second, LOCAL rename target,
  // not a duplicate of the row rename path.
  //
  // Task 11: this is a REAL inline `<input>` replacing the name in place —
  // develop's actual behavior — not a modal dialog.
  describe('double-click-to-rename', () => {
    it('double-clicking the project name replaces it with a focused input, not a dialog', () => {
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
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
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
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
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
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
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
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
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
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
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
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
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      await user.click(screen.getByRole('button', { name: /edit p1 icon/i }))
      expect(await screen.findByText('Icon')).toBeInTheDocument()
      expect(onToggle).not.toHaveBeenCalled()
    })

    // Live-reported: "moving my mouse inside this icon modal, its not
    // letting me" — the popover was closing itself. Opening it left the row
    // hovered (mouse still down over the mark that was just clicked); moving
    // toward the popover's own portaled content crosses the glyph's own
    // narrow hit-target first, which used to swap `EditableProjectIcon` OUT
    // for the chevron (`showChevron = active && !glyphHovered`) — unmounting
    // the very popover the user was reaching for, mid-transit, before the
    // pointer ever got there.
    it('stays open when the mouse leaves the glyph for elsewhere on the still-hovered row', async () => {
      const user = userEvent.setup()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      const row = screen.getByTestId('space-header-row')
      fireEvent.mouseEnter(row)
      fireEvent.mouseEnter(screen.getByTestId('space-glyph'))
      await user.click(screen.getByRole('button', { name: /edit p1 icon/i }))
      expect(await screen.findByText('Icon')).toBeInTheDocument()

      // The mouse's own path toward the popover: off the glyph, still over
      // the row (the popover sits just outside it, but the row itself is
      // wide) — exactly the transit that used to unmount it, and later
      // (once that was fixed) flash the mark between icon and chevron on
      // every such crossing. The mark now holds the icon for as long as the
      // popover itself reports open, regardless of hover.
      fireEvent.mouseLeave(screen.getByTestId('space-glyph'), { relatedTarget: row })
      expect(screen.getByTestId('chevron').parentElement?.className).toContain('hidden')
      expect(screen.getByText('Icon')).toBeInTheDocument()

      // Re-entering the glyph and leaving again must not re-flash it either
      // — still gated on the popover's own open state, not just hover.
      fireEvent.mouseEnter(screen.getByTestId('space-glyph'))
      fireEvent.mouseLeave(screen.getByTestId('space-glyph'), { relatedTarget: row })
      expect(screen.getByTestId('chevron').parentElement?.className).toContain('hidden')
      expect(screen.getByText('Icon')).toBeInTheDocument()
    })

    // The other half of the same fix: the gate must release once the
    // popover reports closed, not hold the icon forever. `onOpenChange`
    // fires straight off IconPopover's own Popover, so closing it via its
    // own imperative action (rather than simulating an outside-press, which
    // Base UI's dismiss logic does not reliably resolve under fireEvent) is
    // the direct way to prove the release side of the same wiring.
    it('the chevron gate releases once the popover reports closed', async () => {
      const user = userEvent.setup()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
          onDeleteSpace={vi.fn()}
        />,
      )
      const row = screen.getByTestId('space-header-row')
      fireEvent.mouseEnter(row)
      await user.click(screen.getByRole('button', { name: /edit p1 icon/i }))
      expect(await screen.findByText('Icon')).toBeInTheDocument()
      fireEvent.mouseLeave(screen.getByTestId('space-glyph'), { relatedTarget: row })
      expect(screen.getByTestId('chevron').parentElement?.className).toContain('hidden')

      await user.keyboard('{Escape}')
      await waitFor(() => expect(screen.queryByText('Icon')).not.toBeInTheDocument())
      expect(screen.getByTestId('chevron').parentElement?.className).not.toContain('hidden')
    })

    it('setting an emoji persists it to this project’s own REST base', async () => {
      const user = userEvent.setup()
      render(
        <SpaceHeader
          project={makeProject('p1')}
          folded={false}
          onToggleFold={vi.fn()}
          onCreateThread={vi.fn()}
          onImportRepo={vi.fn()}
          onCreateFolder={vi.fn()}
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
