import { describe, expect, it, vi, beforeEach } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Folder, FolderOpen, GitBranch, GitPullRequest, Lock, Warning } from '@phosphor-icons/react'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import * as rowActions from '@/components/sidebar/lib/row-actions'
import * as spaceContentActions from '@/components/layout/space-content-actions'
import {
  getInitialInlineRenameState,
  useSidebarInlineRenameStore,
} from '@/lib/store/sidebar-inline-rename'

vi.mock('@/components/sidebar/lib/row-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof rowActions>()),
  performRenameRow: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/components/layout/space-content-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof spaceContentActions>()),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
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

/** What rows-from-repo.ts stamps on the ONE header row per repo — the
 *  field that makes a branch row the repo's project-home row. */
const headerIcon: NonNullable<SidebarRowType['repoIcon']> = {
  repoId: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
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

  // Default (no `hasViewIdle`) is what recents-band.tsx's own instance keeps
  // relying on — see that file's note on why a solo "open but off screen"
  // entry there needs the label itself to carry this signal.
  it('a row with a view greys its label by default, focused or not', () => {
    render(<SidebarRow row={{ ...baseRow, hasView: true }} depth={0} onOpen={vi.fn()} />)
    const label = screen.getByText('Fix the thing')
    expect(label.className).toMatch(/text-muted-foreground|opacity/)
  })

  // User correction, live: a tree row with an open view read as disabled
  // (greyed), not active — "that row is active by definition." `hasViewIdle`
  // is what sidebar-tree.tsx's own `SidebarTreeRow` sets to swap the muted
  // label for the row's own idle ground instead — the same treatment
  // recents-band.tsx already gives a dormant SET member for the identical
  // "open, not currently showing" state (ROW_HAS_VIEW_IDLE's own doc).
  it('hasViewIdle gives a row with a view the idle ground instead of a greyed label', () => {
    render(
      <SidebarRow row={{ ...baseRow, hasView: true }} depth={0} onOpen={vi.fn()} hasViewIdle />,
    )
    const label = screen.getByText('Fix the thing')
    expect(label.className).not.toContain('text-muted-foreground')
    expect(screen.getByRole('treeitem').className).toContain('bg-sidebar-element-idle')
  })

  it('hasViewIdle has no effect on a row with no view at all', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} hasViewIdle />)
    // The persistent, unconditional token — not `hover:bg-sidebar-element-idle`,
    // which ROW_INACTIVE now always carries for its own plain hover (see that
    // token's own doc: unified with ROW_HAS_VIEW_IDLE's resting ground).
    expect(screen.getByRole('treeitem').className.split(/\s+/)).not.toContain(
      'bg-sidebar-element-idle',
    )
  })

  // Correction after a too-broad first pass ("apply the CossUI shadow on all
  // the sidebar rows" was implemented literally onto the tree's own hover and
  // has-view-idle states) — user correction: the tree never gets this glossy
  // top-highlight, and neither does a plain hover on its own. It stays
  // reserved for ROW_ACTIVE (Recents' showing row) and a Recents SET's own
  // group-hover treatment (recents-band.test.tsx).
  it('an ordinary row never carries the CossUI top-highlight, hovered or not', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
    const treeitem = screen.getByRole('treeitem').className
    expect(treeitem).not.toContain('inset-shadow-[0_1px_var(--elevated-highlight)]')
    expect(treeitem).not.toContain('shadow-xs')
  })

  it('a has-view-idle row never carries the CossUI top-highlight either', () => {
    render(
      <SidebarRow row={{ ...baseRow, hasView: true }} depth={0} onOpen={vi.fn()} hasViewIdle />,
    )
    const treeitem = screen.getByRole('treeitem').className
    expect(treeitem).not.toContain('inset-shadow-[0_1px_var(--elevated-highlight)]')
    expect(treeitem).not.toContain('shadow-xs')
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
  // separate, always-rendered buttons. Spec §9 gives an ordinary (unlocked,
  // non-home) branch row a trash too, same as a chat or folder — see
  // sidebar-row.test.tsx's own remove-control coverage below for the two
  // rows it's withheld from (a locked branch, the project home).
  //
  // Order per the trailing-cluster spec (sidebar-row-actions.tsx): repo-menu,
  // Remove, Thread, Branch, Dropdown — this row never gets repo-menu, so
  // Remove leads.
  it('trailing controls on a branch row are remove, thread, fork, chevron', () => {
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
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual([
      'remove',
      'thread',
      'fork',
      'fold',
    ])
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

  it('the fork control mints a workspace on a branch row', () => {
    const onCreate = vi.fn()
    render(
      <SidebarRow
        row={{ ...baseRow, kind: 'branch', ownsWorktree: true }}
        depth={0}
        onOpen={vi.fn()}
        onCreate={onCreate}
      />,
    )
    screen.getByRole('button', { name: /fork/i }).click()
    expect(onCreate).toHaveBeenCalledWith('row-1', 'workspace')
  })

  // Explicit product correction: a thread is not itself a branch, so it must
  // not be allowed to mint one as a child — Fork never renders on a `chat`
  // row, full stop (not conditioned on `canFork`/`ownsWorktree` any more).
  it('never renders Fork on a chat row — a thread cannot have a branch child', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onCreate={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /fork/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /thread/i })).toBeInTheDocument()
  })

  // Regression: `ROW_SUB_ACTION_HOVER` shows this whole cluster on
  // `group-focus-within` too (a keyboard user tabbing to it), but a mouse
  // click leaves the clicked button genuinely `:focus`ed — `:focus-visible`
  // suppresses the ring for a pointer click, but `:focus-within` still
  // matches plain `:focus` — so without an explicit blur, the fork/thread/
  // fold cluster stayed lit long after the pointer moved off the row.
  it('the fork/thread/fold buttons blur themselves after firing, so the cluster does not stay stuck open', () => {
    render(
      <SidebarRow
        row={deletableRow}
        depth={0}
        onOpen={vi.fn()}
        onCreate={vi.fn()}
        onToggleFold={vi.fn()}
      />,
    )
    for (const name of [/fork/i, /thread/i, /expand|collapse/i]) {
      const button = screen.getByRole('button', { name })
      button.focus()
      expect(document.activeElement).toBe(button)
      button.click()
      expect(document.activeElement).not.toBe(button)
    }
  })

  // Regression, reported live: Fork was offered on a project-home chat with
  // no git repo behind it at all. `row.canFork` is how such a row (set by
  // rows-from-home.ts) says so — Thread stays unaffected, since threading
  // into the project's own home workspace is real (space-content-actions.ts).
  it('hides Fork (never Thread) on a chat row whose canFork is explicitly false', () => {
    const onCreate = vi.fn()
    render(
      <SidebarRow
        row={{ ...baseRow, canFork: false }}
        depth={0}
        onOpen={vi.fn()}
        onCreate={onCreate}
      />,
    )
    expect(screen.queryByRole('button', { name: /fork/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /thread/i })).toBeInTheDocument()
  })

  it('both Fork and Thread render on a row that owns a worktree too', () => {
    const onCreate = vi.fn()
    render(<SidebarRow row={deletableRow} depth={0} onOpen={vi.fn()} onCreate={onCreate} />)
    expect(screen.getByRole('button', { name: /fork/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /thread/i })).toBeInTheDocument()
  })

  // The repo header row is identified by the repo icon it carries, not by
  // sitting at the root: a header filed into a home folder keeps its parent
  // AND its identity (sidebar-row-repo-header-in-folder.test.tsx).
  it('a project-home row (branch carrying repoIcon) gets the 20px glyph exception', () => {
    const { container } = render(
      <SidebarRow
        row={{
          ...baseRow,
          kind: 'branch',
          parentId: null,
          ownsWorktree: true,
          repoIcon: headerIcon,
        }}
        depth={0}
        onOpen={vi.fn()}
      />,
    )
    expect(container.querySelector('.size-5')).toBeInTheDocument()
  })

  // Regression, caught live: recents-band.tsx renders every row with
  // `parentId: null` (§5.1, "no parentage") and — once a Recents row could
  // carry `kind: 'branch'` for a workspace-owning chat's real icon — an
  // ordinary chat's Recents mirror wore the repo header's 20px glyph
  // exception. `inlineRenameDisabled` is the one signal that already meant
  // "this instance is Recents' mirror, not the tree's own row" (see its own
  // doc) — it must also suppress isProjectHome.
  it('a branch row does NOT get the project-home treatment when it is a Recents mirror (inlineRenameDisabled)', () => {
    const { container } = render(
      <SidebarRow
        row={{
          ...baseRow,
          kind: 'branch',
          parentId: null,
          ownsWorktree: true,
          repoIcon: headerIcon,
        }}
        depth={0}
        onOpen={vi.fn()}
        inlineRenameDisabled
      />,
    )
    expect(container.querySelector('.size-5')).not.toBeInTheDocument()
    expect(container.querySelector('.size-4')).toBeInTheDocument()
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
  // as wrong for `main`. Every workspace-owning row is now id'd from its
  // owning chat, locked or not (`rows-from-repo.ts`'s `foldWorkspaceOwners`), so
  // `row.locked` (`Workspace.status === 'locked'`, carried straight onto the
  // row) is the signal now — the old id-vs-workspaceId mismatch stopped being
  // unique to the locked case the moment a regular fork started folding too.
  describe('locked branch glyph', () => {
    function iconMarkup(el: React.ReactElement): string {
      const { container, unmount } = render(el)
      const html = container.querySelector('svg')?.outerHTML ?? ''
      unmount()
      return html
    }

    it('renders the Lock glyph when the row is locked', () => {
      const lockedRow: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'branch-chat-1',
        parentId: 'parent-1',
        workspaceId: 'ws-locked',
        ownsWorktree: true,
        branchName: 'develop',
        locked: true,
      }
      const html = iconMarkup(<SidebarRow row={lockedRow} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(<Lock aria-hidden="true" className="size-4" weight="fill" />)
      expect(html).toBe(expected)
    })

    it('renders the Lock glyph on a locked branch sitting at the repo root (no repoIcon: not the header)', () => {
      const rootLocked: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'root-locked-row',
        parentId: null,
        workspaceId: 'ws-release',
        ownsWorktree: true,
        branchName: 'release/1.x',
        locked: true,
      }
      const html = iconMarkup(<SidebarRow row={rootLocked} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(<Lock aria-hidden="true" className="size-4" weight="fill" />)
      expect(html).toBe(expected)
    })

    it('renders the plain GitBranch glyph for a regular (unlocked) fork', () => {
      const forkRow: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'chat-owning-ws-1',
        parentId: 'parent-1',
        workspaceId: 'ws-1',
        ownsWorktree: true,
        branchName: 'feature/x',
        locked: false,
      }
      const html = iconMarkup(<SidebarRow row={forkRow} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(<GitBranch aria-hidden="true" className="size-4" weight="fill" />)
      expect(html).toBe(expected)
    })
  })

  // Caught live: a workspace stuck as a placeholder (checked out at another
  // worktree — `placeholder-toast-watcher.tsx`'s own "Couldn't set up ..."
  // toast) drew the plain GitBranch/Lock glyph like any other row, because
  // `RowGlyph` never read `status`/`isPlaceholder` at all — the toast fired
  // with nothing on the row itself to explain why. Delegating to
  // `WorkspaceBranchIcon` (workspace-branch-icon.tsx) off those two fields is
  // the fix; these pin that it actually reaches the row, PR states included.
  describe('workspace status glyph', () => {
    function iconMarkup(el: React.ReactElement): string {
      const { container, unmount } = render(el)
      const html = container.querySelector('svg')?.outerHTML ?? ''
      unmount()
      return html
    }

    it('renders the warning glyph for a placeholder, even though it is also locked', () => {
      const placeholderRow: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'branch-chat-1',
        parentId: 'parent-1',
        workspaceId: 'ws-placeholder',
        ownsWorktree: true,
        branchName: 'main',
        locked: true,
        status: 'locked',
        isPlaceholder: true,
        // The narrower half: Crowbar TRIED and could not. The repo's own
        // checkout holding its own default branch is `isPlaceholder` too and
        // must NOT draw this glyph (rows-from-repo-own-default-branch.test.ts).
        needsProvisioning: true,
      }
      const html = iconMarkup(<SidebarRow row={placeholderRow} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(
        <Warning
          role="img"
          aria-label="Branch needs provisioning"
          className="size-4 shrink-0 text-amber-500"
          weight="fill"
        />,
      )
      expect(html).toBe(expected)
    })

    it('renders the PR-conflicts warning glyph for an ordinary (unlocked) fork', () => {
      const conflictedRow: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'chat-owning-ws-1',
        parentId: 'parent-1',
        workspaceId: 'ws-1',
        ownsWorktree: true,
        branchName: 'feature/x',
        locked: false,
        status: 'pr-conflicts',
      }
      const html = iconMarkup(<SidebarRow row={conflictedRow} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(
        <Warning aria-hidden="true" className="size-4 shrink-0 text-amber-500" weight="fill" />,
      )
      expect(html).toBe(expected)
    })

    it('renders the open-PR glyph for a fork with an open pull request', () => {
      const prOpenRow: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'chat-owning-ws-2',
        parentId: 'parent-1',
        workspaceId: 'ws-2',
        ownsWorktree: true,
        branchName: 'feature/y',
        locked: false,
        status: 'pr-open',
      }
      const html = iconMarkup(<SidebarRow row={prOpenRow} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(
        <GitPullRequest
          aria-hidden="true"
          className="size-4 shrink-0 text-green-500"
          weight="fill"
        />,
      )
      expect(html).toBe(expected)
    })

    // A row whose Workspace half has not landed yet (walkTreeIntoRows's
    // "no Workspace record" push) carries no `status` at all — this must
    // still fall back to the plain locked/GitBranch guess, not blow up.
    it('falls back to the plain GitBranch glyph with no status to delegate on', () => {
      const noStatusRow: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        id: 'chat-owning-ws-3',
        parentId: 'parent-1',
        workspaceId: 'ws-3',
        ownsWorktree: true,
        branchName: 'feature/z',
        locked: false,
      }
      const html = iconMarkup(<SidebarRow row={noStatusRow} depth={0} onOpen={vi.fn()} />)
      const expected = iconMarkup(<GitBranch aria-hidden="true" className="size-4" weight="fill" />)
      expect(html).toBe(expected)
    })
  })

  // Caught live: `RowGlyph` drew the same closed `Folder` glyph regardless of
  // fold state — `expanded` was computed (line 110-ish, `!folded`) but never
  // threaded through to it, so a folder never visually distinguished open
  // from closed at all.
  describe('folder open/closed glyph', () => {
    function iconMarkup(el: React.ReactElement): string {
      const { container, unmount } = render(el)
      const html = container.querySelector('svg')?.outerHTML ?? ''
      unmount()
      return html
    }
    const folderRow: SidebarRowType = { ...baseRow, kind: 'folder', ownsWorktree: false }

    it('renders the closed Folder glyph while collapsed', () => {
      const html = iconMarkup(<SidebarRow row={folderRow} depth={0} onOpen={vi.fn()} folded />)
      const expected = iconMarkup(<Folder aria-hidden="true" className="size-4" weight="duotone" />)
      expect(html).toBe(expected)
    })

    it('renders the open FolderOpen glyph while expanded', () => {
      const html = iconMarkup(
        <SidebarRow row={folderRow} depth={0} onOpen={vi.fn()} folded={false} />,
      )
      const expected = iconMarkup(
        <FolderOpen aria-hidden="true" className="size-4" weight="duotone" />,
      )
      expect(html).toBe(expected)
    })
  })

  // Rule 6: a `branch` row that owns a real, unlocked workspace now draws a
  // second line under its label — the workspace's branch name and change
  // counts, muted, beneath the (now chat-titled) label line.
  describe('branch row second line', () => {
    const forkRow: SidebarRowType = {
      ...baseRow,
      kind: 'branch',
      id: 'chat-1',
      parentId: 'parent-1',
      label: 'Fix the parser',
      workspaceId: 'ws-1',
      ownsWorktree: true,
      branchName: 'feature/parser-fix',
      added: 42,
      deleted: 7,
      locked: false,
    }

    it('shows the branch name and change counts on a second line', () => {
      render(<SidebarRow row={forkRow} depth={0} onOpen={vi.fn()} />)
      expect(screen.getByText('Fix the parser')).toBeInTheDocument()
      expect(screen.getByText('+42')).toBeInTheDocument()
      expect(screen.getByText('-7')).toBeInTheDocument()
      expect(screen.getByText(/feature\/parser-fix/)).toBeInTheDocument()
    })

    it('omits the counts when there is no diff yet, but still shows the branch name', () => {
      render(<SidebarRow row={{ ...forkRow, added: 0, deleted: 0 }} depth={0} onOpen={vi.fn()} />)
      expect(screen.getByText('feature/parser-fix')).toBeInTheDocument()
      expect(screen.queryByText(/^\+/)).not.toBeInTheDocument()
    })

    it('does not draw a second line for a locked branch', () => {
      render(<SidebarRow row={{ ...forkRow, locked: true }} depth={0} onOpen={vi.fn()} />)
      expect(screen.queryByText('feature/parser-fix')).not.toBeInTheDocument()
    })

    it('does not draw a second line for the project-home row', () => {
      render(<SidebarRow row={{ ...forkRow, parentId: null }} depth={0} onOpen={vi.fn()} />)
      expect(screen.queryByText('feature/parser-fix')).not.toBeInTheDocument()
    })

    it('does not draw a second line for a chat bubble row', () => {
      const { container } = render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      expect(container.querySelector('[data-sidebar-row-label] > span')).not.toBeInTheDocument()
    })
  })

  // Rule 8: the fork button mints a child chat session forked into its own
  // workspace — a git operation — so it now draws the same GitBranch mark
  // every worktree-owning row's glyph does, not the old hand-rolled "+".
  it('the fork control draws a GitBranch glyph, not the old "+"', () => {
    render(<SidebarRow row={deletableRow} depth={0} onOpen={vi.fn()} onCreate={vi.fn()} />)
    const fork = screen.getByRole('button', { name: /fork/i })
    expect(fork.querySelector('svg')).toBeInTheDocument()
    // The old "+" was a hand-rolled two-stroke path; GitBranch is Phosphor's
    // own multi-element mark, which never collapses to that exact shape.
    expect(fork.innerHTML).not.toContain('M8 3v10M3 8h10')
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

  // Spec §9: "every row that owns something carries a trash: chats,
  // workspaces, folders, repos, and the space header for the project." The
  // X/"remove" control follows that — chat, folder, and an ordinary
  // (unlocked, non-home) branch all get it. Reported live: a LOCKED branch
  // showed the X too, which read as "this row can be one-click removed" when
  // it plainly cannot (`handleTrash` refuses it with a toast). The repo's
  // own project-home row is the other exclusion here, for the same reason —
  // but unlike a locked branch, it isn't a dead end: it gets its own
  // overflow menu instead (below), which deletes the whole REPO rather than
  // just this one branch.
  it('the remove control renders on chat, folder, and an ordinary branch — never a locked branch or the project home', () => {
    const shown: SidebarRowType[] = [
      baseRow,
      { ...baseRow, kind: 'folder', ownsWorktree: true },
      {
        ...baseRow,
        kind: 'branch',
        parentId: 'parent-1',
        branchName: 'my-feature',
        ownsWorktree: true,
      },
    ]
    for (const row of shown) {
      const { unmount } = render(
        <SidebarRow row={row} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} />,
      )
      expect(document.querySelector('[data-control="remove"]')).toBeInTheDocument()
      unmount()
    }

    const hidden: SidebarRowType[] = [
      { ...baseRow, kind: 'branch', parentId: 'parent-1', branchName: 'my-feature', locked: true },
      {
        ...baseRow,
        kind: 'branch',
        parentId: null,
        branchName: 'develop',
        ownsWorktree: true,
        repoIcon: headerIcon,
      },
    ]
    for (const row of hidden) {
      const { unmount } = render(
        <SidebarRow row={row} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} />,
      )
      expect(document.querySelector('[data-control="remove"]')).not.toBeInTheDocument()
      unmount()
    }
  })

  // recents-band.tsx's own "end this view" control — never wired alongside
  // `onTrash` (opposite semantics: this closes a VIEW, never touches the
  // chat), and, unlike the tree's destructive Remove, it must accept a
  // caller-supplied label so a set member reads "Close <this chat's title>"
  // rather than a copy of the tree's own wording.
  describe('onClose (the non-destructive "end this view" control)', () => {
    it('renders nothing when onClose is not supplied', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      expect(document.querySelector('[data-control="close"]')).not.toBeInTheDocument()
    })

    it('renders and labels the control from the row itself, distinct from Remove', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onClose={vi.fn()} />)
      const close = screen.getByRole('button', { name: `Close ${baseRow.label}` })
      expect(close).toBeInTheDocument()
      expect(close.getAttribute('aria-label')).not.toMatch(/delete|remove/i)
    })

    it('clicking it calls onClose with the row id and never opens the row', () => {
      const onClose = vi.fn()
      const onOpen = vi.fn()
      render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} onClose={onClose} />)
      screen.getByRole('button', { name: `Close ${baseRow.label}` }).click()
      expect(onClose).toHaveBeenCalledWith(baseRow.id)
      expect(onOpen).not.toHaveBeenCalled()
    })

    it('coexists with onTrash (never wired together by a real caller, but neither excludes the other structurally)', () => {
      render(
        <SidebarRow
          row={deletableRow}
          depth={0}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
          onClose={vi.fn()}
        />,
      )
      expect(document.querySelector('[data-control="remove"]')).toBeInTheDocument()
      expect(document.querySelector('[data-control="close"]')).toBeInTheDocument()
    })

    // Regression: every trailing button used the ambient `text-muted-
    // foreground`/`hover:bg-sidebar-element-hover` tokens unconditionally,
    // including on a row painted on an inverted `ROW_ACTIVE` ground
    // (Recents' own showing row) — the same low-contrast bug `activeGround`
    // already fixes for the label/icon, just never applied to the buttons,
    // which is what made them unreadable once every close button moved onto
    // this inline cluster (live-reported: "any button are not noticeable").
    it('re-keys its color onto the inverted pair when painted on an activeGround', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onClose={vi.fn()} activeGround />)
      const close = screen.getByRole('button', { name: `Close ${baseRow.label}` })
      expect(close.className).toContain('text-foreground-inverse/70')
      expect(close.className).toContain('hover:text-foreground-inverse')
      expect(close.className).not.toContain('text-muted-foreground')
    })

    it('keeps the ambient color when there is no activeGround', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onClose={vi.fn()} />)
      const close = screen.getByRole('button', { name: `Close ${baseRow.label}` })
      expect(close.className).toContain('text-muted-foreground')
      expect(close.className).not.toContain('text-foreground-inverse')
    })
  })

  describe("the repo-home row's own overflow (repo delete)", () => {
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

    it('renders only on the repo-home row, first in the trailing cluster — not on an ordinary branch', () => {
      render(<SidebarRow row={homeRow} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} />)
      expect(document.querySelector('[data-control="repo-menu"]')).toBeInTheDocument()

      const ordinary: SidebarRowType = {
        ...baseRow,
        kind: 'branch',
        parentId: 'parent-1',
        branchName: 'my-feature',
        ownsWorktree: true,
      }
      const { container } = render(
        <SidebarRow row={ordinary} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} />,
      )
      expect(container.querySelector('[data-control="repo-menu"]')).not.toBeInTheDocument()
    })

    it('is absent until repoIcon has seeded', () => {
      render(
        <SidebarRow
          row={{ ...homeRow, repoIcon: undefined }}
          depth={0}
          onOpen={vi.fn()}
          onTrash={vi.fn()}
        />,
      )
      expect(document.querySelector('[data-control="repo-menu"]')).not.toBeInTheDocument()
    })

    // The button's own click behavior (opening the menu, "Delete Repo" and
    // every other item in it) now lives in row-context-menu.test.tsx — this
    // component only renders the trigger, since `SidebarRowContextMenu`
    // (a sibling, listening on `treeRef`) owns the menu itself, the same one
    // a right-click on this row opens.
  })

  it('a chat row shows remove, thread, and fold — never fork', () => {
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
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual([
      'remove',
      'thread',
      'fold',
    ])
  })

  // The ghost/bootstrap row a childless folder used to render underneath
  // itself (sidebar-tree.tsx, now removed) existed only to hold these two
  // buttons — a folder's own row carries them directly now, same place
  // every other kind's Fork/Thread already lived.
  //
  // A folder applies "the same logic as its parent" (product rule): under a
  // real repo it gets BOTH Fork (gated by `ownsWorktree`, same as a `branch`
  // row) and Thread (ungated, same as every other row kind) — never Fork
  // alone, the way a locked branch's own folder used to.
  it('a folder that owns a worktree shows Thread AND Fork on its own row', () => {
    render(
      <SidebarRow
        row={{ ...baseRow, kind: 'folder', ownsWorktree: true }}
        depth={0}
        onOpen={vi.fn()}
        onCreate={vi.fn()}
        onToggleFold={vi.fn()}
      />,
    )
    const controls = screen.getAllByRole('button').filter((b) => b.hasAttribute('data-control'))
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual(['thread', 'fork', 'fold'])
  })

  // A project-home folder (rows-from-home.ts) never owns a worktree — no
  // repo, nothing to fork — so it gets no Fork button. It DOES still get
  // Thread: project home applies the same logic to a folder it applies to a
  // chat, and a home chat has always gotten Thread with no repo required.
  it('a folder that owns no worktree shows Thread but not Fork', () => {
    render(
      <SidebarRow
        row={{ ...baseRow, kind: 'folder', ownsWorktree: false }}
        depth={0}
        onOpen={vi.fn()}
        onCreate={vi.fn()}
        onToggleFold={vi.fn()}
      />,
    )
    const controls = screen.getAllByRole('button').filter((b) => b.hasAttribute('data-control'))
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual(['thread', 'fold'])
  })

  // The glyph-click "Make workspace" promotion dropdown was removed from the
  // thread logo (reported live) — a bubble chat's glyph is now always the
  // plain static glyph, on every row shape that used to make it promotable.
  describe('promotion dropdown removed', () => {
    it('never renders on a bubble chat row (chat, no worktree, not working)', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      expect(screen.queryByTestId('promote-dropdown')).not.toBeInTheDocument()
      expect(screen.queryByText('Make workspace')).not.toBeInTheDocument()
    })

    it('clicking the glyph opens the row like any other click, never a menu', async () => {
      const user = userEvent.setup()
      const onOpen = vi.fn()
      render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} />)
      await user.click(screen.getByRole('treeitem'))
      expect(onOpen).toHaveBeenCalledWith('row-1')
    })
  })

  // Task 11: double-click-to-rename is a real inline `<input>` replacing the
  // label in place — restored to match `develop`'s actual behavior, not the
  // modal Task 4 wrongly built. Driven by `sidebar-inline-rename.ts`'s store
  // (set by the delegated dblclick listener in sidebar-tree-chrome.tsx), read
  // here the same way a real double-click would leave it.
  // `startRenaming` decides, at the moment it is called, whether a
  // tree-rendered instance of the row is in the DOM (`sidebar-inline-rename.ts`'s
  // own doc) — so every test here starts renaming AFTER the row is mounted,
  // matching how a real double-click or Rename click actually fires (never on
  // a row that hasn't rendered yet).
  describe('inline rename', () => {
    it('renders the real, focused input in place of the label when this row is the one renaming', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveValue('Fix the thing')
      expect(input).toHaveFocus()
    })

    it('a different row renaming leaves this row showing its plain label', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => useSidebarInlineRenameStore.getState().startRenaming('some-other-row'))
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
      expect(screen.getByText('Fix the thing')).toBeInTheDocument()
    })

    it('confirming (Enter) calls performRenameRow with the row id and stops renaming', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'New title')
      expect(useSidebarInlineRenameStore.getState().renamingRowId).toBeNull()
    })

    it('Escape cancels with no call to performRenameRow', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Escape' })
      expect(rowActions.performRenameRow).not.toHaveBeenCalled()
      expect(useSidebarInlineRenameStore.getState().renamingRowId).toBeNull()
    })

    it('blur without Enter/Escape commits the rename, matching develop', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'Blurred title' } })
      fireEvent.blur(input)
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'Blurred title')
    })

    it('unchanged value does not call performRenameRow', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
      fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Enter' })
      expect(rowActions.performRenameRow).not.toHaveBeenCalled()
    })

    it('clicking inside the input does not fire onOpen', () => {
      const onOpen = vi.fn()
      render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} />)
      act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
      fireEvent.click(screen.getByRole('textbox'))
      expect(onOpen).not.toHaveBeenCalled()
    })

    it('a branch row renames with a monospace input, matching its label', () => {
      render(
        <SidebarRow
          row={{ ...baseRow, kind: 'branch', parentId: 'p1', ownsWorktree: true }}
          depth={0}
          onOpen={vi.fn()}
        />,
      )
      act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
      expect(screen.getByRole('textbox')).toHaveClass('font-mono')
    })

    // A chat that is the live pane (row.hasView) renders through TWO
    // SidebarRow instances at once — its tree row, and a second one
    // recents-band.tsx's RecentsMemberRow builds for the same chat id,
    // marked with the real `data-sidebar-recents-row` flag `dragProps`
    // carries in production (`use-sidebar-drag.ts`'s `inRecents`). Both used
    // to read the SAME `renamingRowId === row.id` with no notion of which
    // DOM instance was actually double-clicked: starting a rename flipped
    // BOTH into rename mode, the second one's own mount-time focus()+select()
    // stole focus from the first (jsdom fires real focus/blur here, same as a
    // browser), and that unhandled blur committed the unchanged value —
    // cancelling the rename before it was ever visible. `startRenaming` now
    // checks the DOM for a non-Recents instance and only that one renders —
    // `inlineRenameDisabled` is what tells the Recents instance it is not it.
    it('a second same-id instance (Recents mirroring a live pane) does not steal focus and cancel the tree row rename', () => {
      render(
        <>
          <SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />
          <SidebarRow
            row={baseRow}
            depth={0}
            onOpen={vi.fn()}
            inlineRenameDisabled
            dragProps={{ 'data-sidebar-recents-row': '' }}
          />
        </>,
      )
      act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveFocus()
      expect(useSidebarInlineRenameStore.getState().renamingRowId).toBe('row-1')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'New title')
    })

    // Regression, reported live: double-clicking a Recents row's label started
    // the rename on the row's TREE copy (`inlineRenameDisabled`, above), which
    // shares one scroller with Recents and is normally scrolled far out of
    // view — so nothing visibly happened and the keystrokes went to an input
    // the user could not see. Same silent failure for the context menu's
    // Rename on any row the tree has scrolled away from.
    it('scrolls the editor into view so a rename started from the Recents copy is not invisible', () => {
      const scrollIntoView = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
      try {
        render(
          <>
            <SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />
            <SidebarRow
              row={baseRow}
              depth={0}
              onOpen={vi.fn()}
              inlineRenameDisabled
              dragProps={{ 'data-sidebar-recents-row': '' }}
            />
          </>,
        )
        act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
        expect(scrollIntoView).toHaveBeenCalledTimes(1)
        expect(scrollIntoView.mock.contexts[0]).toBe(screen.getByRole('textbox'))
      } finally {
        scrollIntoView.mockRestore()
      }
    })

    // Live-reproduced: a Recents entry for a row whose TREE copy is not
    // mounted at all — its repo/folder ancestor collapsed, not merely
    // scrolled away — used to have nowhere to draw the editor at all, since
    // `inlineRenameDisabled` always refused it regardless of whether a tree
    // copy actually existed. This is the actual reason `RenameDialog` used
    // to exist as a fallback. `startRenaming` now finds no non-Recents
    // instance in the DOM and lets this, the only mounted instance, draw it.
    it('a Recents-only row (its tree copy is not mounted at all) renames itself', () => {
      render(
        <SidebarRow
          row={baseRow}
          depth={0}
          onOpen={vi.fn()}
          inlineRenameDisabled
          dragProps={{ 'data-sidebar-recents-row': '' }}
        />,
      )
      act(() => useSidebarInlineRenameStore.getState().startRenaming('row-1'))
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveValue('Fix the thing')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'New title')
    })
  })
})

