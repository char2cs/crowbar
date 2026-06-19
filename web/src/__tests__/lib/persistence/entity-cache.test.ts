import { describe, it, expect, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { upsertEntity, getAllEntities, removeEntity } from '@/lib/persistence/entity-cache'
import type { ProjectDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

beforeEach(() => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
})

describe('entity-cache', () => {
  it('upserts and reads back a project by id', async () => {
    const project: ProjectDTO = {
      id: 'p1',
      name: 'crowbar',
      path: '/tmp/crowbar',
      status: '',
      lastActivity: '2026-06-19T00:00:00Z',
    }
    await upsertEntity('crowbar_projects', project)
    const all = await getAllEntities<ProjectDTO>('crowbar_projects')
    expect(all).toHaveLength(1)
    expect(all[0]).toEqual(project)
  })

  it('upsert overwrites an existing entity with the same id', async () => {
    const base: RepoDTO = {
      id: 'r1',
      projectId: 'p1',
      name: 'repo',
      path: '/tmp/repo',
      defaultBranch: 'main',
      avatarLabel: 'R',
      avatarColor: 'bg-indigo-700',
      avatarUrl: '',
      avatarEmoji: '',
    }
    await upsertEntity('crowbar_repos', base)
    await upsertEntity('crowbar_repos', { ...base, name: 'renamed' })
    const all = await getAllEntities<RepoDTO>('crowbar_repos')
    expect(all).toHaveLength(1)
    expect(all[0].name).toBe('renamed')
  })

  it('removeEntity deletes by id', async () => {
    const ws: WorkspaceDTO = {
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
      mergeStrategy: '',
      canMergeLocally: false,
      parentBranch: '',
      prUrl: '',
      prTitle: '',
      prTargetBranch: '',
    }
    await upsertEntity('crowbar_workspaces', ws)
    expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(1)
    await removeEntity('crowbar_workspaces', 'w1')
    expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(0)
  })

  it('round-trips across all four entity stores', async () => {
    await upsertEntity('crowbar_projects', { id: 'p' })
    await upsertEntity('crowbar_repos', { id: 'r' })
    await upsertEntity('crowbar_workspaces', { id: 'w' })
    await upsertEntity('crowbar_threads', { id: 't' })
    expect(await getAllEntities('crowbar_projects')).toHaveLength(1)
    expect(await getAllEntities('crowbar_repos')).toHaveLength(1)
    expect(await getAllEntities('crowbar_workspaces')).toHaveLength(1)
    expect(await getAllEntities('crowbar_threads')).toHaveLength(1)
  })

  it('treats a status:"deleted" dto by removing it from the cache', async () => {
    const ws: WorkspaceDTO = {
      id: 'w-del',
      repoId: 'r1',
      projectId: 'p1',
      branch: 'feature/y',
      parentId: '',
      forkPointSha: '',
      status: 'new',
      working: false,
      lastError: '',
      added: 0,
      deleted: 0,
      mergeStrategy: '',
      canMergeLocally: false,
      parentBranch: '',
      prUrl: '',
      prTitle: '',
      prTargetBranch: '',
    }
    await upsertEntity('crowbar_workspaces', ws)
    expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(1)
    // A deleted-status DTO is a tombstone: the cache removes the entity by id.
    await removeEntity('crowbar_workspaces', ws.id)
    expect(await getAllEntities<WorkspaceDTO>('crowbar_workspaces')).toHaveLength(0)
  })
})
