import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  recentsForProject,
  workspaceIdsForProject,
} from '@/components/sidebar/lib/recents-for-project'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore, resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import type { Repo, Workspace } from '@/lib/store/sidebar'

// Task 26: panes/dormantArrangements are window-level now (one flat store
// for every workspace — see window-pane-store.ts), so this no longer needs
// N separate fake per-workspace pane trees — the REAL windowPaneStore is
// seeded directly per test instead. Only `agentChats` (still per-workspace —
// AgentChatsSlice did not move) stays behind the registry mock.
interface FakeWorkspaceStoreState {
  agentChats: { chats: { id: string }[]; working: Record<string, boolean> }
}

const { activeIds, storeStates, homeIds } = vi.hoisted(() => ({
  activeIds: { current: [] as string[] },
  storeStates: { current: new Map<string, FakeWorkspaceStoreState>() },
  homeIds: { current: new Map<string, string>() },
}))

vi.mock('@/features/workspace/stores/workspace-store-registry', () => ({
  getAllActiveWorkspaceIds: () => activeIds.current,
  getOrCreateWorkspaceStore: vi.fn((wsId: string) => ({
    getState: () =>
      storeStates.current.get(wsId) ?? {
        agentChats: { chats: [], working: {} },
      },
  })),
}))

vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId: (projectId: string) => homeIds.current.get(projectId) ?? null,
}))

function makeTestWorkspace(over: Partial<Workspace> & { id: string; branch: string }): Workspace {
  return { age: '', ...over }
}

function makeTestRepo(over: Partial<Repo> = {}): Repo {
  return {
    id: 'r1',
    projectId: 'p1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...over,
  }
}

/** Seed the REAL window pane store with one pane holding `chatId`. */
function seedLivePane(chatId: string) {
  const { paneActions, activePaneId } = windowPaneStore.getState()
  const target = paneActions.getPaneById(activePaneId)
  const paneId = target?.chatId == null ? activePaneId : paneActions.splitPane(activePaneId, 'horizontal')!
  paneActions.setPaneChat(paneId, chatId, null)
}

beforeEach(() => {
  activeIds.current = []
  storeStates.current = new Map()
  homeIds.current = new Map()
  resetWindowPaneStoreForTests()
})

describe('workspaceIdsForProject', () => {
  it('includes the repo-home id and every workspace id under the project', () => {
    const repos = [
      makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        defaultWorkspaceId: 'home-1',
        workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'alpha' })],
      }),
      makeTestRepo({ id: 'r2', projectId: 'p2', defaultWorkspaceId: 'home-2' }),
    ]
    expect(workspaceIdsForProject(repos, 'p1')).toEqual(['home-1', 'ws-1'])
  })

  it("also includes the project's OWN resolved home-workspace id", () => {
    // Project home (`/ide/$projectId/home`) is a real, store-backed
    // WorkspaceView with no tree row — `Repo.workspaces` never carries it
    // (workspace-host.tsx: "Home is a project-level concept, not a repo
    // workspace"). Every project switch lands there, so omitting it would
    // silently exclude any chat opened on project home from Recents.
    homeIds.current.set('p1', 'home-ws-p1')
    const repos = [makeTestRepo({ id: 'r1', projectId: 'p1' })]
    expect(workspaceIdsForProject(repos, 'p1')).toContain('home-ws-p1')
  })

  it("never pulls in ANOTHER project's resolved home-workspace id", () => {
    homeIds.current.set('p2', 'home-ws-p2')
    const repos = [makeTestRepo({ id: 'r1', projectId: 'p1' })]
    expect(workspaceIdsForProject(repos, 'p1')).not.toContain('home-ws-p2')
  })

  it('omits home entirely when it has not resolved yet for this project', () => {
    const repos = [makeTestRepo({ id: 'r1', projectId: 'p1' })]
    expect(workspaceIdsForProject(repos, 'p1')).toEqual([])
  })
})

