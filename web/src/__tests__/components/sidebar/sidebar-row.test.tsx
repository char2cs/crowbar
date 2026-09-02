import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { GitBranch, Lock } from '@phosphor-icons/react'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import * as rowActions from '@/components/sidebar/lib/row-actions'
import {
  getInitialInlineRenameState,
  useSidebarInlineRenameStore,
} from '@/lib/store/sidebar-inline-rename'

vi.mock('@/components/sidebar/lib/row-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof rowActions>()),
  performPromoteChat: vi.fn().mockResolvedValue(undefined),
  performRenameRow: vi.fn().mockResolvedValue(undefined),
}))

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarInlineRenameStore.setState(getInitialInlineRenameState())
})

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

/**
 * A non-home branch row — used by the trailing-cluster cases below, which are
 * about the CLUSTER (which controls, in what order, with what treatment), not
 * about any one row kind.
 */
const deletableRow: SidebarRowType = {
  ...baseRow,
  kind: 'branch',
  parentId: 'parent-1',
  branchName: 'my-feature',
  ownsWorktree: true,
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

  // Addendum §1: the single contextual "+" is gone — Fork and Thread are two
  // separate, always-rendered buttons, and the trash button that used to lead
  // this cluster is gone entirely (deleting moved to drag-to-trash,
  // addendum §2). `onTrash` is still passed here (sidebar-tree.tsx still
  // threads it to every row) to prove it renders nothing on its own.
  it('trailing controls are fork, thread, chevron in that order, revealed on hover', () => {
    render(
      <SidebarRow
        row={deletableRow}
        depth={0}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        onToggleFold={vi.fn()}
      />,
    )
    const controls = screen.getAllByRole('button')
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual(['fork', 'thread', 'fold'])
  })

  it('no HANDLER-driven trailing controls render when no handler is passed for them', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
    // baseRow is itself a promotable bubble (chat, !ownsWorktree, !working), so
    // its own intrinsic promote dropdown still renders on the glyph — it is
    // driven by the row's own fields, unlike fork/thread/fold, which only
    // render when a caller opts in with a handler prop (trash no longer
    // exists as a row control at all — see addendum §1/§2).
    const handlerDriven = screen
      .queryAllByRole('button')
      .filter((b) => b.hasAttribute('data-control'))
    expect(handlerDriven).toHaveLength(0)
  })

  // Addendum §1 (revises spec §3.1): Fork and Thread are both always legal
  // and always rendered — neither depends on `row.ownsWorktree` any more.
  it('the thread control always mints a thread, regardless of ownsWorktree', () => {
    const onCreate = vi.fn()
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onCreate={onCreate} />)
    screen.getByRole('button', { name: /thread/i }).click()
    expect(onCreate).toHaveBeenCalledWith('row-1', 'thread')
  })

  it('the fork control always mints a workspace, regardless of ownsWorktree', () => {
    const onCreate = vi.fn()
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onCreate={onCreate} />)
    screen.getByRole('button', { name: /fork/i }).click()
    expect(onCreate).toHaveBeenCalledWith('row-1', 'workspace')
  })

  it('both Fork and Thread render on a row that owns a worktree too', () => {
    const onCreate = vi.fn()
    render(
      <SidebarRow
        row={{ ...baseRow, ownsWorktree: true }}
        depth={0}
        onOpen={vi.fn()}
        onCreate={onCreate}
      />,
    )
    expect(screen.getByRole('button', { name: /fork/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /thread/i })).toBeInTheDocument()
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

  // Task 5 (icon personalization): the project-home row's glyph is the
  // repo's own click-to-edit icon (EditableRepoIcon, repo-icon-mark.tsx)
  // once `repoIcon` has seeded, not the static GitBranch mark every other
  // branch row draws.
  describe('project-home row icon', () => {
    const repoIcon = {
      repoId: 'r1',
      projectId: 'p1',
      name: 'crowbar',
      avatarLabel: 'C',
      avatarColor: 'bg-indigo-700',
    }
    const homeRow: SidebarRowType = {
      ...baseRow,
      kind: 'branch',
      parentId: null,
      ownsWorktree: true,
      repoIcon,
    }

    it('draws the repo mark instead of the static GitBranch glyph once repoIcon has seeded', () => {
      const { container } = render(<SidebarRow row={homeRow} depth={0} onOpen={vi.fn()} />)
      expect(screen.getByRole('button', { name: /edit crowbar icon/i })).toBeInTheDocument()
      // The letter tile — RepoIconMark's own default, drawn as the popover's
      // trigger content, in place of the plain GitBranch mark.
      expect(container.textContent).toContain('C')
    })

    it('falls back to the static glyph when repoIcon has not seeded yet', () => {
      render(<SidebarRow row={{ ...homeRow, repoIcon: undefined }} depth={0} onOpen={vi.fn()} />)
      expect(screen.queryByRole('button', { name: /edit crowbar icon/i })).not.toBeInTheDocument()
    })

    it('clicking the icon opens the picker, not onOpen', async () => {
      const user = userEvent.setup()
      const onOpen = vi.fn()
      render(<SidebarRow row={homeRow} depth={0} onOpen={onOpen} />)
      await user.click(screen.getByRole('button', { name: /edit crowbar icon/i }))
      expect(await screen.findByText('Icon')).toBeInTheDocument()
      expect(onOpen).not.toHaveBeenCalled()
    })

    it('a working project-home row still shows the spinner, not the icon', () => {
      const { container } = render(
        <SidebarRow row={{ ...homeRow, working: true }} depth={0} onOpen={vi.fn()} />,
      )
      expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
      expect(screen.queryByRole('button', { name: /edit crowbar icon/i })).not.toBeInTheDocument()
    })
  })

  // Addendum §6: a locked/protected branch (repo/project home, or any other
  // locked branch `rows-from-repo.ts` mints) must draw the Lock glyph, not
  // the plain GitBranch mark every ordinary workspace draws — confirmed live
  // as wrong for `main`. `rows-from-repo.ts`'s own `walk()` sources a locked
  // branch row's `id` from its OWNING CHAT while `workspaceId` still names the
  // workspace — an ordinary fork instead keeps `id === workspaceId` (see that
  // file's own extensive doc on `rowId`). That existing mismatch is the
  // signal used here; no new field was added to `SidebarRow` for it.
  describe('locked branch glyph', () => {
    function iconMarkup(el: React.ReactElement): string {
      const { container, unmount } = render(el)
      const html = container.querySelector('svg')?.outerHTML ?? ''
      unmount()
      return html
    }

    it('renders the Lock glyph when the row id was sourced from its owning chat (locked branch)', () => {
      const lockedRow: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'branch-chat-1',
        parentId: 'parent-1',
        workspaceId: 'ws-locked',
        ownsWorktree: true,
        branchName: 'develop',
      }
      const html = iconMarkup(<SidebarRow row={lockedRow} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(<Lock aria-hidden="true" className="size-4" weight="fill" />)
      expect(html).toBe(expected)
    })

    it('renders the Lock glyph on the repo/project-home row when its own repoIcon has not seeded', () => {
      const homeRow: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'home-branch-row',
        parentId: null,
        workspaceId: 'ws-home',
        ownsWorktree: true,
        branchName: 'main',
      }
      const html = iconMarkup(<SidebarRow row={homeRow} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(<Lock aria-hidden="true" className="size-5" weight="fill" />)
      expect(html).toBe(expected)
    })

    it('renders the plain GitBranch glyph for a regular fork, whose id equals its own workspaceId', () => {
      const forkRow: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'ws-1',
        parentId: 'parent-1',
        workspaceId: 'ws-1',
        ownsWorktree: true,
        branchName: 'feature/x',
      }
      const html = iconMarkup(<SidebarRow row={forkRow} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(<GitBranch aria-hidden="true" className="size-4" weight="fill" />)
      expect(html).toBe(expected)
    })
  })

  it('clicking the row body opens it, not the trailing controls', () => {
    const onOpen = vi.fn()
    const onCreate = vi.fn()
    render(<SidebarRow row={deletableRow} depth={0} onOpen={onOpen} onCreate={onCreate} />)
    screen.getByRole('button', { name: /fork/i }).click()
    expect(onCreate).toHaveBeenCalledWith('row-1', 'workspace')
    expect(onOpen).not.toHaveBeenCalled()
  })

  it('clicking the row body itself calls onOpen with the row id', () => {
    const onOpen = vi.fn()
    render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} />)
    screen.getByRole('treeitem').click()
    expect(onOpen).toHaveBeenCalledWith('row-1')
  })

  // Double-click-to-rename (restored from the deleted tree's per-row inline
  // editors) is wired via a DOM-delegated `dblclick` listener in
  // sidebar-tree-chrome.tsx — the same "sibling, not a hook inside the tree"
  // design row-context-menu.tsx's own `contextmenu` listener already uses, so
  // opening the rename dialog does not force every row in the tree to
  // re-render. `SidebarRow` itself only needs to mark which span is the
  // trigger surface, so the delegated listener can tell a double-click on the
  // label apart from one on the trailing fork/thread/fold controls.
  it('the label span carries the delegation marker double-click-to-rename targets', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
    expect(screen.getByText('Fix the thing')).toHaveAttribute('data-sidebar-row-label')
  })

  // Addendum §1/§2: the trash button is gone from every row kind, not just
  // the protected-branch one — deleting is now a drag-to-trash gesture built
  // elsewhere. `onTrash` is still accepted (see the prop's own doc) purely
  // for type-compat with `sidebar-tree.tsx`, which still threads a handler
  // down to every row; it must render nothing regardless.
  it('no row kind renders a trash control, even though onTrash is supplied', () => {
    const rows: SidebarRowType[] = [
      baseRow,
      { ...baseRow, kind: 'branch', parentId: 'parent-1', branchName: 'my-feature', ownsWorktree: true },
      { ...baseRow, kind: 'branch', parentId: null, branchName: 'develop', ownsWorktree: true },
      { ...baseRow, kind: 'folder', ownsWorktree: true },
    ]
    for (const row of rows) {
      const { unmount } = render(<SidebarRow row={row} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} />)
      expect(screen.queryByTestId('trash-control')).not.toBeInTheDocument()
      expect(document.querySelector('[data-control="trash"]')).not.toBeInTheDocument()
      unmount()
    }
  })

  it('a chat row shows both HANDLER-driven trailing controls plus fold, trash never among them', () => {
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
    const controls = screen.getAllByRole('button').filter((b) => b.hasAttribute('data-control'))
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual(['fork', 'thread', 'fold'])
  })

  // §3.5/§4.2: a bubble chat's glyph is itself a promotion dropdown — gated
  // purely on the row's own fields (row.kind === 'chat' && !row.ownsWorktree
  // && !row.working), never on a caller-supplied handler.
  describe('promotion dropdown', () => {
    it('renders on a bubble chat row (chat, no worktree, not working)', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      expect(screen.getByTestId('promote-dropdown')).toBeInTheDocument()
    })

    it('does not render on a chat row that already owns a worktree', () => {
      render(<SidebarRow row={{ ...baseRow, ownsWorktree: true }} depth={0} onOpen={vi.fn()} />)
      expect(screen.queryByTestId('promote-dropdown')).not.toBeInTheDocument()
    })

    it('does not render on a working chat row', () => {
      render(<SidebarRow row={{ ...baseRow, working: true }} depth={0} onOpen={vi.fn()} />)
      expect(screen.queryByTestId('promote-dropdown')).not.toBeInTheDocument()
    })

    it('does not render on a non-chat row', () => {
      render(
        <SidebarRow
          row={{ ...baseRow, kind: 'folder', ownsWorktree: false }}
          depth={0}
          onOpen={vi.fn()}
        />,
      )
      expect(screen.queryByTestId('promote-dropdown')).not.toBeInTheDocument()
    })

    it('opens to a single "Make workspace" item', async () => {
      const user = userEvent.setup()
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      await user.click(screen.getByTestId('promote-dropdown'))
      expect(await screen.findByText('Make workspace')).toBeInTheDocument()
    })

    it('clicking "Make workspace" calls performPromoteChat with the row id, and does not fire onOpen', async () => {
      const user = userEvent.setup()
      const onOpen = vi.fn()
      render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} />)
      await user.click(screen.getByTestId('promote-dropdown'))
      await user.click(await screen.findByText('Make workspace'))
      expect(rowActions.performPromoteChat).toHaveBeenCalledWith('row-1')
      expect(onOpen).not.toHaveBeenCalled()
    })

    it('clicking the dropdown trigger itself does not fire onOpen', async () => {
      const user = userEvent.setup()
      const onOpen = vi.fn()
      render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} />)
      await user.click(screen.getByTestId('promote-dropdown'))
      expect(onOpen).not.toHaveBeenCalled()
    })
  })

  // Task 11: double-click-to-rename is a real inline `<input>` replacing the
  // label in place — restored to match `develop`'s actual behavior, not the
  // modal Task 4 wrongly built. Driven by `sidebar-inline-rename.ts`'s store
  // (set by the delegated dblclick listener in sidebar-tree-chrome.tsx), read
  // here the same way a real double-click would leave it.
  describe('inline rename', () => {
    it('renders the real, focused input in place of the label when this row is the one renaming', () => {
      useSidebarInlineRenameStore.getState().startRenaming('row-1')
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveValue('Fix the thing')
      expect(input).toHaveFocus()
    })

    it('a different row renaming leaves this row showing its plain label', () => {
      useSidebarInlineRenameStore.getState().startRenaming('some-other-row')
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
      expect(screen.getByText('Fix the thing')).toBeInTheDocument()
    })

    it('confirming (Enter) calls performRenameRow with the row id and stops renaming', () => {
      useSidebarInlineRenameStore.getState().startRenaming('row-1')
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'New title')
      expect(useSidebarInlineRenameStore.getState().renamingRowId).toBeNull()
    })

    it('Escape cancels with no call to performRenameRow', () => {
      useSidebarInlineRenameStore.getState().startRenaming('row-1')
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Escape' })
      expect(rowActions.performRenameRow).not.toHaveBeenCalled()
      expect(useSidebarInlineRenameStore.getState().renamingRowId).toBeNull()
    })

    it('blur without Enter/Escape commits the rename, matching develop', () => {
      useSidebarInlineRenameStore.getState().startRenaming('row-1')
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'Blurred title' } })
      fireEvent.blur(input)
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'Blurred title')
    })

    it('unchanged value does not call performRenameRow', () => {
      useSidebarInlineRenameStore.getState().startRenaming('row-1')
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Enter' })
      expect(rowActions.performRenameRow).not.toHaveBeenCalled()
    })

    it('clicking inside the input does not fire onOpen', () => {
      useSidebarInlineRenameStore.getState().startRenaming('row-1')
      const onOpen = vi.fn()
      render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} />)
      fireEvent.click(screen.getByRole('textbox'))
      expect(onOpen).not.toHaveBeenCalled()
    })

    it('a branch row renames with a monospace input, matching its label', () => {
      useSidebarInlineRenameStore.getState().startRenaming('row-1')
      render(
        <SidebarRow
          row={{ ...baseRow, kind: 'branch', parentId: 'p1', ownsWorktree: true }}
          depth={0}
          onOpen={vi.fn()}
        />,
      )
      expect(screen.getByRole('textbox')).toHaveClass('font-mono')
    })

    // A chat that is the live pane (row.hasView) renders through TWO
    // SidebarRow instances at once — its tree row, and a second one
    // recents-band.tsx's RecentsMemberRow builds for the same chat id.
    // Both used to read the SAME `renamingRowId === row.id` with no
    // notion of which DOM instance was actually double-clicked: starting
    // a rename flipped BOTH into rename mode, the second one's own
    // mount-time focus()+select() stole focus from the first (jsdom fires
    // real focus/blur here, same as a browser), and that unhandled blur
    // committed the unchanged value — cancelling the rename before it was
    // ever visible. `inlineRenameDisabled` is what recents-band.tsx now
    // sets on its own instance to keep this from happening.
    it('a second same-id instance (Recents mirroring a live pane) does not steal focus and cancel the tree row rename', () => {
      useSidebarInlineRenameStore.getState().startRenaming('row-1')
      render(
        <>
          <SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />
          <SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} inlineRenameDisabled />
        </>,
      )
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveFocus()
      expect(useSidebarInlineRenameStore.getState().renamingRowId).toBe('row-1')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'New title')
    })
  })
})
