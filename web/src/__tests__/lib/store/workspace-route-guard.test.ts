import { expect, test } from 'vitest'
import type { Operation } from '@/core/entities/operations'
import {
  mergeRunsOn,
  shouldRedirectUnknownWorkspace,
  type WorkspaceRouteKnowledge,
} from '@/lib/store/workspace-route-guard'
import type { Repo } from '@/core/entities/repo-types'

const REPOS: Repo[] = [
  {
    id: 'repo-1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    defaultWorkspaceId: 'ws-default',
    workspaces: [
      { id: 'ws-base', branch: 'develop', status: 'locked', age: '—' },
      { id: 'ws-child', branch: 'feature/x', parentId: 'ws-base', status: 'new', age: '—' },
    ],
  },
]

const READ: WorkspaceRouteKnowledge = { repos: REPOS, synced: true, merging: false }

const route = (wsId: string | undefined, repoId = 'repo-1') => ({ projectId: 'p1', repoId, wsId })

// A deep link on a cold cache: the tree is empty only because nothing has
// been read from the daemon yet, which says nothing about the workspace.
test('does not redirect before the daemon has answered for the route', () => {
  const cold: WorkspaceRouteKnowledge = { ...READ, repos: [], synced: false }
  expect(shouldRedirectUnknownWorkspace(route('ws-base'), cold)).toBe(false)
  expect(shouldRedirectUnknownWorkspace(route('ghost'), { ...READ, synced: false })).toBe(false)
})

test('does not redirect when the workspace exists', () => {
  expect(shouldRedirectUnknownWorkspace(route('ws-base'), READ)).toBe(false)
  expect(shouldRedirectUnknownWorkspace(route('ws-child'), READ)).toBe(false)
})

test('redirects once the daemon has answered and the id is not in the tree', () => {
  expect(shouldRedirectUnknownWorkspace(route('ghost'), READ)).toBe(true)
  expect(shouldRedirectUnknownWorkspace(route('ghost', 'repo-gone'), READ)).toBe(true)
})

test('no id means nothing to guard', () => {
  expect(shouldRedirectUnknownWorkspace(route(undefined), READ)).toBe(false)
})

test('does not redirect for the repo default workspace id (excluded from the tree)', () => {
  expect(shouldRedirectUnknownWorkspace(route('ws-default'), READ)).toBe(false)
})

// A merge's fold removes the child before its operation settles; the merge
// moves the user to the parent, and a guard redirect home would override it.
test('does not redirect while a merge folds the workspace', () => {
  expect(shouldRedirectUnknownWorkspace(route('ghost'), { ...READ, merging: true })).toBe(false)
})

const op = (kind: string, state: Operation['state']): Operation => ({
  ...{ id: kind, kind, subject: { kind: 'workspace', id: 'ws-child' }, projectId: 'p1' },
  ...{ label: kind, state, items: [], dismissed: false, version: 1, createdAt: '', updatedAt: '' },
})

test('only a running merge into the parent holds the guard, not other work on the workspace', () => {
  const push = op('git.push', 'running')
  const merge = op('workspace.merge_into_parent', 'running')
  expect(mergeRunsOn({ [push.id]: push }, 'ws-child')).toBe(false)
  expect(mergeRunsOn({ [merge.id]: merge }, 'ws-child')).toBe(true)
  expect(mergeRunsOn({ [merge.id]: { ...merge, state: 'succeeded' } }, 'ws-child')).toBe(false)
  expect(mergeRunsOn({ [merge.id]: merge }, 'ws-base')).toBe(false)
})