describe('recentsForProject', () => {
  it("excludes another project's entries", () => {
    activeIds.current = ['ws-1', 'ws-2']
    storeStates.current.set('ws-1', { agentChats: { chats: [{ id: 'chat-1' }], working: {} } })
    storeStates.current.set('ws-2', { agentChats: { chats: [{ id: 'chat-2' }], working: {} } })
    seedLivePane('chat-1')
    seedLivePane('chat-2')
    const repos = [
      makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'a' })],
      }),
      makeTestRepo({
        id: 'r2',
        projectId: 'p2',
        workspaces: [makeTestWorkspace({ id: 'ws-2', branch: 'b' })],
      }),
    ]

    const entries = recentsForProject(repos, 'p1')
    expect(entries).toHaveLength(1)
    expect(entries[0].workspaceId).toBe('ws-1')
    expect(entries[0].chatIds).toEqual(['chat-1'])
    expect(entries.some((e) => e.chatIds.includes('chat-2'))).toBe(false)
  })

  it('aggregates across MULTIPLE workspaces under the same project', () => {
    activeIds.current = ['ws-1', 'ws-2']
    storeStates.current.set('ws-1', { agentChats: { chats: [{ id: 'chat-1' }], working: {} } })
    storeStates.current.set('ws-2', {
      agentChats: { chats: [{ id: 'chat-2' }], working: { 'chat-2': true } },
    })
    seedLivePane('chat-1')
    const repos = [
      makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        workspaces: [
          makeTestWorkspace({ id: 'ws-1', branch: 'a' }),
          makeTestWorkspace({ id: 'ws-2', branch: 'b' }),
        ],
      }),
    ]

    const entries = recentsForProject(repos, 'p1')
    const byWorkspace = new Map(entries.map((e) => [e.workspaceId, e]))
    expect(byWorkspace.get('ws-1')?.chatIds).toEqual(['chat-1'])
    expect(byWorkspace.get('ws-2')?.chatIds).toEqual(['chat-2'])
    expect(byWorkspace.get('ws-2')?.state).toBe('working')
  })

  it("a chat opened in the project-home workspace shows up in that project's Recents", () => {
    homeIds.current.set('p1', 'home-ws-p1')
    activeIds.current = ['home-ws-p1']
    storeStates.current.set('home-ws-p1', {
      agentChats: { chats: [{ id: 'home-chat-1' }], working: {} },
    })
    seedLivePane('home-chat-1')
    const repos = [makeTestRepo({ id: 'r1', projectId: 'p1' })] // no tree workspaces at all

    const entries = recentsForProject(repos, 'p1')

    expect(entries).toHaveLength(1)
    expect(entries[0].workspaceId).toBe('home-ws-p1')
    expect(entries[0].chatIds).toEqual(['home-chat-1'])
  })

  it('every entry id is real and globally unique — no workspace-qualification needed', () => {
    // Task 26: panes are window-level now (ROOT_PANE_ID/BOTTOM_PANE_ID are no
    // longer duplicated per workspace store — there is exactly one pane
    // store), so unlike the pre-hoist version of this function, entry ids
    // need no `${wsId}:` qualification to stay collision-free — the pane
    // store already guarantees that.
    activeIds.current = ['ws-1', 'ws-2']
    storeStates.current.set('ws-1', { agentChats: { chats: [{ id: 'chat-1' }], working: {} } })
    storeStates.current.set('ws-2', { agentChats: { chats: [{ id: 'chat-2' }], working: {} } })
    seedLivePane('chat-1')
    seedLivePane('chat-2')
    const repos = [
      makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        workspaces: [
          makeTestWorkspace({ id: 'ws-1', branch: 'a' }),
          makeTestWorkspace({ id: 'ws-2', branch: 'b' }),
        ],
      }),
    ]

    const entries = recentsForProject(repos, 'p1')

    expect(entries).toHaveLength(2)
    const ids = entries.map((e) => e.id)
    expect(new Set(ids).size).toBe(2) // distinct — no collision
    // localId is just id now — no separate qualified/real pair to keep in sync.
    expect(entries.every((e) => e.localId === e.id)).toBe(true)
  })

  it("trims a SET down to this project's own members instead of leaking the whole entry", () => {
    // A record remembering chats from two different projects at once — built
    // directly via `groupIntoArrangement` rather than through a real
    // cross-project drag (the matrix already refuses one); this pins the
    // READ side regardless of how such a record came to exist.
    activeIds.current = ['ws-1', 'ws-2']
    storeStates.current.set('ws-1', { agentChats: { chats: [{ id: 'chat-1' }], working: {} } })
    storeStates.current.set('ws-2', { agentChats: { chats: [{ id: 'chat-2' }], working: {} } })
    windowPaneStore.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2'])
    const repos = [
      makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'a' })],
      }),
    ]

    const entries = recentsForProject(repos, 'p1')

    expect(entries).toHaveLength(1)
    expect(entries[0].chatIds).toEqual(['chat-1'])
    expect(entries[0].chatWorkspaces).toEqual({ 'chat-1': 'ws-1' })
  })

  it('resolves each SET member to its OWN workspace, not the first chat\'s', () => {
    activeIds.current = ['ws-1', 'ws-2']
    storeStates.current.set('ws-1', { agentChats: { chats: [{ id: 'chat-1' }], working: {} } })
    storeStates.current.set('ws-2', { agentChats: { chats: [{ id: 'chat-2' }], working: {} } })
    windowPaneStore.getState().paneActions.groupIntoArrangement(['chat-1', 'chat-2'])
    const repos = [
      makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        workspaces: [
          makeTestWorkspace({ id: 'ws-1', branch: 'a' }),
          makeTestWorkspace({ id: 'ws-2', branch: 'b' }),
        ],
      }),
    ]

    const entries = recentsForProject(repos, 'p1')

    expect(entries).toHaveLength(1)
    expect(entries[0].chatWorkspaces).toEqual({ 'chat-1': 'ws-1', 'chat-2': 'ws-2' })
  })

  it('a workspace with no live store contributes nothing, and none is created for it', () => {
    activeIds.current = [] // nothing registered — workspace never opened this session
    vi.mocked(getOrCreateWorkspaceStore).mockClear()
    const repos = [
      makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        workspaces: [makeTestWorkspace({ id: 'ws-never-opened', branch: 'a' })],
      }),
    ]
    expect(recentsForProject(repos, 'p1')).toEqual([])
    // Creating a store for a workspace nobody opened would register (and
    // leak — WorkspaceHost never destroys a store it did not itself mount)
    // one just to check for recents. `getAllActiveWorkspaceIds` names the
    // workspace ids allowed to be read; this one is not in it.
    expect(getOrCreateWorkspaceStore).not.toHaveBeenCalled()
  })
})
