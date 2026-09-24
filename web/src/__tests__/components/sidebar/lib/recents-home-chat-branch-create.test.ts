import { beforeEach, describe, expect, it, vi } from 'vitest'
import { openChatInOwnPane } from '@/components/sidebar/lib/drop-actions'
import { selectProjectViewIds } from '@/features/panes/lib/view-selectors'
import { viewChatIds } from '@/features/panes/lib/view-state'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useSidebarStore, type Repo, type Workspace } from '@/lib/store/sidebar'

/**
 * Live report: "creating a new branch out of develop while a project-home chat
 * is open deletes that RECENT row and then opens the branch."
 *
 * This drives the create path's own tail — `navigateThenOpenChat` ->
 * `openChatInOwnPane` -> `openChat` — against the REAL window pane store,
 * with a project-home chat and a repo chat each already open in a view of
 * their own, and asserts both rows survive the new branch's own row.
 *
 * The repo chat and the home chat are asserted TOGETHER on purpose: the live
 * report's whole shape is that the repo one survives and the home one does
 * not, so a test that only watched the home chat could pass for the wrong
 * reason (both gone).
 */
interface FakeWorkspaceStoreState {
  agentChats: { chats: { id: string; workspaceId?: string }[]; working: Record<string, boolean> }
}

const { activeIds, storeStates, homeIds, activeWsId } = vi.hoisted(() => ({
  activeIds: { current: [] as string[] },
  storeStates: { current: new Map<string, FakeWorkspaceStoreState>() },
  homeIds: { current: new Map<string, string>() },
  activeWsId: { current: null as string | null },
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@/features/workspace/stores/workspace-store-registry')
  >()),
  getAllActiveWorkspaceIds: () => activeIds.current,
  getActiveWorkspaceId: () => activeWsId.current,
  getWorkspaceStore: (wsId: string) => {
    const state = storeStates.current.get(wsId)
    return state ? { getState: () => state } : undefined
  },
  getOrCreateWorkspaceStore: (wsId: string) => ({
    getState: () => storeStates.current.get(wsId) ?? { agentChats: { chats: [], working: {} } },
  }),
  isChatWorking: (chatId: string) => {
    for (const state of storeStates.current.values()) {
      if (state.agentChats.working[chatId]) return true
    }
    return false
  },
}))

vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId: (projectId: string) => homeIds.current.get(projectId) ?? null,
  getHomeOwningChatId: () => null,
}))

// pane-slice imports this for closePane's teardown; nothing here closes a
// pane, and the real module reaches the network.
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

/** Every chat on a row of p1's band, row by row. */
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
  storeStates.current = new Map()
  homeIds.current = new Map()
  activeWsId.current = null
  useHomeTreeStore.setState({ trees: {} })
  useSidebarStore.setState({ repos: [] })
  resetWindowPaneStoreForTests()
})

describe('creating a branch while a project-home chat is open', () => {
  it('leaves the project-home chat its own Recents entry', () => {
    homeIds.current.set('p1', HOME_WS)
    activeWsId.current = HOME_WS
    activeIds.current = [HOME_WS, 'ws-1']
    storeStates.current.set(HOME_WS, {
      agentChats: { chats: [{ id: 'chat-home', workspaceId: HOME_WS }], working: {} },
    })
    storeStates.current.set('ws-1', {
      agentChats: { chats: [{ id: 'chat-repo', workspaceId: 'ws-1' }], working: {} },
    })
    useHomeTreeStore.getState().setTree('p1', {
      chats: [{ id: 'chat-home', repoId: '', title: 'Plan', order: 0, workspaceId: HOME_WS }],
      folders: [],
    })
    const repos = reposWith(
      [makeTestWorkspace({ id: 'ws-1', branch: 'main' })],
      [{ id: 'chat-repo', repoId: 'r1', title: 'Repo chat', order: 0, workspaceId: 'ws-1' }],
    )

    windowPaneStore.getState().paneActions.setActiveProject('p1')
    // A tree click on each: `openChatIdInOwnView` is the one shared core every
    // "open this chat the way a click does" caller runs.
    useSidebarStore.setState({ repos })
    click('chat-repo', 'ws-1')
    click('chat-home', HOME_WS)
    expect(bandChats().sort()).toEqual(['chat-home', 'chat-repo'])

    // The fork lands: its workspace + owning chat join the tree, the route
    // moves to that workspace, and the new chat is opened in its own view.
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
    storeStates.current.set('ws-new', {
      agentChats: { chats: [{ id: 'chat-new', workspaceId: 'ws-new' }], working: {} },
    })
    windowPaneStore.getState().paneActions.setActiveProject('p1')
    useSidebarStore.setState({ repos: reposAfter })
    click('chat-new', 'ws-new')

    expect(bandChats().sort()).toEqual(['chat-home', 'chat-new', 'chat-repo'])
    // The new chat took a pane of its own — it never displaced the home
    // chat's, and no chat lost its pane.
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
    useSidebarStore.setState({ repos })
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
    useSidebarStore.setState({ repos: reposAfter })
    click('chat-new', 'ws-new')

    expect(bandChats().sort()).toEqual(['chat-new', 'chat-repo'])
    expect(windowPaneStore.getState().viewOrder).toHaveLength(2)
  })
})
