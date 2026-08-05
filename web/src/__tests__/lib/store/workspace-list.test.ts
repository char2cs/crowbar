import { describe, it, expect, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { dataOf, idle, success } from '@/lib/loadable'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Project, RepoDTO, WorkspaceDTO } from '@/lib/types'

const project = (id: string): Project => ({
  id,
  name: id,
  path: `/p/${id}`,
  lastActivity: new Date(0),
})

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
  // The entity cache is cross-project; the sidebar tree is scoped to the VISIBLE
  // projects. Make one active (always visible) so the tree isn't empty, start
  // from nothing collapsed, and start with the project list still unlanded so
  // each test says explicitly which projects are KNOWN.
  useProjectStore.setState({ activeProjectId: 'p1' })
  useProjectDataStore.setState({ data: idle() })
  useSidebarStore.setState({ collapsedProjects: new Set<string>() })
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
  // project (each project's repo stream prunes only its own scope, and rows
  // survive across sessions so an expand is instant), so the sidebar tree must
  // scope to the VISIBLE projects. Before this fix, switching to project p2
  // still showed p1's repos because the tree was built from the whole cache.
  it('fetch excludes a project the project list has not delivered yet', async () => {
    await seedTwoProjects()

    useProjectStore.setState({ activeProjectId: 'p2' })
    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()

    const repos = dataOf(useWorkspaceListStore.getState().data)!
    expect(repos.map((r) => r.id)).toEqual(['r2'])
  })

  // The sidebar shows every project at once now: once the project list has
  // landed, every project's repos belong in the tree without anything being
  // expanded first.
  it('fetch includes every known project, not just the active one', async () => {
    await seedTwoProjects()

    useProjectStore.setState({ activeProjectId: 'p1' })
    useProjectDataStore.setState({ data: success([project('p1'), project('p2')]) })
    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()

    const repos = dataOf(useWorkspaceListStore.getState().data)!
    expect(repos.map((r) => r.id).sort()).toEqual(['r1', 'r2'])
    expect(repos.map((r) => r.projectId).sort()).toEqual(['p1', 'p2'])
  })

  it('fetch drops a project once it is folded away', async () => {
    await seedTwoProjects()

    useProjectStore.setState({ activeProjectId: 'p1' })
    useProjectDataStore.setState({ data: success([project('p1'), project('p2')]) })
    useSidebarStore.getState().toggleProject('p2')
    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()

    const repos = dataOf(useWorkspaceListStore.getState().data)!
    expect(repos.map((r) => r.id)).toEqual(['r1'])
  })

  it('fetch yields an empty tree when no project is visible', async () => {
    await upsertEntity('crowbar_repos', repoDTO)
    useProjectStore.setState({ activeProjectId: '' })

    const { useWorkspaceListStore } = await import('@/lib/store/workspace-list')
    await useWorkspaceListStore.getState().fetch()

    expect(dataOf(useWorkspaceListStore.getState().data)).toEqual([])
  })
})

async function seedTwoProjects(): Promise<void> {
  await upsertEntity('crowbar_repos', repoDTO) // r1 in p1
  await upsertEntity('crowbar_workspaces', wsDTO) // w1 in r1/p1
  await upsertEntity('crowbar_repos', { ...repoDTO, id: 'r2', projectId: 'p2', name: 'other' })
  await upsertEntity('crowbar_workspaces', { ...wsDTO, id: 'w2', repoId: 'r2', projectId: 'p2' })
}
