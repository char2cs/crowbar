import { describe, it, expect, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf } from '@/lib/loadable'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import { useProjectStore } from '@/lib/store/projects'
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
  // The entity cache is cross-project; the sidebar tree is scoped to the active
  // project. Set one so the tree isn't empty.
  useProjectStore.setState({ activeProjectId: 'p1' })
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

  // Regression: the entity cache deliberately accumulates repos from EVERY
  // project (each project's repo stream prunes only its own scope), so the
  // sidebar tree must filter to the active project. Before this fix, switching
  // to project p2 still showed p1's repos because the tree was built from the
  // whole cache.
  it('fetch scopes the tree to the active project, excluding other projects repos', async () => {
    await upsertEntity('crowbar_repos', repoDTO) // r1 in p1
    await upsertEntity('crowbar_workspaces', wsDTO) // w1 in r1/p1
    await upsertEntity('crowbar_repos', {
      ...repoDTO,
      id: 'r2',
      projectId: 'p2',
      name: 'other',
    })
    await upsertEntity('crowbar_workspaces', { ...wsDTO, id: 'w2', repoId: 'r2', projectId: 'p2' })

    useProjectStore.setState({ activeProjectId: 'p2' })
    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()

    const repos = dataOf(useWorkspaceListStore.getState().data)!
    expect(repos.map((r) => r.id)).toEqual(['r2'])
  })

  it('fetch yields an empty tree when there is no active project', async () => {
    await upsertEntity('crowbar_repos', repoDTO)
    useProjectStore.setState({ activeProjectId: '' })

    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()

    expect(dataOf(useWorkspaceListStore.getState().data)).toEqual([])
  })
})
