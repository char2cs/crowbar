import { describe, it, expect, beforeEach } from 'vitest'
import { IDBFactory } from 'fake-indexeddb'
import { resetDB } from '@/lib/persistence/idb'
import { getAllEntities, upsertEntity } from '@/lib/persistence/entity-cache'
import {
  applyChatPlacement,
  applyFolderPlacements,
  applyRepoPlacements,
  applyWorkspacePlacement,
  applyWorkspacePlacements,
} from '@/lib/store/applied-placement'
import { toSidebarChat, toSidebarFolder, toSidebarRepo } from '@/lib/store/build-repo-tree'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useSidebarStore, getInitialState } from '@/lib/store/sidebar'
import type { ChatDTO, FolderDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

const repoDTO = (id: string, order: number): RepoDTO => ({
  id,
  projectId: 'p1',
  name: id,
  path: `/p/${id}`,
  defaultBranch: 'main',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  avatarUrl: '',
  avatarEmoji: '',
  order,
})

const chatDTO = (id: string, order: number, parentId = ''): ChatDTO => ({
  id,
  repoId: 'r1',
  projectId: 'p1',
  type: 'chat',
  workspaceId: 'ws-1',
  ownsWorktree: false,
  parentId,
  title: id,
  order,
})

const folderDTO = (id: string, order: number, parentId = ''): FolderDTO => ({
  id,
  repoId: 'r1',
  projectId: 'p1',
  parentId,
  name: id,
  order,
})

const byId = <T extends { id: string }>(rows: T[], id: string) => rows.find((r) => r.id === id)

beforeEach(async () => {
  resetDB()
  globalThis.indexedDB = new IDBFactory()
  useFolderSignalStore.setState({ generations: {} })
  useSidebarStore.setState(getInitialState())
  await upsertEntity('crowbar_repos', repoDTO('r1', 0))
  await upsertEntity('crowbar_chats', chatDTO('c1', 0))
  await upsertEntity('crowbar_chats', chatDTO('c2', 1))
  await upsertEntity('crowbar_folders', folderDTO('f1', 2))
  useSidebarStore
    .getState()
    .setRepos([
      toSidebarRepo(
        repoDTO('r1', 0),
        [],
        [toSidebarFolder(folderDTO('f1', 2))],
        [toSidebarChat(chatDTO('c1', 0)), toSidebarChat(chatDTO('c2', 1))],
      ),
    ])
})

describe('applyChatPlacement', () => {
  it('writes the moved chat and its shifted siblings — chats AND folders — through to the cache before the store, then bumps', async () => {
    const moved = await applyChatPlacement({ id: 'c1', parentId: 'f1', order: 0 }, [
      { id: 'c2', parentId: '', order: 0 },
      { id: 'f1', parentId: '', order: 1 },
    ])
    expect(moved).toBe('r1')

    const chats = await getAllEntities<ChatDTO>('crowbar_chats')
    expect(byId(chats, 'c1')).toEqual(chatDTO('c1', 0, 'f1'))
    expect(byId(chats, 'c2')).toEqual(chatDTO('c2', 0))
    const folders = await getAllEntities<FolderDTO>('crowbar_folders')
    expect(byId(folders, 'f1')).toEqual(folderDTO('f1', 1))

    const repo = useSidebarStore.getState().repos[0]
    expect(byId(repo.chats ?? [], 'c1')).toMatchObject({ parentId: 'f1', order: 0 })
    expect(byId(repo.chats ?? [], 'c2')).toMatchObject({ parentId: undefined, order: 0 })
    expect(byId(repo.folders ?? [], 'f1')).toMatchObject({ parentId: undefined, order: 1 })
    expect(useFolderSignalStore.getState().generations['r1']).toBe(1)
  })

  it('still applies to the store, without a bump target, when no repo holds the chat', async () => {
    const moved = await applyChatPlacement({ id: 'ghost', parentId: '', order: 3 })
    expect(moved).toBeNull()
    expect(useFolderSignalStore.getState().generations['r1']).toBeUndefined()
  })
})

describe('applyFolderPlacements', () => {
  it('replaces the cached folder rows, applies them to the store and bumps the repo', async () => {
    await applyFolderPlacements('r1', [folderDTO('f1', 0), folderDTO('f2', 1)])

    const folders = await getAllEntities<FolderDTO>('crowbar_folders')
    expect(byId(folders, 'f1')).toEqual(folderDTO('f1', 0))
    expect(byId(folders, 'f2')).toEqual(folderDTO('f2', 1))
    const repo = useSidebarStore.getState().repos[0]
    expect((repo.folders ?? []).map((f) => [f.id, f.order])).toEqual([
      ['f1', 0],
      ['f2', 1],
    ])
    expect(useFolderSignalStore.getState().generations['r1']).toBe(1)
  })
})

const workspaceDTO = (id: string, order: number, folderId = ''): WorkspaceDTO => ({
  id,
  repoId: 'r1',
  projectId: 'p1',
  branch: id,
  parentId: '',
  forkPointSha: '',
  status: 'new',
  working: false,
  lastError: '',
  added: 0,
  deleted: 0,
  mergeStrategy: '',
  canMergeLocally: false,
  mergeConflicts: false,
  parentBranch: '',
  prUrl: '',
  prTitle: '',
  prTargetBranch: '',
  folderId,
  order,
})

describe('applyWorkspacePlacement', () => {
  beforeEach(async () => {
    await upsertEntity('crowbar_workspaces', workspaceDTO('ws-a', 0))
    await upsertEntity('crowbar_workspaces', workspaceDTO('ws-b', 1))
    useSidebarStore
      .getState()
      .setRepos([
        toSidebarRepo(
          repoDTO('r1', 0),
          [workspaceDTO('ws-a', 0), workspaceDTO('ws-b', 1)],
          [toSidebarFolder(folderDTO('f1', 2))],
          [],
        ),
      ])
  })

  it('writes the moved workspace through the cache, applies it and its shifted folder to the store, then bumps', async () => {
    await applyWorkspacePlacement('ws-b', { parentId: '', order: 0 }, [
      { id: 'f1', parentId: '', order: 3 },
    ])

    const cached = await getAllEntities<WorkspaceDTO>('crowbar_workspaces')
    expect(byId(cached, 'ws-b')).toEqual(workspaceDTO('ws-b', 0))
    const repo = useSidebarStore.getState().repos[0]
    expect(byId(repo.workspaces, 'ws-b')).toMatchObject({ folderId: '', order: 0 })
    expect(byId(repo.folders ?? [], 'f1')).toMatchObject({ order: 3 })
    expect(useFolderSignalStore.getState().generations['r1']).toBe(1)
  })

  it('still applies a shifted folder the cache has never seen', async () => {
    await applyWorkspacePlacement('ws-b', { parentId: 'f1', order: 0 }, [
      { id: 'f-cold', parentId: '', order: 5 },
    ])
    expect(byId(useSidebarStore.getState().repos[0].workspaces, 'ws-b')).toMatchObject({
      folderId: 'f1',
      order: 0,
    })
  })
})

describe('applyWorkspacePlacements', () => {
  it('replaces the cached workspace rows before merging their placement into the store', async () => {
    useSidebarStore
      .getState()
      .setRepos([
        toSidebarRepo(repoDTO('r1', 0), [workspaceDTO('ws-a', 0), workspaceDTO('ws-b', 1)]),
      ])

    await applyWorkspacePlacements([workspaceDTO('ws-a', 1, 'f1'), workspaceDTO('ws-b', 0)])

    const cached = await getAllEntities<WorkspaceDTO>('crowbar_workspaces')
    expect(byId(cached, 'ws-a')).toEqual(workspaceDTO('ws-a', 1, 'f1'))
    const repo = useSidebarStore.getState().repos[0]
    expect(byId(repo.workspaces, 'ws-a')).toMatchObject({ folderId: 'f1', order: 1 })
    expect(byId(repo.workspaces, 'ws-b')).toMatchObject({ folderId: '', order: 0 })
  })
})

describe('applyRepoPlacements', () => {
  it('replaces the cached repo rows before merging their placement into the store', async () => {
    await applyRepoPlacements([repoDTO('r1', 1), repoDTO('r2', 0)])

    const repos = await getAllEntities<RepoDTO>('crowbar_repos')
    expect(byId(repos, 'r1')?.order).toBe(1)
    expect(byId(repos, 'r2')?.order).toBe(0)
    expect(useSidebarStore.getState().repos.map((r) => [r.id, r.order])).toEqual([
      ['r2', 0],
      ['r1', 1],
    ])
  })
})
