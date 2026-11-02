import { beforeEach, describe, expect, it } from 'vitest'
import { getHomeTree, resolveHomeRowScope } from '@/core/entities/home-rows'
import {
  resetEntities,
  seedHome,
  seedHomeTree,
  seedRepos,
} from '@/__tests__/__fixtures__/sidebar-entities'

beforeEach(() => {
  resetEntities()
  seedHomeTree('p1', {
    chats: [{ id: 'c-home', repoId: '', title: 'Plan', order: 0 }],
    folders: [{ id: 'f-home', repoId: '', name: 'Notes', order: 1 }],
  })
  seedRepos([
    {
      id: 'r1',
      projectId: 'p1',
      name: 'crowbar',
      avatarLabel: 'C',
      avatarColor: '',
      workspaces: [],
      chats: [{ id: 'c-repo', repoId: 'r1', title: 'Repo', order: 0 }],
    },
  ])
})

describe('a project-home row', () => {
  it('is no home row before its home workspace is known', () => {
    expect(resolveHomeRowScope('c-home')).toBeNull()
  })

  it('resolves a home chat or folder to its project and home workspace', () => {
    seedHome('p1', 'ws-home')
    expect(resolveHomeRowScope('c-home')).toEqual({
      kind: 'chat',
      projectId: 'p1',
      homeWorkspaceId: 'ws-home',
    })
    expect(resolveHomeRowScope('f-home')).toMatchObject({ kind: 'folder', projectId: 'p1' })
  })

  it("never claims a repo's row or one the store does not hold", () => {
    seedHome('p1', 'ws-home')
    expect(resolveHomeRowScope('c-repo')).toBeNull()
    expect(resolveHomeRowScope('nowhere')).toBeNull()
  })

  it("reads a project's home chats and folders, and an empty home for one it never saw", () => {
    expect(getHomeTree('p1').chats.map((c) => c.id)).toEqual(['c-home'])
    expect(getHomeTree('p1').folders.map((f) => f.id)).toEqual(['f-home'])
    expect(getHomeTree('p9')).toEqual({ chats: [], folders: [] })
  })
})
