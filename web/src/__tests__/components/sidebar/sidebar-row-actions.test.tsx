import { describe, expect, it, vi } from 'vitest'
import { render } from '@testing-library/react'
import { SidebarRowActions } from '@/components/sidebar/sidebar-row-actions'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

const provisioned: SidebarRowType = {
  id: 'chat-owning-ws-1',
  kind: 'branch',
  parentId: 'repo-home-1',
  order: 0,
  label: 'main',
  ownsWorktree: true,
  workspaceId: 'ws-1',
  working: false,
  hasView: false,
  branchName: 'main',
  locked: true,
  status: 'locked',
  isPlaceholder: false,
}

function controlsOf(row: SidebarRowType, isProjectHome = false): string[] {
  const { container } = render(
    <SidebarRowActions
      row={row}
      isProjectHome={isProjectHome}
      expanded={false}
      subActionClass=""
      onCreate={vi.fn()}
      onTrash={vi.fn()}
      onToggleFold={vi.fn()}
    />,
  )
  return Array.from(container.querySelectorAll('[data-control]')).map(
    (el) => el.getAttribute('data-control') ?? '',
  )
}

describe('SidebarRowActions', () => {
  it('offers Thread and Fork on a branch row whose workspace is provisioned', () => {
    expect(controlsOf(provisioned)).toEqual(['thread', 'fork', 'fold'])
  })

  // Caught live: the amber "Branch needs provisioning" row still offered both
  // verbs. A placeholder workspace has no worktree on disk (`localPath: null`,
  // lib/workspace/placeholder.ts), so the chat either verb spawns has nowhere
  // to run — the daemon answered 201 and the new chat then 500'd on every read
  // ("catalog worktree is invalid") with no error anywhere in the UI.
  it('offers neither Thread nor Fork on a placeholder branch row', () => {
    const controls = controlsOf({ ...provisioned, isPlaceholder: true })
    expect(controls).not.toContain('thread')
    expect(controls).not.toContain('fork')
  })

  // The gate is on the two verbs that need a worktree, not on the row: a
  // placeholder still carries its own remedy (Retry) and stays removable and
  // foldable.
  it('leaves a placeholder branch row its remedy, remove and fold', () => {
    const controls = controlsOf({ ...provisioned, locked: false, isPlaceholder: true })
    expect(controls).toEqual(['retry-provision', 'remove', 'fold'])
  })

  // Explicit product spec: repo-menu, Remove, Thread, Branch, Dropdown — left
  // to right. An ordinary row never gets repo-menu, so Remove leads.
  it('orders Remove before Thread and Fork on an ordinary unlocked branch row', () => {
    const controls = controlsOf({ ...provisioned, locked: false })
    expect(controls).toEqual(['remove', 'thread', 'fork', 'fold'])
  })

  // The one row kind that DOES get repo-menu folds Remove into it ("Delete
  // Repo" in row-context-menu.tsx) instead of drawing a second, standalone
  // delete affordance.
  it('shows repo-menu instead of a standalone Remove on a project-home row', () => {
    const controls = controlsOf(
      {
        ...provisioned,
        locked: false,
        repoIcon: {
          repoId: 'repo-1',
          projectId: 'project-1',
          name: 'repo',
          avatarLabel: 'R',
          avatarColor: '#000',
        },
      },
      true,
    )
    expect(controls).toEqual(['repo-menu', 'thread', 'fork', 'fold'])
    expect(controls).not.toContain('remove')
  })
})
