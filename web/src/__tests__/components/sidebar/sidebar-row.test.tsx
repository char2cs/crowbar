import { describe, expect, it, vi, beforeEach } from 'vitest'
import type { ReactElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Folder, FolderOpen, GitBranch, GitPullRequest, Lock, Warning } from '@phosphor-icons/react'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { RowMetricsContext, TOUCH_ROW_METRICS } from '@/components/sidebar/lib/row-metrics'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import * as rowActions from '@/components/sidebar/lib/row-actions'
import * as createActions from '@/components/layout/create-actions'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { writeChat } from '@/__tests__/__fixtures__/agent-chat'

vi.mock('@/components/sidebar/lib/row-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof rowActions>()),
  performRenameRow: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/components/layout/create-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof createActions>()),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

beforeEach(() => {
  vi.clearAllMocks()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
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

/** What deriveRows stamps on the ONE header row per repo — the
 *  field that makes a branch row the repo's project-home row. */
const headerIcon: NonNullable<SidebarRowType['repoIcon']> = {
  repoId: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
}

/** A non-home branch row, for the trailing-cluster cases (about the cluster, not the row kind). */
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

  // Default (no `hasViewIdle`) is what recents-band.tsx keeps relying on.
  it('a row with a view greys its label by default, focused or not', () => {
    render(<SidebarRow row={{ ...baseRow, hasView: true }} depth={0} onOpen={vi.fn()} />)
    const label = screen.getByText('Fix the thing')
    expect(label.className).toMatch(/text-muted-foreground|opacity/)
  })

  // A tree row with an open view is active, not disabled: `hasViewIdle` swaps the
  // muted label for the row's idle ground, as Recents does for a dormant member.
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
    // The persistent token, not ROW_INACTIVE's own plain hover.
    expect(screen.getByRole('treeitem').className.split(/\s+/)).not.toContain(
      'bg-sidebar-element-idle',
    )
  })

  // The glossy top-highlight is reserved for ROW_ACTIVE and a Recents set's hover;
  // the tree's hover and has-view-idle states never get it.
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
    // FlickerSpinner marks itself with `data-flicker-spinner`, as every call site asserts.
    const { container } = render(
      <SidebarRow row={{ ...baseRow, working: true }} depth={0} onOpen={vi.fn()} />,
    )
    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  // Fork and Thread are separate buttons, and an ordinary branch gets a trash too.
  // Cluster order (sidebar-row-actions.tsx): repo-menu, Remove, Thread, Branch,
  // Dropdown; this row has no repo-menu, so Remove leads.
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
    // baseRow's own promote dropdown is driven by its fields; fork/thread/fold
    // render only when a caller passes a handler.
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

  // A thread is not a branch, so it cannot mint one: Fork never renders on a `chat` row.
  it('never renders Fork on a chat row — a thread cannot have a branch child', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onCreate={vi.fn()} />)
    expect(screen.queryByRole('button', { name: /fork/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /thread/i })).toBeInTheDocument()
  })

  // A clicked button stays `:focus`, which `group-focus-within` matches, so without a
  // blur the cluster stayed lit after the pointer left.
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

  // The thread's own title becomes the branch-name input: the row is not replaced by another.
  it("Make this thread a branch turns the row's title into the branch-name input", () => {
    const onOpen = vi.fn()
    render(<SidebarRow row={{ ...baseRow, canBranchOut: true }} depth={0} onOpen={onOpen} />)
    fireEvent.click(screen.getByRole('button', { name: 'Make this thread a branch' }))
    const input = screen.getByRole('textbox')
    expect(input).toHaveFocus()
    expect(screen.getByRole('treeitem', { name: 'Fix the thing' })).toContainElement(input)
    fireEvent.click(input)
    expect(onOpen).not.toHaveBeenCalled()
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(screen.getByText('Fix the thing')).toBeInTheDocument()
  })

  it('both Fork and Thread render on a row that owns a worktree too', () => {
    const onCreate = vi.fn()
    render(<SidebarRow row={deletableRow} depth={0} onOpen={vi.fn()} onCreate={onCreate} />)
    expect(screen.getByRole('button', { name: /fork/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /thread/i })).toBeInTheDocument()
  })

  // The repo header is identified by its repo icon, not by sitting at the root.
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

  // Recents renders rows with `parentId: null`; `inlineRenameDisabled` marks that
  // mirror, so it must not wear the repo header's glyph exception.
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

  // The project-home row's glyph is the repo's editable icon once `repoIcon` has seeded.
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

  // A locked branch draws the Lock glyph; `row.locked` is the signal, since every
  // workspace-owning row is id'd by its owning chat now.
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

  // A placeholder workspace draws `WorkspaceBranchIcon`'s state on the row itself,
  // or the setup-failure toast has nothing on the row to explain it.
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
        // A repo's own checkout of its default branch is `isPlaceholder` too and
        // must not draw this (views.test.ts).
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

    // A row whose Workspace has not landed carries no `status`: plain fallback glyph.
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

  // The folder glyph follows fold state, open versus closed.
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

  // An unlocked worktree-owning branch row draws its branch name and change
  // counts on a muted second line under the chat-titled label.
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

    it('separates the branch name from the counts with a space, not a dash', () => {
      const { container } = render(<SidebarRow row={forkRow} depth={0} onOpen={vi.fn()} />)
      expect(container.textContent).toContain('feature/parser-fix +42 -7')
      expect(container.textContent).not.toContain('--')
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
      render(
        <SidebarRow
          row={{
            ...forkRow,
            parentId: null,
            repoIcon: headerIcon,
          }}
          depth={0}
          onOpen={vi.fn()}
        />,
      )
      expect(screen.queryByText('feature/parser-fix')).not.toBeInTheDocument()
    })

    it('does not draw a second line for a row drawn as a view tab', () => {
      render(<SidebarRow row={forkRow} depth={0} onOpen={vi.fn()} tab="tab" />)
      expect(screen.getByText('Fix the parser')).toBeInTheDocument()
      expect(screen.queryByText(/feature\/parser-fix/)).not.toBeInTheDocument()
      expect(screen.queryByText('+42')).not.toBeInTheDocument()
    })

    it('does not draw a second line for a chat bubble row', () => {
      const { container } = render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      expect(container.querySelector('[data-sidebar-row-label] > span')).not.toBeInTheDocument()
    })
  })

  // Forking is a git operation, so the fork button draws the GitBranch mark.
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

  // A delegated dblclick listener (sidebar-tree-chrome.tsx) renames; the row only
  // marks the label span so a double-click there differs from one on its controls.
  it('the label span carries the delegation marker double-click-to-rename targets', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
    expect(screen.getByText('Fix the thing')).toHaveAttribute('data-sidebar-row-label')
  })

  // Every row that owns something carries a trash, except a locked branch (which
  // cannot be removed) and the project-home row (which gets its repo menu instead).
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

  // Recents' "end this view" control closes a VIEW, never the chat, and takes a
  // caller-supplied label.
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

    // On the inverted ROW_ACTIVE ground the trailing buttons need the same
    // contrast treatment as the label, or they are unreadable.
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

    // The menu itself is row-context-menu.test.tsx's; this row renders only the trigger.
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

  // A folder carries Fork and Thread itself, with its parent's logic: under a real
  // repo both (Fork gated by `ownsWorktree`), never Fork alone.
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

  // A project-home folder owns no worktree, so no Fork; Thread still, as for a home chat.
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

  // A chat dropped into another workspace mid-turn stays put until the turn ends, and says so.
  describe('a chat waiting to move', () => {
    it('says where it moves after this turn, under its title', () => {
      writeChat({ id: 'row-1', pendingMove: { requestId: 'm1', label: 'feature-b' } })
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      expect(screen.getByText('Fix the thing')).toBeInTheDocument()
      expect(screen.getByText('Moving to feature-b after this turn')).toBeInTheDocument()
    })

    it('says nothing once the move has run', () => {
      writeChat({ id: 'row-1', pendingMove: undefined })
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      expect(screen.queryByText(/after this turn/)).not.toBeInTheDocument()
    })
  })

  // Double-click renames inline, driven by `sidebar-inline-rename.ts`'s store;
  // `startRenaming` checks the DOM, so each test starts after the row mounts.
  describe('inline rename', () => {
    it('renders the real, focused input in place of the label when this row is the one renaming', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => treeUiStore.getState().startRenaming('row-1'))
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveValue('Fix the thing')
      expect(input).toHaveFocus()
    })

    it('a different row renaming leaves this row showing its plain label', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => treeUiStore.getState().startRenaming('some-other-row'))
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
      expect(screen.getByText('Fix the thing')).toBeInTheDocument()
    })

    it('confirming (Enter) calls performRenameRow with the row id and stops renaming', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => treeUiStore.getState().startRenaming('row-1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'New title')
      expect(treeUiStore.getState().renaming).toBeNull()
    })

    it('Escape cancels with no call to performRenameRow', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => treeUiStore.getState().startRenaming('row-1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Escape' })
      expect(rowActions.performRenameRow).not.toHaveBeenCalled()
      expect(treeUiStore.getState().renaming).toBeNull()
    })

    it('blur without Enter/Escape commits the rename, matching develop', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => treeUiStore.getState().startRenaming('row-1'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'Blurred title' } })
      fireEvent.blur(input)
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'Blurred title')
    })

    it('unchanged value does not call performRenameRow', () => {
      render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
      act(() => treeUiStore.getState().startRenaming('row-1'))
      fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Enter' })
      expect(rowActions.performRenameRow).not.toHaveBeenCalled()
    })

    it('clicking inside the input does not fire onOpen', () => {
      const onOpen = vi.fn()
      render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} />)
      act(() => treeUiStore.getState().startRenaming('row-1'))
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
      act(() => treeUiStore.getState().startRenaming('row-1'))
      expect(screen.getByRole('textbox')).toHaveClass('font-mono')
    })

    // A live chat renders twice (tree and Recents): only the tree instance may edit,
    // or the second editor's focus steals and blur-commits the first.
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
      act(() => treeUiStore.getState().startRenaming('row-1'))
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveFocus()
      expect(treeUiStore.getState().renaming).toBe('row-1')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'New title')
    })

    // Renaming a Recents row edits in place there, not on a tree copy that may be
    // scrolled out of view.
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
        act(() => treeUiStore.getState().startRenaming('row-1'))
        expect(scrollIntoView).toHaveBeenCalledTimes(1)
        expect(scrollIntoView.mock.contexts[0]).toBe(screen.getByRole('textbox'))
      } finally {
        scrollIntoView.mockRestore()
      }
    })

    // With no tree copy mounted (its ancestor collapsed), the Recents instance is
    // the only one and draws the editor.
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
      act(() => treeUiStore.getState().startRenaming('row-1'))
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      const input = screen.getByRole('textbox') as HTMLInputElement
      expect(input).toHaveValue('Fix the thing')
      fireEvent.change(input, { target: { value: 'New title' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(rowActions.performRenameRow).toHaveBeenCalledWith('row-1', 'New title')
    })
  })
})

