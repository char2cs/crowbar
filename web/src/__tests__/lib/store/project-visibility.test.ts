/**
 * Contract pin: which projects cost streams.
 *
 * Visibility is "every KNOWN project minus the folded ones, plus the active
 * one". Both halves matter. Open-by-default is the product decision — the
 * sidebar shows every project — and "known" (the always-on /v0/projects stream)
 * is what stops that decision from meaning "subscribe the world at boot": a
 * project only starts costing streams once its row actually exists.
 */
import { describe, it, expect, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { upsertEntity } from '@/lib/persistence/entity-cache'
import { idle, success } from '@/lib/loadable'
import { getVisibleProjectIds, readVisibleRepoTree } from '@/lib/store/project-visibility'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { FolderDTO, Project, RepoDTO, WorkspaceDTO } from '@/lib/types'

const repoDTO = (id: string, projectId: string): RepoDTO => ({
  id,
  projectId,
  name: id,
  path: `/p/${id}`,
  defaultBranch: 'main',
  avatarLabel: id[0].toUpperCase(),
  avatarColor: 'bg-indigo-700',
  avatarUrl: '',
  avatarEmoji: '',
})

const wsDTO = (id: string, repoId: string, projectId: string): WorkspaceDTO => ({
  id,
  repoId,
  projectId,
  branch: `feature/${id}`,
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
})

const folderDTO = (
  id: string,
  repoId: string,
  projectId: string,
  over: Partial<FolderDTO> = {},
): FolderDTO => ({
  id,
  repoId,
  projectId,
  name: id,
  order: 0,
  ...over,
})

const project = (id: string): Project => ({
  id,
  name: id,
  path: `/p/${id}`,
  lastActivity: new Date(0),
})

beforeEach(async () => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  useProjectStore.setState({ activeProjectId: 'p1' })
  useProjectDataStore.setState({ data: success([project('p1'), project('p2')]) })
  useSidebarStore.setState({ collapsedProjects: new Set<string>() })
  // Two projects, each with one repo and one workspace, in the cross-project cache.
  await upsertEntity('crowbar_repos', repoDTO('r1', 'p1'))
  await upsertEntity('crowbar_repos', repoDTO('r2', 'p2'))
  await upsertEntity('crowbar_workspaces', wsDTO('w1', 'r1', 'p1'))
  await upsertEntity('crowbar_workspaces', wsDTO('w2', 'r2', 'p2'))
  await upsertEntity('crowbar_folders', folderDTO('f1', 'r1', 'p1'))
})

describe('getVisibleProjectIds', () => {
  it('includes every known project — nothing has to be expanded first', () => {
    expect([...getVisibleProjectIds()].sort()).toEqual(['p1', 'p2'])
  })

  it('drops a project once it is folded away', () => {
    useSidebarStore.getState().toggleProject('p2')
    expect([...getVisibleProjectIds()]).toEqual(['p1'])
  })

  it('keeps the ACTIVE project visible even when its own row is folded', () => {
    // The app needs the active project's repo scope whether or not the section
    // is open; collapsing the row must not tear the live workspace's stream out
    // from under it.
    useSidebarStore.getState().toggleProject('p1')
    expect([...getVisibleProjectIds()].sort()).toEqual(['p1', 'p2'])
  })

  it('does not double-count the active project', () => {
    expect([...getVisibleProjectIds()].filter((id) => id === 'p1')).toHaveLength(1)
  })

  it('is empty before the project list has landed and with nothing active', () => {
    // "Known", not "existing": a project only costs streams once /v0/projects
    // has actually delivered its row.
    useProjectDataStore.setState({ data: idle() })
    useProjectStore.setState({ activeProjectId: '' })
    expect(getVisibleProjectIds().size).toBe(0)
  })

  it('is non-empty from the project list alone, with no active project', () => {
    useProjectStore.setState({ activeProjectId: '' })
    expect([...getVisibleProjectIds()].sort()).toEqual(['p1', 'p2'])
  })
})

describe('readVisibleRepoTree', () => {
  it('holds every visible project’s repos at once, each tagged with its project', async () => {
    const tree = await readVisibleRepoTree()
    expect(tree.map((r) => [r.id, r.projectId]).sort()).toEqual([
      ['r1', 'p1'],
      ['r2', 'p2'],
    ])
    // ...with their workspaces grouped under the right repo.
    expect(tree.find((r) => r.id === 'r2')!.workspaces.map((w) => w.id)).toEqual(['w2'])
  })

  it('carries the cached folders onto the repo that owns them', async () => {
    // The whole assembly path in one assertion: this is the ONLY place the tree
    // is built from the cache, so a folders read missing here is a folder that
    // never reaches the sidebar no matter what the daemon sent.
    const tree = await readVisibleRepoTree()
    expect(tree.find((r) => r.id === 'r1')!.folders?.map((f) => f.id)).toEqual(['f1'])
    // ...and only onto that repo — a folder is repo-scoped, never global.
    expect(tree.find((r) => r.id === 'r2')!.folders).toBeUndefined()
  })

  it('keeps a folder’s parentId and order, which decide where the row lands', async () => {
    await upsertEntity(
      'crowbar_folders',
      folderDTO('f2', 'r1', 'p1', { parentId: 'f1', name: 'nested', order: 3 }),
    )
    const folders = (await readVisibleRepoTree()).find((r) => r.id === 'r1')!.folders!
    const nested = folders.find((f) => f.id === 'f2')!
    expect(nested).toMatchObject({ parentId: 'f1', name: 'nested', order: 3 })
    // A root folder's empty parentId becomes undefined, which is what
    // buildSidebarTree reads as "hangs off the repo".
    expect(folders.find((f) => f.id === 'f1')!.parentId).toBeUndefined()
  })

  it('excludes a project that has been folded away', async () => {
    useSidebarStore.getState().toggleProject('p2')
    expect((await readVisibleRepoTree()).map((r) => r.id)).toEqual(['r1'])
  })

  it('brings a project’s repos back when it is re-opened', async () => {
    useSidebarStore.getState().toggleProject('p2')
    expect((await readVisibleRepoTree()).map((r) => r.id)).toEqual(['r1'])
    useSidebarStore.getState().toggleProject('p2')
    expect((await readVisibleRepoTree()).map((r) => r.id).sort()).toEqual(['r1', 'r2'])
  })

  it('renders a non-active project from the cache alone — no fetch involved', async () => {
    // The rows come straight out of IndexedDB, which is what makes expanding a
    // project instant and offline-capable.
    useProjectStore.setState({ activeProjectId: '' })
    useSidebarStore.getState().toggleProject('p1')
    const tree = await readVisibleRepoTree()
    expect(tree.map((r) => r.id)).toEqual(['r2'])
    expect(tree[0].workspaces.map((w) => w.id)).toEqual(['w2'])
  })

  it('yields an empty tree when no project is visible', async () => {
    useProjectDataStore.setState({ data: idle() })
    useProjectStore.setState({ activeProjectId: '' })
    expect(await readVisibleRepoTree()).toEqual([])
  })

  it('returns the SAME empty array reference each time (stable snapshot)', async () => {
    // An inline `[]` fallback makes Zustand's snapshot compare unstable and
    // React eventually throws "Maximum update depth exceeded" elsewhere.
    useProjectDataStore.setState({ data: idle() })
    useProjectStore.setState({ activeProjectId: '' })
    expect(await readVisibleRepoTree()).toBe(await readVisibleRepoTree())
  })
})
