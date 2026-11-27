import { describe, expect, it, test } from 'vitest'
import {
  getPostDeleteNavigationTarget,
  isWorkspaceLockedInSidebar,
} from '@/core/entities/repo-queries'
import type { Repo } from '@/core/entities/repo-types'

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

function repoWith(workspaces: Repo['workspaces']): Repo[] {
  return [
    {
      id: 'repo-1',
      name: 'crowbar',
      avatarLabel: 'C',
      avatarColor: 'bg-indigo-700',
      workspaces,
    },
  ]
}

test('targets the parent workspace when it survives the delete', () => {
  const repos = repoWith([
    { id: 'base', branch: 'develop', status: 'locked', age: '—' },
    { id: 'parent', branch: 'feat/parent', parentId: 'base', status: 'new', age: '—' },
    { id: 'child', branch: 'feat/child', parentId: 'parent', status: 'new', age: '—' },
  ])
  expect(getPostDeleteNavigationTarget(repos, 'child')).toBe('parent')
})

test('falls back to the repo base workspace for a root-level workspace', () => {
  const repos = repoWith([
    { id: 'base', branch: 'develop', status: 'locked', age: '—' },
    { id: 'root-ws', branch: 'feat/root', status: 'new', age: '—' },
  ])
  expect(getPostDeleteNavigationTarget(repos, 'root-ws')).toBe('base')
})

test('descendants of the deleted workspace are not navigation targets', () => {
  const repos = repoWith([
    { id: 'base', branch: 'develop', status: 'locked', age: '—' },
    { id: 'parent', branch: 'feat/parent', parentId: 'base', status: 'new', age: '—' },
    { id: 'child', branch: 'feat/child', parentId: 'parent', status: 'new', age: '—' },
  ])
  expect(getPostDeleteNavigationTarget(repos, 'parent')).toBe('base')
})

test('locked descendants survive the delete and can be targets', () => {
  const repos = repoWith([
    { id: 'parent', branch: 'feat/parent', status: 'new', age: '—' },
    { id: 'locked-child', branch: 'develop', parentId: 'parent', status: 'locked', age: '—' },
  ])
  expect(getPostDeleteNavigationTarget(repos, 'parent')).toBe('locked-child')
})

test('returns null when no workspace in the repo survives', () => {
  const repos = repoWith([{ id: 'only', branch: 'feat/only', status: 'new', age: '—' }])
  expect(getPostDeleteNavigationTarget(repos, 'only')).toBeNull()
})

test('returns null for an unknown workspace id', () => {
  expect(getPostDeleteNavigationTarget(repoWith([]), 'ghost')).toBeNull()
})