// The phone's one row scale: every kind of row at the same height, title size and glyph box,
// with one secondary size and one indent step.
describe('a row drawn at a given scale', () => {
  const metrics = TOUCH_ROW_METRICS
  const atScale = (row: ReactElement) =>
    render(<RowMetricsContext value={metrics}>{row}</RowMetricsContext>)
  const rows: Array<[string, SidebarRowType, number]> = [
    ['chat', { ...baseRow, labelProvisional: true }, 0],
    ['folder', { ...baseRow, id: 'f', kind: 'folder' }, 1],
    [
      'branch',
      { ...baseRow, id: 'b', kind: 'branch', ownsWorktree: true, branchName: 'feature/x' },
      2,
    ],
    [
      'locked branch',
      { ...baseRow, id: 'l', kind: 'branch', label: 'main', branchName: 'main', locked: true },
      1,
    ],
  ]
  for (const [kind, row, depth] of rows) {
    it(`sizes a ${kind} row from the scale`, () => {
      const { container } = atScale(<SidebarRow row={row} depth={depth} onOpen={vi.fn()} />)
      const item = screen.getByRole('treeitem')
      expect(item.style.height).toBe(`${metrics.height}px`)
      expect(item.style.fontSize).toBe(`${metrics.titleSize}px`)
      const glyph = item.firstElementChild as HTMLElement
      expect(glyph.style.width).toBe(`${metrics.glyph}px`)
      const indent = (container.firstElementChild as HTMLElement).style.marginInlineStart
      expect(indent).toBe(`${depth * metrics.indent}px`)
    })
  }

  it('sets a branch name under its title at the one secondary size', () => {
    atScale(<SidebarRow row={rows[2][1]} depth={0} onOpen={vi.fn()} />)
    expect(screen.getByText('feature/x').style.fontSize).toBe(`${metrics.subSize}px`)
  })

  it('draws a repo header icon at the header size', () => {
    atScale(
      <SidebarRow
        row={{ ...baseRow, kind: 'branch', label: 'crowbar', repoIcon: headerIcon }}
        depth={0}
        onOpen={vi.fn()}
        inlineRenameDisabled
      />,
    )
    const glyph = screen.getByRole('treeitem').firstElementChild as HTMLElement
    expect(glyph.style.width).toBe(`${metrics.glyph}px`)
    expect(glyph.style.height).toBe(`${metrics.glyph}px`)
  })

  // No hover on a phone: the disclosure is always there, and it alone folds the row.
  describe('a foldable row', () => {
    const folder: SidebarRowType = { ...baseRow, id: 'f', kind: 'folder', label: 'Specs' }
    const renderFolder = (folded: boolean) => {
      const onOpen = vi.fn()
      const onToggleFold = vi.fn()
      const view = atScale(
        <SidebarRow
          row={folder}
          depth={0}
          onOpen={onOpen}
          onToggleFold={onToggleFold}
          folded={folded}
        />,
      )
      return { ...view, onOpen, onToggleFold }
    }

    it.each([
      [false, 'Collapse Specs', 'true'],
      [true, 'Expand Specs', 'false'],
    ])(
      'shows a disclosure that says whether the row is open (folded: %s)',
      (folded, name, open) => {
        renderFolder(folded)
        expect(screen.getByRole('button', { name })).toHaveAttribute('aria-expanded', open)
      },
    )

    it('gives the disclosure a touch-sized target', () => {
      renderFolder(false)
      const button = screen.getByRole('button', { name: 'Collapse Specs' })
      expect(button.style.width).toBe(`${metrics.hitTarget}px`)
      expect(button.style.height).toBe(`${metrics.hitTarget}px`)
    })

    it('folds from the disclosure without opening the row', () => {
      const { onOpen, onToggleFold } = renderFolder(false)
      fireEvent.click(screen.getByRole('button', { name: 'Collapse Specs' }))
      expect(onToggleFold).toHaveBeenCalledWith('f')
      expect(onOpen).not.toHaveBeenCalled()
    })

    it('opens from the rest of the row without folding it', () => {
      const { onOpen, onToggleFold } = renderFolder(false)
      fireEvent.click(screen.getByText('Specs'))
      expect(onOpen).toHaveBeenCalledWith('f')
      expect(onToggleFold).not.toHaveBeenCalled()
    })
  })
})
