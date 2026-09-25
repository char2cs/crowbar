import { expect, test } from 'vitest'
import {
  shouldRedirectUnknownWorkspace,
  type WorkspaceRouteKnowledge,
} from '@/lib/store/workspace-route-guard'
import type { Repo } from '@/lib/store/sidebar'

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

const NONE: ReadonlySet<string> = new Set()
const READ: WorkspaceRouteKnowledge = {
  repos: REPOS,
  workspacesRead: new Set(['repo-1']),
  reposRead: new Set(['p1']),
}

const route = (wsId: string | undefined, repoId = 'repo-1') => ({ projectId: 'p1', repoId, wsId })

// A deep link on a cold cache: the tree is empty only because nothing has
// been read from the daemon yet, which says nothing about the workspace.
test('does not redirect before the daemon has answered for the route', () => {
  const cold: WorkspaceRouteKnowledge = { repos: [], workspacesRead: NONE, reposRead: NONE }
  expect(shouldRedirectUnknownWorkspace(route('ws-base'), cold)).toBe(false)
  const reposOnly: WorkspaceRouteKnowledge = { ...READ, workspacesRead: NONE }
  expect(shouldRedirectUnknownWorkspace(route('ghost'), reposOnly)).toBe(false)
})

test('does not redirect when the workspace exists', () => {
  expect(shouldRedirectUnknownWorkspace(route('ws-base'), READ)).toBe(false)
  expect(shouldRedirectUnknownWorkspace(route('ws-child'), READ)).toBe(false)
})

test('redirects once the repo workspace list is read and the id is not in it', () => {
  expect(shouldRedirectUnknownWorkspace(route('ghost'), READ)).toBe(true)
})

test('redirects once the project repo list is read and the repo is not in it', () => {
  expect(shouldRedirectUnknownWorkspace(route('ghost', 'repo-gone'), READ)).toBe(true)
  const unread: WorkspaceRouteKnowledge = { ...READ, reposRead: NONE }
  expect(shouldRedirectUnknownWorkspace(route('ghost', 'repo-gone'), unread)).toBe(false)
})

test('no id means nothing to guard', () => {
  expect(shouldRedirectUnknownWorkspace(route(undefined), READ)).toBe(false)
})

test('does not redirect for the repo default workspace id (excluded from the tree)', () => {
  expect(shouldRedirectUnknownWorkspace(route('ws-default'), READ)).toBe(false)
})
