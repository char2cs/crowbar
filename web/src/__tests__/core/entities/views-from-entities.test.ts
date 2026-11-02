import { beforeEach, describe, expect, it } from 'vitest'
import { entityStore } from '@/core/entities/entity-store'
import { viewsFromEntities } from '@/core/entities/views-from-entities'
import type { RepoDTO } from '@/lib/types'
import {
  chatEntity,
  markProjectsSynced,
  projectDTO,
  putEntities,
  resetEntities,
  workspaceDTO,
} from '@/__tests__/__fixtures__/sidebar-entities'

const views = () => viewsFromEntities(entityStore.getState())

const repoDTO = (id: string, order: number): RepoDTO => ({
  id,
  projectId: 'p1',
  name: id,
  path: `/${id}`,
  defaultBranch: 'main',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  avatarUrl: '',
  avatarEmoji: '',
  order,
})

beforeEach(() => {
  resetEntities()
  putEntities('project', [projectDTO({ id: 'p1', order: 1 }), projectDTO({ id: 'p0', order: 0 })])
  putEntities('repo', [repoDTO('r1', 0), repoDTO('r2', 1)])
  putEntities('workspace', [
    workspaceDTO({ id: 'ws-main', repoId: 'r1', isDefault: true, owningChatId: 'c-main' }),
    workspaceDTO({ id: 'ws-fork', repoId: 'r1', owningChatId: 'c-fork' }),
    workspaceDTO({ id: 'ws-r2', repoId: 'r2', isDefault: true }),
    workspaceDTO({ id: 'ws-home', kind: 'home', owningChatId: 'c-home' }),
  ])
  putEntities('chat', [
    chatEntity({ id: 'c-main', repoId: 'r1', workspaceId: 'ws-main' }),
    chatEntity({ id: 'c-fork', repoId: 'r1', workspaceId: 'ws-fork', liveRunnerId: 'run-1' }),
    chatEntity({ id: 'c-thread', repoId: 'r1', workspaceId: 'ws-fork', parentId: 'c-fork' }),
    chatEntity({ id: 'c-home', workspaceId: 'ws-home' }),
  ])
  putEntities('folder', [
    { id: 'f-home', projectId: 'p1', repoId: '', parentId: '', name: 'Notes', order: 0 },
  ])
})

describe('the sidebar views of the entity store', () => {
  it('draws projects in order, and each repo with its own rows and its checkout as header', () => {
    const { projects, repos } = views()
    expect(projects.map((p) => p.id)).toEqual(['p0', 'p1'])
    expect(repos.map((r) => r.id)).toEqual(['r1', 'r2'])
    const r1 = repos[0]!
    expect(r1).toMatchObject({ defaultWorkspaceId: 'ws-main', defaultOwningChatId: 'c-main' })
    expect(r1.workspaces.map((w) => w.id)).toEqual(['ws-fork'])
    expect(r1.chats?.map((c) => [c.id, c.ownsWorktree])).toEqual([
      ['c-fork', true],
      ['c-main', true],
      ['c-thread', false],
    ])
  })

  it("files a chat or folder with no repo under its project's home", () => {
    const home = views().homeTrees.p1
    expect(home?.chats.map((c) => c.id)).toEqual(['c-home'])
    expect(home?.folders.map((f) => f.id)).toEqual(['f-home'])
    expect(views().homeTrees.p0).toEqual({ chats: [], folders: [] })
  })

  it('reads every chat for the chat surfaces, and which chat a runner runs', () => {
    expect(Object.keys(views().chats).sort()).toEqual(['c-fork', 'c-home', 'c-main', 'c-thread'])
    expect(views().byRunner).toEqual({ 'run-1': 'c-fork' })
  })

  it('is synced only once the projects scope has landed', () => {
    expect(views().synced).toBe(false)
    markProjectsSynced()
    expect(views().synced).toBe(true)
  })

  it('answers the same object until a kind it reads changes', () => {
    const before = views()
    entityStore.setState({ epoch: 'other' })
    expect(views()).toBe(before)
  })

  it('a frame for one chat keeps every other row, repo and chat object it drew', () => {
    const before = views()
    putEntities('chat', [chatEntity({ id: 'c-home', workspaceId: 'ws-home', title: 'renamed' })])
    const after = views()
    expect(after.repos).toBe(before.repos)
    expect(after.chats['c-fork']).toBe(before.chats['c-fork'])
    expect(after.homeTrees.p1?.folders).toBe(before.homeTrees.p1?.folders)
    expect(after.homeTrees.p1?.chats[0]?.title).toBe('renamed')
  })

  it('a repo frame lands the repo at its new order', () => {
    putEntities('repo', [repoDTO('r2', -1)])
    expect(views().repos.map((r) => r.id)).toEqual(['r2', 'r1'])
  })
})
