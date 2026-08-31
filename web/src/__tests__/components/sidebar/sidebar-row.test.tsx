import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import * as rowActions from '@/components/sidebar/lib/row-actions'

vi.mock('@/components/sidebar/lib/row-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof rowActions>()),
  performPromoteChat: vi.fn().mockResolvedValue(undefined),
}))

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
 * A row that actually carries a trash — a forked branch.
 *
 * The trailing-control cases below are about the CLUSTER (which controls, in
 * what order, with what treatment), not about any one row kind, so they need a
 * row that is guaranteed a trash regardless of kind. Every kind gets one now
 * except the project-home row (see the "protected branch" case below).
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

  it('trailing controls are trash, +, chevron in that order, revealed on hover', () => {
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
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual(['trash', 'create', 'fold'])
  })

  it('no HANDLER-driven trailing controls render when no handler is passed for them', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} />)
    // baseRow is itself a promotable bubble (chat, !ownsWorktree, !working), so
    // its own intrinsic promote dropdown still renders on the glyph — it is
    // driven by the row's own fields, unlike trash/create/fold, which only
    // render when a caller opts in with a handler prop.
    const handlerDriven = screen
      .queryAllByRole('button')
      .filter((b) => b.hasAttribute('data-control'))
    expect(handlerDriven).toHaveLength(0)
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
    render(<SidebarRow row={deletableRow} depth={0} onOpen={onOpen} onTrash={onTrash} />)
    screen.getByRole('button', { name: /delete/i }).click()
    expect(onTrash).toHaveBeenCalledWith('row-1')
    expect(onOpen).not.toHaveBeenCalled()
  })

  it('clicking the row body itself calls onOpen with the row id', () => {
    const onOpen = vi.fn()
    render(<SidebarRow row={baseRow} depth={0} onOpen={onOpen} />)
    screen.getByRole('treeitem').click()
    expect(onOpen).toHaveBeenCalledWith('row-1')
  })

  it('a protected branch row has no trash, even though onTrash is supplied', () => {
    // spec §9: "a protected branch is the repo's own ground … not workspaces
    // you made" — structurally the one row rows-from-repo.ts ever gives a
    // null parentId, the repo's own default worktree (kind 'branch',
    // parentId null). The population rule lives in SidebarRow itself, not in
    // whether a caller withholds the handler — SidebarTree always passes one.
    render(
      <SidebarRow
        row={{ ...baseRow, kind: 'branch', branchName: 'develop', ownsWorktree: true }}
        depth={0}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
      />,
    )
    expect(screen.queryByTestId('trash-control')).not.toBeInTheDocument()
  })

  // A chat's delete now routes to a real `deleteChat` call
  // (space-content-actions.ts's `handleTrash`), so its row carries the same
  // trash every other non-home row does.
  it('a chat row carries a trash, wired to the real delete', () => {
    render(<SidebarRow row={baseRow} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} />)
    expect(screen.getByTestId('trash-control')).toBeInTheDocument()
  })

  it('a chat row shows all three HANDLER-driven trailing controls, trash included', () => {
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
    expect(controls.map((c) => c.getAttribute('data-control'))).toEqual(['trash', 'create', 'fold'])
  })

  it('a non-home branch row still carries a trash', () => {
    render(
      <SidebarRow
        row={{
          ...baseRow,
          kind: 'branch',
          parentId: 'parent-1',
          branchName: 'my-feature',
          ownsWorktree: true,
        }}
        depth={0}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
      />,
    )
    expect(screen.getByTestId('trash-control')).toBeInTheDocument()
  })

  it('trash takes the deny tint on hover', () => {
    render(<SidebarRow row={deletableRow} depth={0} onOpen={vi.fn()} onTrash={vi.fn()} />)
    const trash = screen.getByTestId('trash-control')
    fireEvent.mouseEnter(trash)
    // The real token this codebase uses for a destructive/deny hover treatment
    // is Tailwind's `text-destructive` (backed by the `--destructive` CSS var
    // in theme.css) — there is no `--deny` token anywhere in web/src. The tint
    // is CSS-driven (a static `hover:` utility class), so the class is present
    // in the DOM regardless of the mouseenter simulation; the real activation
    // happens via the browser's own `:hover` match.
    expect(trash).toHaveClass('hover:text-destructive')
    expect(trash).toHaveClass('hover:bg-destructive/10')
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
})
