import { describe, it, expect, beforeEach } from 'vitest'
import {
  applyChatPlacement,
  applyFolderPlacements,
  applyWorkspacePlacement,
} from '@/lib/store/applied-placement'
import { entityStore } from '@/core/entities/entity-store'
import type { FolderDTO, RepoDTO } from '@/lib/types'
import {
  chatEntity,
  putEntities,
  resetEntities,
  seedHomeTree,
  workspaceDTO,
} from '@/__tests__/__fixtures__/sidebar-entities'

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

const folderDTO = (id: string, order: number, parentId = ''): FolderDTO => ({
  id,
  repoId: 'r1',
  projectId: 'p1',
  parentId,
  name: id,
  order,
})

const held = () => entityStore.getState().byKind

beforeEach(() => {
  resetEntities()
  putEntities('repo', [repoDTO('r1', 0)])
  putEntities('chat', [
    chatEntity({ id: 'c1', repoId: 'r1', workspaceId: 'ws-1', order: 0 }),
    chatEntity({ id: 'c2', repoId: 'r1', workspaceId: 'ws-1', order: 1 }),
  ])
  putEntities('folder', [folderDTO('f1', 2)])
  putEntities('workspace', [
    workspaceDTO({ id: 'ws-1', repoId: 'r1', order: 0 }),
    workspaceDTO({ id: 'ws-lock', repoId: 'r1', order: 3 }),
  ])
})

describe('applyChatPlacement', () => {
  it('places the moved chat and its shifted siblings, chats and folders alike', async () => {
    const repoId = await applyChatPlacement('local', { id: 'c2', parentId: 'c1', order: 0 }, [
      { id: 'c1', order: 1 },
      { id: 'f1', order: 3 },
    ])
    expect(repoId).toBe('r1')
    expect(held().chat.c2?.data).toMatchObject({ parentId: 'c1', order: 0 })
    expect(held().chat.c1?.data).toMatchObject({ parentId: '', order: 1 })
    expect(held().folder.f1?.data).toMatchObject({ order: 3 })
  })

  it('answers no repo for a project-home chat', async () => {
    seedHomeTree('p1', { chats: [{ id: 'h1', repoId: '', title: 'h', order: 0 }], folders: [] })
    expect(await applyChatPlacement('local', { id: 'h1', order: 4 })).toBeNull()
    expect(held().chat.h1?.data.order).toBe(4)
  })

  it('places a shifted locked-branch sibling on its workspace row', async () => {
    await applyChatPlacement('local', { id: 'c1', order: 0 }, [
      { id: 'ws-lock', parentId: 'f1', order: 1 },
    ])
    expect(held().workspace['ws-lock']?.data).toMatchObject({ folderId: 'f1', order: 1 })
  })

  it('keeps the version it holds, so the next frame still replaces the row', async () => {
    const v = held().chat.c2?.v
    await applyChatPlacement('local', { id: 'c2', order: 5 })
    expect(held().chat.c2?.v).toBe(v)
  })
})

describe('applyFolderPlacements', () => {
  it('places the written folders, even one no frame has brought yet', async () => {
    await applyFolderPlacements('local', [folderDTO('f1', 0), folderDTO('f-new', 1, 'c1')])
    expect(held().folder.f1?.data.order).toBe(0)
    expect(held().folder['f-new']?.data).toMatchObject({ parentId: 'c1', order: 1 })
  })

  it('places a shifted locked-branch sibling on its workspace row, never as a folder', async () => {
    await applyFolderPlacements('local', [folderDTO('f1', 0)], [{ id: 'ws-lock', order: 1 }])
    expect(held().workspace['ws-lock']?.data.order).toBe(1)
    expect(held().folder['ws-lock']).toBeUndefined()
  })
})

describe('applyWorkspacePlacement', () => {
  it('places the moved workspace and the folder it shifted', async () => {
    await applyWorkspacePlacement('local', 'ws-lock', { parentId: 'f1', order: 0 }, [
      { id: 'f1', order: 1 },
    ])
    expect(held().workspace['ws-lock']?.data).toMatchObject({ folderId: 'f1', order: 0 })
    expect(held().folder.f1?.data.order).toBe(1)
  })

  it('ignores a shifted row the store does not hold', async () => {
    await applyWorkspacePlacement('local', 'ws-lock', { parentId: '', order: 0 }, [
      { id: 'f-x', order: 1 },
    ])
    expect(held().folder['f-x']).toBeUndefined()
  })
})

describe('a placement answered by a paired host', () => {
  it("places host-1's rows and leaves local's twins alone", async () => {
    putEntities('chat', [chatEntity({ id: 'c2', repoId: 'r1', order: 7 })], 'host-1')
    putEntities('folder', [folderDTO('f1', 9)], 'host-1')
    const repoId = await applyChatPlacement('host-1', { id: 'c2', order: 0 }, [
      { id: 'f1', order: 1 },
    ])
    expect(repoId).toBe('r1')
    expect(held().chat['host-1~c2']?.data.order).toBe(0)
    expect(held().folder['host-1~f1']?.data.order).toBe(1)
    expect(held().chat.c2?.data.order).toBe(1)
    expect(held().folder.f1?.data.order).toBe(2)
  })
})
