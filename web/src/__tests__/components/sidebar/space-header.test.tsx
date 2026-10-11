import type { ComponentProps } from 'react'
import { afterEach, describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SpaceHeader } from '@/components/sidebar/space-header'
import * as rowActions from '@/components/sidebar/lib/row-actions'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import type { Project } from '@/lib/types'
import { loadProjects, pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

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
    hostId: 'local',
  }
}

function renderHeader(props: Partial<ComponentProps<typeof SpaceHeader>> = {}) {
  return render(
    <SpaceHeader
      project={makeProject('p1')}
      folded={false}
      onToggleFold={vi.fn()}
      onCreateThread={vi.fn()}
      onImportRepo={vi.fn()}
      onCreateFolder={vi.fn()}
      onDeleteSpace={vi.fn()}
      {...props}
    />,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

afterEach(() => resetHostWorld())

describe('SpaceHeader', () => {
  it('at rest shows the project mark and name, no controls', () => {
    renderHeader()
    // The chevron stays mounted (icon-popover.tsx's own popover state lives
    // inside the icon it would otherwise unmount — see its own doc), so this
    // checks it is hidden rather than absent.
    expect(screen.getByTestId('chevron').parentElement?.className).toContain('hidden')
    expect(screen.queryByTestId('new-thread')).not.toBeInTheDocument()
    expect(screen.queryByTestId('delete-menu')).not.toBeInTheDocument()
    expect(screen.getByText('p1')).toBeInTheDocument()
  })

  // Spec §4: hovering the ROW swaps the mark for a chevron and shows the overflow; hovering the
  // mark's own hit-target does not (next test), or its click-to-edit could never be reached.
  it('on hover (off the glyph) the chevron, thread button and overflow menu all appear', () => {
    renderHeader()
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
    renderHeader()
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
    renderHeader()
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
    renderHeader({ folded: true })
    expect(screen.getByTestId('chevron')).toBeInTheDocument()
    fireEvent.mouseLeave(screen.getByTestId('space-header-row'))
    expect(screen.getByTestId('chevron')).toBeInTheDocument()
  })

  it('clicking folds: chevron stays, rotated', () => {
    const onToggle = vi.fn()
    renderHeader({ folded: true, onToggleFold: onToggle })
    expect(screen.getByTestId('chevron')).toHaveClass('rotate-180')
  })

  // The whole row's height must fold, not just the text's own line box. jsdom runs no layout,
  // so this pins the mechanism: the button stretches across the row and re-centres its text.
  it('fold button stretches to the row’s full height, not just its own text', () => {
    renderHeader()
    const foldButton = screen.getByRole('button', { name: 'Collapse p1' })
    expect(foldButton).toHaveClass('self-stretch')
    expect(foldButton).toHaveClass('items-center')
  })

  // The container is inert (it would swallow the nested controls); the label's own button folds.
  it('clicking the header (its label button) calls onToggleFold', () => {
    const onToggle = vi.fn()
    renderHeader({ onToggleFold: onToggle })
    fireEvent.click(screen.getByRole('button', { name: 'Collapse p1' }))
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  // The host line is part of the fold control, which it describes: clicking anywhere on the label
  // folds, and a screen reader hears which host (and its state) the project is on.
  it('shows the project’s host under its name, dotless, inside the fold control it describes', () => {
    const onToggle = vi.fn()
    renderHeader({ onToggleFold: onToggle })
    const fold = screen.getByRole('button', { name: 'Collapse p1' })

    expect(fold).toContainElement(screen.getByText('p1'))
    expect(fold).toContainElement(screen.getByText('Local'))
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    expect(fold).toHaveAccessibleDescription('Local, Status unknown')
    fireEvent.click(screen.getByText('Local'))
    expect(onToggle).toHaveBeenCalledTimes(1)
  })

  // Local is the app itself: only a remote project has a host to reach settings for.
  it('a local project has no host button', () => {
    renderHeader()
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    expect(screen.queryByRole('button', { name: /^Host settings/ })).not.toBeInTheDocument()
  })

  it('a remote project’s host button is shown at rest, and last in the row’s tab order', async () => {
    const project = { ...makeProject('p1'), hostId: 'h1' }
    pairRemoteHost('h1', 'homelab-prox')
    loadProjects(project)
    const user = userEvent.setup()
    renderHeader({ project })
    const host = screen.getByRole('button', { name: /^Host settings: homelab-prox, / })

    for (const name of [
      /edit p1 icon/i,
      'Collapse p1',
      'More actions for p1',
      'New thread on p1',
    ]) {
      await user.tab()
      expect(screen.getByRole('button', { name })).toHaveFocus()
    }
    await user.tab()
    expect(host).toHaveFocus()
  })

  // The container itself must NOT carry its own click behavior — that would
  // be exactly the reintroduced bug (a clickable ancestor wrapping the
  // overflow/thread buttons). A bare click on the row's own box, missing
  // every actual control, does nothing.
  it('a bare click on the row container (missing every real control) does not toggle', () => {
    const onToggle = vi.fn()
    renderHeader({ onToggleFold: onToggle })
    fireEvent.click(screen.getByTestId('space-header-row'))
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('clicking the thread button calls onCreateThread, not onToggleFold', () => {
    const onToggle = vi.fn()
    const onCreateThread = vi.fn()
    renderHeader({ onToggleFold: onToggle, onCreateThread })
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
    renderHeader({ onToggleFold: onToggle, onImportRepo })
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
    renderHeader({ onToggleFold: onToggle, onCreateFolder })
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
    renderHeader({ onToggleFold: onToggle, onDeleteSpace })
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    await user.click(screen.getByTestId('delete-menu'))
    await user.click(await screen.findByText('Delete Space'))

    expect(onDeleteSpace).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  // Starting ONE chat on the CLI without flipping Settings → Chat for every chat after it.
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
      renderHeader({ onToggleFold: onToggle, onCreateThread, onCreateThreadTerminal })
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
      renderHeader({ onCreateThreadTerminal: vi.fn() })
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
      renderHeader({ onCreateThreadTerminal: vi.fn() })
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
      renderHeader({ onCreateThreadTerminal: vi.fn() })
      fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
      await user.click(screen.getByTestId('delete-menu'))

      expect(await screen.findByText('New thread in Terminal')).toBeInTheDocument()
    })
  })

  it('keyboard-activating the thread button fires its own handler, not onToggleFold', async () => {
    // userEvent, not fireEvent.keyDown: only it synthesizes a button's click from Enter.
    const onToggle = vi.fn()
    const onCreateThread = vi.fn()
    const user = userEvent.setup()
    renderHeader({ onToggleFold: onToggle, onCreateThread })
    fireEvent.mouseEnter(screen.getByTestId('space-header-row'))
    screen.getByTestId('new-thread').focus()
    await user.keyboard('{Enter}')
    expect(onCreateThread).toHaveBeenCalledTimes(1)
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('no background at rest, the same idle-ground hover every other row takes', () => {
    renderHeader()
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
      renderHeader()
      fireEvent.doubleClick(screen.getByText('p1'))
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveValue('p1')
      expect(input).toHaveFocus()
    })

    it('Enter confirms and calls performRenameProject with the project id and new name', () => {
      renderHeader()
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
      renderHeader()
      fireEvent.doubleClick(screen.getByText('p1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'New Name' } })
      fireEvent.keyDown(input, { key: 'Escape' })
      expect(rowActions.performRenameProject).not.toHaveBeenCalled()
      expect(screen.getByText('p1')).toBeInTheDocument()
    })

    it('blur without Enter/Escape commits the rename, matching develop', () => {
      renderHeader()
      fireEvent.doubleClick(screen.getByText('p1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'Blurred Name' } })
      fireEvent.blur(input)
      expect(rowActions.performRenameProject).toHaveBeenCalledWith('p1', 'Blurred Name')
    })

    it('a single click on the name still folds the space, not just double-click', () => {
      const onToggle = vi.fn()
      renderHeader({ onToggleFold: onToggle })
      fireEvent.click(screen.getByText('p1'))
      expect(onToggle).toHaveBeenCalledTimes(1)
    })

    it('clicking inside the input while renaming does not toggle the fold', () => {
      const onToggle = vi.fn()
      renderHeader({ onToggleFold: onToggle })
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
      renderHeader({ onToggleFold: onToggle })
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
      renderHeader()
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
      renderHeader()
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
      renderHeader()
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
