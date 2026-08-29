import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  recentsForProject,
  workspaceIdsForProject,
} from '@/components/sidebar/lib/recents-for-project'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { Repo, Workspace } from '@/lib/store/sidebar'

interface FakeWorkspaceStoreState {
  panes: Record<string, { id: string; chatId: string | null }>
  agentChats: { working: Record<string, boolean> }
  dormantArrangements: { id: string; chatIds: string[]; state: string }[]
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
        panes: {},
        agentChats: { working: {} },
        dormantArrangements: [],
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

beforeEach(() => {
  activeIds.current = []
  storeStates.current = new Map()
  homeIds.current = new Map()
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
    storeStates.current.set('ws-1', {
      panes: { pane1: { id: 'pane1', chatId: 'chat-1' } },
      agentChats: { working: {} },
      dormantArrangements: [],
    })
    storeStates.current.set('ws-2', {
      panes: { pane2: { id: 'pane2', chatId: 'chat-2' } },
      agentChats: { working: {} },
      dormantArrangements: [],
    })
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
    storeStates.current.set('ws-1', {
      panes: { pane1: { id: 'pane1', chatId: 'chat-1' } },
      agentChats: { working: {} },
      dormantArrangements: [],
    })
    storeStates.current.set('ws-2', {
      panes: {},
      agentChats: { working: { 'chat-2': true } },
      dormantArrangements: [],
    })
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
      panes: { pane1: { id: 'pane1', chatId: 'home-chat-1' } },
      agentChats: { working: {} },
      dormantArrangements: [],
    })
    const repos = [makeTestRepo({ id: 'r1', projectId: 'p1' })] // no tree workspaces at all

    const entries = recentsForProject(repos, 'p1')

    expect(entries).toHaveLength(1)
    expect(entries[0].workspaceId).toBe('home-ws-p1')
    expect(entries[0].chatIds).toEqual(['home-chat-1'])
  })

  it("workspace-qualifies every entry's id so two retained workspaces sharing a literal pane id (e.g. ROOT_PANE_ID) never collide", () => {
    // ROOT_PANE_ID/BOTTOM_PANE_ID are module-level constants, identical
    // across EVERY workspace store — WorkspaceHost commonly retains several
    // workspaces at once, each perfectly able to hold a chat in its own
    // 'root-pane'. Without qualification both entries would carry the same
    // literal `.id`, a guaranteed React-key/data-testid collision.
    activeIds.current = ['ws-1', 'ws-2']
    storeStates.current.set('ws-1', {
      panes: { 'root-pane': { id: 'root-pane', chatId: 'chat-1' } },
      agentChats: { working: {} },
      dormantArrangements: [],
    })
    storeStates.current.set('ws-2', {
      panes: { 'root-pane': { id: 'root-pane', chatId: 'chat-2' } },
      agentChats: { working: {} },
      dormantArrangements: [],
    })
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
    // Each entry still carries the REAL (unqualified) store id for callers
    // (e.g. paneActions.forgetDormantArrangement) that must match it against
    // the owning store's own state.
    expect(entries.every((e) => e.localId === 'root-pane')).toBe(true)
    expect(ids).toContain('ws-1:root-pane')
    expect(ids).toContain('ws-2:root-pane')
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
