import { describe, expect, it } from 'vitest'
import { isWorkspaceLockedInSidebar } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'

// The file explorer gates its mutation menu items on this lookup. A workspace
// lives in one of TWO id spaces: the repo's tree rows (repo.workspaces) or the
// default (main-worktree) workspace, which is never a tree row — only
// repo.defaultWorkspaceId + repo.defaultWorkspaceStatus. Adopted protected
// branches are locked AND default, so missing the second space un-gated every
// repo home (the original review finding).
function makeRepo(over: Partial<Repo> = {}): Repo {
  return {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...over,
  }
}

describe('isWorkspaceLockedInSidebar', () => {
  it('reports a locked tree-row workspace', () => {
    const repos = [
      makeRepo({
        workspaces: [
          { id: 'ws-develop', branch: 'develop', status: 'locked', age: '' },
          { id: 'ws-feature', branch: 'feature/x', status: 'new', age: '' },
        ],
      }),
    ]
    expect(isWorkspaceLockedInSidebar(repos, 'ws-develop')).toBe(true)
    expect(isWorkspaceLockedInSidebar(repos, 'ws-feature')).toBe(false)
  })

  // The review finding: a locked DEFAULT workspace (adoptMainWorktree creates
  // Protected → status 'locked' + IsDefault) is not a tree row — the lookup must
  // resolve it via repo.defaultWorkspaceId + defaultWorkspaceStatus.
  it('reports a locked default (main-worktree) workspace', () => {
    const repos = [
      makeRepo({
        defaultWorkspaceId: 'ws-default',
        defaultBranch: 'develop',
        defaultWorkspaceStatus: 'locked',
        workspaces: [{ id: 'ws-child', branch: 'feature/x', status: 'new', age: '' }],
      }),
    ]
    expect(isWorkspaceLockedInSidebar(repos, 'ws-default')).toBe(true)
  })

  it('reports an unlocked default workspace as unlocked', () => {
    const repos = [
      makeRepo({
        defaultWorkspaceId: 'ws-default',
        defaultBranch: 'main',
        defaultWorkspaceStatus: 'new',
      }),
    ]
    expect(isWorkspaceLockedInSidebar(repos, 'ws-default')).toBe(false)
  })

  it('is false for an unknown or null workspace id', () => {
    const repos = [
      makeRepo({
        defaultWorkspaceId: 'ws-default',
        defaultWorkspaceStatus: 'locked',
        workspaces: [{ id: 'ws-a', branch: 'a', status: 'locked', age: '' }],
      }),
    ]
    expect(isWorkspaceLockedInSidebar(repos, 'ws-unknown')).toBe(false)
    expect(isWorkspaceLockedInSidebar(repos, null)).toBe(false)
  })
})
