import { beforeEach, describe, expect, it, vi } from 'vitest'
import { openChatInOwnPane } from '@/components/sidebar/lib/pane-drop-actions'
import { selectProjectViewIds } from '@/features/panes/lib/view-selectors'
import { viewChatIds } from '@/features/panes/lib/view-state'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import type { Repo, Workspace } from '@/core/entities/repo-types'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'
import { resetEntities, seedHomeTree, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

const { activeIds, homeIds, activeWsId } = vi.hoisted(() => ({
  activeIds: { current: [] as string[] },
  homeIds: { current: new Map<string, string>() },
  activeWsId: { current: null as string | null },
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@/features/workspace/stores/workspace-store-registry')
  >()),
  getAllActiveWorkspaceIds: () => activeIds.current,
  getActiveWorkspaceId: () => activeWsId.current,
}))

vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId: (projectId: string) => homeIds.current.get(projectId) ?? null,
  getHomeOwningChatId: () => null,
}))

vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

const HOME_WS = 'home-ws-p1'

function click(chatId: string, workspaceId: string): void {
  openChatInOwnPane({
    id: chatId,
    kind: 'chat',
    parentId: null,
    order: 0,
    label: '',
    ownsWorktree: false,
    workspaceId,
    working: false,
    hasView: false,
  } as SidebarRow)
}

function bandChats(): string[] {
  const state = windowPaneStore.getState()
  return selectProjectViewIds(state, 'p1').flatMap((id) => viewChatIds(state, id))
}

function makeTestWorkspace(over: Partial<Workspace> & { id: string; branch: string }): Workspace {
  return { age: '', ...over }
}

function reposWith(workspaces: Workspace[], chats: Repo['chats']): Repo[] {
  return [
    {
      id: 'r1',
      projectId: 'p1',
      name: 'crowbar',
      avatarLabel: 'C',
      avatarColor: 'bg-indigo-700',
      defaultWorkspaceId: 'repo-home-ws',
      workspaces,
      chats,
    },
  ]
}

beforeEach(() => {
  activeIds.current = []
  resetChatStore()
  homeIds.current = new Map()
  activeWsId.current = null
  resetEntities()
  seedRepos([])
  resetWindowPaneStoreForTests()
})

describe('creating a branch while a project-home chat is open', () => {
  it('leaves the project-home chat its own Recents entry', () => {
    homeIds.current.set('p1', HOME_WS)
    activeWsId.current = HOME_WS
    activeIds.current = [HOME_WS, 'ws-1']
    writeChat({ id: 'chat-home', workspaceId: HOME_WS })
    writeChat({ id: 'chat-repo', workspaceId: 'ws-1' })
    seedHomeTree('p1', {
      chats: [{ id: 'chat-home', repoId: '', title: 'Plan', order: 0, workspaceId: HOME_WS }],
      folders: [],
    })
    const repos = reposWith(
      [makeTestWorkspace({ id: 'ws-1', branch: 'main' })],
      [{ id: 'chat-repo', repoId: 'r1', title: 'Repo chat', order: 0, workspaceId: 'ws-1' }],
    )

    windowPaneStore.getState().paneActions.setActiveProject('p1')
    seedRepos(repos)
    click('chat-repo', 'ws-1')
    click('chat-home', HOME_WS)
    expect(bandChats().sort()).toEqual(['chat-home', 'chat-repo'])

    const reposAfter = reposWith(
      [
        makeTestWorkspace({ id: 'ws-1', branch: 'main' }),
        makeTestWorkspace({ id: 'ws-new', branch: 'feature', owningChatId: 'chat-new' }),
      ],
      [
        { id: 'chat-repo', repoId: 'r1', title: 'Repo chat', order: 0, workspaceId: 'ws-1' },
        { id: 'chat-new', repoId: 'r1', title: 'feature', order: 1, workspaceId: 'ws-new' },
      ],
    )
    activeWsId.current = 'ws-new'
    activeIds.current = [HOME_WS, 'ws-1', 'ws-new']
    writeChat({ id: 'chat-new', workspaceId: 'ws-new' })
    windowPaneStore.getState().paneActions.setActiveProject('p1')
    seedRepos(reposAfter)
    click('chat-new', 'ws-new')

    expect(bandChats().sort()).toEqual(['chat-home', 'chat-new', 'chat-repo'])
    expect(
      Object.values(windowPaneStore.getState().panes)
        .map((p) => p.chatId)
        .filter(Boolean)
        .sort(),
    ).toEqual(['chat-home', 'chat-new', 'chat-repo'])
  })

  it('the same with a repo chat showing: both rows exist', () => {
    activeIds.current = ['ws-1']
    const repos = reposWith(
      [makeTestWorkspace({ id: 'ws-1', branch: 'main' })],
      [{ id: 'chat-repo', repoId: 'r1', title: 'Repo chat', order: 0, workspaceId: 'ws-1' }],
    )
    seedRepos(repos)
    windowPaneStore.getState().paneActions.setActiveProject('p1')
    click('chat-repo', 'ws-1')

    const reposAfter = reposWith(
      [
        makeTestWorkspace({ id: 'ws-1', branch: 'main' }),
        makeTestWorkspace({ id: 'ws-new', branch: 'feature', owningChatId: 'chat-new' }),
      ],
      [
        { id: 'chat-repo', repoId: 'r1', title: 'Repo chat', order: 0, workspaceId: 'ws-1' },
        { id: 'chat-new', repoId: 'r1', title: 'feature', order: 1, workspaceId: 'ws-new' },
      ],
    )
    seedRepos(reposAfter)
    click('chat-new', 'ws-new')

    expect(bandChats().sort()).toEqual(['chat-new', 'chat-repo'])
    expect(windowPaneStore.getState().viewOrder).toHaveLength(2)
  })
})
