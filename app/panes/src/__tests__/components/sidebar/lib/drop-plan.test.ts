import { beforeEach, describe, expect, it } from 'vitest'
import { planRowDrop } from '@/components/sidebar/lib/drop-plan'
import type { Repo } from '@/core/entities/repo-types'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { homeRows, makeChat, repoRows } from '@/__tests__/__fixtures__/tree-rows'
import {
  projectDTO,
  resetEntities,
  seedHome,
  seedHomeTree,
  seedProjects,
  seedRepos,
} from '@/__tests__/__fixtures__/sidebar-entities'

// repo-1: its checkout ws-home (owned by home-row), branch ws-1 with a folder "Specs" under it,
// a root folder "Bugs" and two threads; proj-1's home holds one chat.
const REPO: Repo = {
  id: 'repo-1',
  projectId: 'proj-1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-home',
  defaultBranch: 'main',
  workspaces: [{ id: 'ws-1', branch: 'feature-x', age: '' }],
  folders: [
    { id: 'bugs', repoId: 'repo-1', name: 'Bugs', order: 0 },
    { id: 'specs', repoId: 'repo-1', name: 'Specs', order: 0, parentId: 'ws-1' },
  ],
  chats: [
    {
      id: 'home-row',
      repoId: 'repo-1',
      ownsWorktree: true,
      workspaceId: 'ws-home',
      title: '',
      order: 0,
    },
    { id: 'root-thread', repoId: 'repo-1', workspaceId: 'ws-home', title: 'Root', order: 2 },
    {
      id: 'branch-thread',
      repoId: 'repo-1',
      workspaceId: 'ws-1',
      parentId: 'ws-1',
      title: 'On the branch',
      order: 1,
    },
  ],
}

const homeChat = makeChat({ id: 'home-chat', title: 'Plans', repoId: '', workspaceId: 'ws-proj' })
let rows: SidebarRow[] = []
const row = (id: string) => rows.find((r) => r.id === id)!

beforeEach(() => {
  resetEntities()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  seedProjects([projectDTO({ id: 'proj-1', name: 'Acme' })])
  seedRepos([REPO])
  seedHome('proj-1', 'ws-proj')
  seedHomeTree('proj-1', { chats: [homeChat], folders: [] })
  rows = [...homeRows('ws-proj', [homeChat], []), ...repoRows(REPO)]
})

// A chat lands in whatever workspace its container runs in; another than its own is a move there,
// still addressed by the workspace it runs in now.
describe('planRowDrop — a chat into another workspace', () => {
  it('moves a repo thread into a branch row, named by its branch', () => {
    expect(planRowDrop([row('root-thread')], row('ws-1'), 'into')).toEqual([
      {
        kind: 'chat',
        workspaceId: 'ws-home',
        chatId: 'root-thread',
        parentId: 'ws-1',
        order: 2,
        moveTo: { workspaceId: 'ws-1', label: 'feature-x' },
      },
    ])
  })

  it('moves a thread filed into a folder under a branch to that branch', () => {
    const [call] = planRowDrop([row('root-thread')], row('specs'), 'into')
    expect(call).toMatchObject({
      parentId: 'specs',
      moveTo: { workspaceId: 'ws-1', label: 'Specs' },
    })
  })

  it("lands 'after' an expanded branch as its first child, in that branch", () => {
    const [call] = planRowDrop([row('root-thread')], row('ws-1'), 'after')
    expect(call).toMatchObject({ parentId: 'ws-1', order: 0, moveTo: { workspaceId: 'ws-1' } })
  })

  it('moves a branch thread to the project home root, named by the project', () => {
    const [call] = planRowDrop([row('branch-thread')], row('home-chat'), 'before')
    expect(call).toMatchObject({
      workspaceId: 'ws-1',
      parentId: '',
      moveTo: { workspaceId: 'ws-proj', label: 'Acme' },
    })
  })

  it('moves a home chat reordered beside a branch to its level, the repo checkout', () => {
    const [call] = planRowDrop([row('home-chat')], row('ws-1'), 'before')
    expect(call).toMatchObject({
      workspaceId: 'ws-proj',
      parentId: row('ws-1').parentId,
      moveTo: { workspaceId: 'ws-home', label: 'repo' },
    })
  })

  it('sends no move for a drop that stays in the chat’s own workspace', () => {
    const [call] = planRowDrop([row('root-thread')], row('bugs'), 'into')
    expect(call).toEqual({
      kind: 'chat',
      workspaceId: 'ws-home',
      chatId: 'root-thread',
      parentId: 'bugs',
      order: 0,
    })
  })
})
