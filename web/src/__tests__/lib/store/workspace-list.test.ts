import { describe, it, expect, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

const repoDTO: RepoDTO = {
  id: 'r1',
  projectId: 'p1',
  name: 'repo',
  path: '/repo',
  defaultBranch: 'main',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  avatarUrl: '',
  avatarEmoji: '',
}

const wsDTO: WorkspaceDTO = {
  id: 'w1',
  repoId: 'r1',
  projectId: 'p1',
  branch: 'feature/x',
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

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
})

describe('useWorkspaceListStore', () => {
  it('fetch builds the nested repo tree from the WS-driven entity cache', async () => {
    await upsertEntity('crowbar_repos', repoDTO)
    await upsertEntity('crowbar_workspaces', wsDTO)

    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()

    const repos = dataOf(useWorkspaceListStore.getState().data)!
    expect(repos).toHaveLength(1)
    expect(repos[0].id).toBe('r1')
    expect(repos[0].projectId).toBe('p1')
    expect(repos[0].workspaces.map((w) => w.id)).toEqual(['w1'])
  })

  it('fetch yields an empty tree when the entity cache is empty', async () => {
    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()
    expect(dataOf(useWorkspaceListStore.getState().data)).toEqual([])
  })
})
