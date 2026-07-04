import { beforeEach, expect, test } from 'vitest'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'
import type { WorkspaceDTO } from '@/lib/types'
import {
  getWorkspaceScope,
  setWorkspaceScope,
  __resetWorkspaceScopesForTest,
} from '@/lib/workspace-scope'

// Regression: the placeholder row's Retry/Detach… actions call workspaceBase(),
// which throws when the workspace's scope was never recorded. Scopes used to be
// recorded ONLY by visiting the /ide/:p/:r/:wsId route, so acting on a workspace
// you had never navigated to (a placeholder can't be provisioned, so users
// rarely visit it) threw before the request was ever sent — the Detach button
// silently did nothing. The sidebar store now records every workspace's scope
// as its data arrives.

const REPOS: Repo[] = [
  {
    id: 'repo-1',
    projectId: 'proj-1',
    name: 'quiver.core',
    avatarLabel: 'Q',
    avatarColor: 'avatar-rose',
    defaultWorkspaceId: 'ws-home',
    workspaces: [
      { id: 'ws-main', branch: 'main', status: 'locked', age: '—' },
      { id: 'ws-placeholder', branch: 'develop', status: 'locked', age: '—', heldByPath: '/x' },
    ],
  },
]

beforeEach(() => {
  useSidebarStore.setState(getInitialState())
  __resetWorkspaceScopesForTest()
})

test('setRepos records a scope for every workspace, including the default', () => {
  useSidebarStore.getState().setRepos(REPOS)
  expect(getWorkspaceScope('ws-placeholder')).toEqual({
    projectId: 'proj-1',
    repoId: 'repo-1',
    wsId: 'ws-placeholder',
  })
  expect(getWorkspaceScope('ws-main')).not.toBeNull()
  expect(getWorkspaceScope('ws-home')).toEqual({
    projectId: 'proj-1',
    repoId: 'repo-1',
    wsId: 'ws-home',
  })
})

test('recording scopes from sidebar data does not steal the active workspace', () => {
  setWorkspaceScope({ projectId: 'proj-1', repoId: 'repo-1', wsId: 'ws-active' })
  useSidebarStore.getState().setRepos(REPOS)
  // getWorkspaceScope() with no id resolves the ACTIVE workspace — it must
  // still be the route-recorded one, not whatever the sidebar loaded last.
  expect(getWorkspaceScope()?.wsId).toBe('ws-active')
})

test('setRepos skips repos with no projectId (no URL can be built anyway)', () => {
  useSidebarStore.getState().setRepos([{ ...REPOS[0], projectId: undefined }])
  expect(getWorkspaceScope('ws-placeholder')).toBeNull()
})

test('mergeRepos records scopes for appended repos and workspaces', () => {
  useSidebarStore.getState().setRepos(REPOS)
  useSidebarStore.getState().mergeRepos([
    {
      ...REPOS[0],
      workspaces: [...REPOS[0].workspaces, { id: 'ws-new', branch: 'feat/x', age: '—' }],
    },
  ])
  expect(getWorkspaceScope('ws-new')).toEqual({
    projectId: 'proj-1',
    repoId: 'repo-1',
    wsId: 'ws-new',
  })
})

test('applyWorkspaceDTO records the scope of an upserted workspace', () => {
  useSidebarStore.getState().setRepos(REPOS)
  const dto: WorkspaceDTO = {
    id: 'ws-dto',
    repoId: 'repo-1',
    projectId: 'proj-1',
    branch: 'feat/y',
    parentId: '',
    forkPointSha: '',
    status: 'new',
    working: false,
    lastError: '',
    added: 0,
    deleted: 0,
    mergeStrategy: 'merge',
    canMergeLocally: false,
    mergeConflicts: false,
    parentBranch: '',
    prUrl: '',
    prTitle: '',
    prTargetBranch: '',
  }
  useSidebarStore.getState().applyWorkspaceDTO(dto)
  expect(getWorkspaceScope('ws-dto')).toEqual({
    projectId: 'proj-1',
    repoId: 'repo-1',
    wsId: 'ws-dto',
  })
})