// Regression, reported live: an unlabeled, empty naming input read as a chat
// box to type INTO rather than a name to give the new branch — what got
// typed there became the fork's own branch name (a nonsense one, since the
// user thought they were asking a question). `CREATE_ROW_PLACEHOLDER`
// restores the wording `develop`'s own create-row input showed, minus the
// "or name/ for a folder" half — this input only ever forks a branch now.
describe('a pending row awaiting its branch name', () => {
  const namingRow: SidebarRowType = {
    ...baseRow,
    kind: 'branch',
    ownsWorktree: true,
    pending: { tempId: 'pending-1', status: 'naming' },
  }

  it('shows the branch-name placeholder on the naming input', () => {
    render(<SidebarRow row={namingRow} depth={0} onOpen={vi.fn()} />)
    expect(screen.getByRole('textbox')).toHaveAttribute('placeholder', 'branch-name')
  })
})

// A create the daemon refused carries its reason on the entry
// (`pending-creates.ts`'s `setError` stores `err.message`) — the row must
// say it, not just "failed": a 404 "parent … not found" and a 409 "no fork
// parent" need different fixes, and a bare badge is indistinguishable from
// a network drop.
describe('a pending row whose create failed', () => {
  const failedRow: SidebarRowType = {
    ...baseRow,
    kind: 'branch',
    ownsWorktree: true,
    label: 'test/test',
    pending: {
      tempId: 'pending-1',
      status: 'error',
      error: 'agent chat folder: parent ws-1: apperr: not found',
    },
  }

  it('surfaces the daemon’s own reason, not only a "failed" badge', () => {
    render(<SidebarRow row={failedRow} depth={0} onOpen={vi.fn()} />)
    expect(screen.getByText('failed')).toBeInTheDocument()
    const reason = /parent ws-1: apperr: not found/
    expect(screen.queryByText(reason) ?? screen.queryByTitle(reason)).not.toBeNull()
  })
})
