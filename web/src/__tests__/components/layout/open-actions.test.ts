/** Opening a sidebar row: what an id resolves to, and where its chat lands. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const { postWorkspace, toastError, getHomeWorkspaceId } = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  toastError: vi.fn(),
  getHomeWorkspaceId: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
// `handleOpen`'s home branch reads this directly (see `resolveHomeRow`) —
// the real resolver needs an async fetch+cache round trip these tests have
// no reason to exercise; `handleCreateHomeThread`'s own tests never needed
// this mock since they take `homeWorkspaceId` as a direct argument instead.
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { resolveRow, handleOpen } from '@/components/layout/open-actions'
import { getInitialState, useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { usePendingCreatesStore, getInitialPendingCreatesState } from '@/lib/store/pending-creates'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'home-1',
  defaultBranch: 'main',
  workspaces: [],
  folders: [],
  ...over,
})

afterEach(() => {
  // A GLOBAL store — a leaked 'terminal' default would silently arm every
  // later create in this file with a surface its own assertions never named.
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
})

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
  useHomeTreeStore.setState({ trees: {} })
  usePendingCreatesStore.setState(getInitialPendingCreatesState())
  // Create-workspace now needs a PROVIDER (the new atomic endpoint starts a
  // CLI, unlike the old chat-less postWorkspace) — the global provider store
  // (agent-providers-store.ts), not a per-workspace one, since there is no
  // workspace yet to scope a per-workspace read through.
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
})

describe('resolveRow', () => {
  it('resolves a real workspace row', () => {
    const repos = [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })]
    const found = resolveRow(repos, 'ws-a')
    expect(found?.repo.id).toBe('r1')
    expect(found?.subject).toEqual({
      kind: 'workspace',
      id: 'ws-a',
      repoId: 'r1',
      locked: false,
      parentId: undefined,
    })
  })

  it('resolves a folder row', () => {
    const repos = [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })]
    const found = resolveRow(repos, 'f1')
    expect(found?.subject).toEqual({ kind: 'folder', id: 'f1', repoId: 'r1', parentId: undefined })
  })

  it('resolves the repo-home id as a workspace subject with no matching row', () => {
    const repos = [repo()]
    const found = resolveRow(repos, 'home-1')
    expect(found?.subject).toEqual({ kind: 'workspace', id: 'home-1', repoId: 'r1' })
  })

  it('returns null for an id in no repo', () => {
    expect(resolveRow([repo()], 'nope')).toBeNull()
  })
})

describe('handleOpen', () => {
  // A workspace row whose owning chat has not been recorded yet is a list
  // still landing (the daemon mints an owner on the first read of a chatless
  // workspace), so it says so instead of navigating to a route whose
  // chat-keyed explorer could never load.
  it('says the chat is still loading, and does not navigate, for a workspace row with no owning chat yet', () => {
    const navigate = vi.fn()
    const repos = [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })]
    handleOpen('ws-a', repos, navigate)
    expect(navigate).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledWith("Can't open alpha yet — its chat is still loading")
  })

  it('toggles a folder instead of navigating', () => {
    const navigate = vi.fn()
    const toggle = vi.spyOn(useSidebarStore.getState(), 'toggleChatRow')
    const repos = [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })]
    handleOpen('f1', repos, navigate)
    expect(toggle).toHaveBeenCalledWith('f1')
    expect(navigate).not.toHaveBeenCalled()
  })

  it('is a no-op for a row not in the given (removal-filtered) repos', () => {
    const navigate = vi.fn()
    handleOpen('ghost', [repo()], navigate)
    expect(navigate).not.toHaveBeenCalled()
  })

  // Chat rows are drawn in the tree now (design spec §3.1's fourth row kind).
  // They looked exactly like every other row and did nothing at all, because
  // `resolveRow` only ever searched workspaces/folders/the repo home.
  describe('a chat row', () => {
    const withChat = (chat: Partial<Chat> & { id: string }) =>
      repo({
        workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
        chats: [{ repoId: 'r1', title: 'a chat', order: 0, ...chat }],
      })

    it('navigates to the workspace a WORKTREE chat owns, like a branch row', () => {
      const navigate = vi.fn()
      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-a' })], navigate)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
      })
    })

    it('navigates to the repo home when that is the workspace it owns', () => {
      const navigate = vi.fn()
      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'home-1' })], navigate)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'home-1' },
      })
    })

    // Regression: a real deadlock, not a guess — live-reproduced by clicking
    // a chat whose workspace was NOT the one an already-occupied pane
    // belonged to. `navigateThenOpenChat` used to `navigate()` then POLL
    // `getActiveWorkspaceId()` for up to 2s before opening a pane — but that
    // global only ever changes from INSIDE `WorkspaceView`'s own effect, which
    // only runs once `IDEShell`'s `effectiveActiveWorkspaceId` resolves to the
    // new workspace, and that resolution prefers `activePaneWorkspaceId` (the
    // ALREADY-occupied pane's own workspace) over the just-changed route by
    // design (ide-shell.tsx, for the unrelated "switch focus between two
    // panes of an existing split" case). With any pane already holding a
    // foreign chat, the poll could never win — the click looked like it did
    // nothing, live-reported as "clicking on a not opened row... simply
    // anything happens." Panes are window-level (Task 26), so nothing here
    // ever actually needed "active" to be true — this pins that a chat now
    // lands in a pane immediately once `navigate()` resolves, not after some
    // global that this suite never had to move to begin with.
    it('opens into a pane immediately after navigating, even while the active PANE already holds a chat from a DIFFERENT workspace', async () => {
      resetWindowPaneStoreForTests()
      setActiveWorkspaceId('ws-other') // some other workspace is "active"
      windowPaneStore.getState().paneActions.openChat('already-open-chat')
      const navigate = vi.fn()

      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-a' })], navigate)
      // navigateThenOpenChat awaits navigate() before opening the pane —
      // flush that one microtask.
      await Promise.resolve()
      await Promise.resolve()

      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
      })
      const panes = windowPaneStore.getState().panes
      expect(Object.values(panes).some((p) => p.chatId === 'c1')).toBe(true)
      resetWindowPaneStoreForTests()
    })

    // Regression: a bubble at the repo root used to fold instead of opening
    // — it owns no `Workspace` of its own, so `chat.workspaceId` is null, and
    // that null used to be read as "this row opens nothing." It now falls
    // back to its nearest ANCESTOR workspace (rows-from-repo.ts's own
    // `ancestorWorkspaceId`) — the repo's own home, for a bubble with no
    // workspace/folder ancestor above it at all. Live-reported: "clicking on
    // a not opened row... it should create a view on its own."
    it('opens a BUBBLE into its nearest ancestor workspace (the repo home, at the root)', () => {
      const navigate = vi.fn()
      handleOpen('c1', [withChat({ id: 'c1' })], navigate)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'home-1' },
      })
    })

    it('opens a BUBBLE nested under a real workspace into THAT workspace', () => {
      const navigate = vi.fn()
      handleOpen('c1', [withChat({ id: 'c1', parentId: 'ws-a' })], navigate)
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/$repoId/$wsId',
        params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
      })
    })

    // Spec §9.2: a repo's chats are not a closed set, so a chat naming a
    // workspace outside this repo is ordinary. Routing to /ide/:p/:r/:ws with a
    // ws that is not under :r would be a URL nothing resolves.
    it('folds rather than routing to a workspace that is not in this repo', () => {
      const navigate = vi.fn()
      const toggle = vi.spyOn(useSidebarStore.getState(), 'toggleChatRow')
      handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-in-another-repo' })], navigate)
      expect(toggle).toHaveBeenCalledWith('c1')
      expect(navigate).not.toHaveBeenCalled()
    })

    /**
     * Spec §8.4: "clicking a chat in the tree makes its own view." The click
     * used to be routed straight through the DROP (`openChatIntoPane` with a
     * synthetic `zone: 'center'` on the active pane), which meant an occupied
     * active pane took the drop's MERGE branch: a split carved out of it, plus
     * `groupIntoArrangement` filing both chats into one Recents entry. Clicking
     * a second row therefore appended a chat to the view you were already in.
     */
    describe('opening it into a pane (the workspace is already on screen)', () => {
      beforeEach(() => {
        resetWindowPaneStoreForTests()
        setActiveWorkspaceId('ws-a')
      })
      afterEach(() => {
        resetWindowPaneStoreForTests()
      })

      it('fills the pane already on screen when it is empty, without navigating away', () => {
        const navigate = vi.fn()
        handleOpen('c1', [withChat({ id: 'c1', workspaceId: 'ws-a' })], navigate)

        expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
        expect(navigate).not.toHaveBeenCalled()
      })

      it('gives a second clicked chat a pane of its OWN, and merges nothing', () => {
        const navigate = vi.fn()
        const repos = [
          repo({
            workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
            chats: [
              { id: 'c1', repoId: 'r1', title: 'one', order: 0, workspaceId: 'ws-a' },
              { id: 'c2', repoId: 'r1', title: 'two', order: 1, workspaceId: 'ws-a' },
            ],
          }),
        ]

        handleOpen('c1', repos, navigate)
        handleOpen('c2', repos, navigate)

        const panes = windowPaneStore.getState().panes
        expect(panes[ROOT_PANE_ID]?.chatId).toBe('c1')
        expect(Object.values(panes).find((p) => p.chatId === 'c2')?.id).not.toBe(ROOT_PANE_ID)
        // The drop's merge would have grouped c1+c2 into one Recents row.
        expect(windowPaneStore.getState().viewOrder).toHaveLength(2)
      })

      it('a chat already up is gone TO rather than opened a second time', () => {
        const navigate = vi.fn()
        const repos = [
          repo({
            workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
            chats: [
              { id: 'c1', repoId: 'r1', title: 'one', order: 0, workspaceId: 'ws-a' },
              { id: 'c2', repoId: 'r1', title: 'two', order: 1, workspaceId: 'ws-a' },
            ],
          }),
        ]
        handleOpen('c1', repos, navigate)
        handleOpen('c2', repos, navigate)
        const paneCount = Object.keys(windowPaneStore.getState().panes).length

        handleOpen('c1', repos, navigate)

        expect(Object.keys(windowPaneStore.getState().panes)).toHaveLength(paneCount)
        expect(windowPaneStore.getState().activePaneId).toBe(ROOT_PANE_ID)
      })
    })
  })

  // The gap the user hit directly, right after project-home rows started
  // rendering: `resolveChatRow`/`resolveRow` search `repos`, which home rows
  // are never part of (home rides no repo) — so every home row rendered but
  // clicking one did nothing at all. `resolveHomeRow` is checked first now.
  describe('a project-home row', () => {
    beforeEach(() => {
      resetWindowPaneStoreForTests()
    })
    afterEach(() => {
      resetWindowPaneStoreForTests()
    })

    it('opens an existing home chat in place when home is already active', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      useHomeTreeStore.setState({
        trees: {
          p1: {
            chats: [
              { id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 },
            ],
            folders: [],
          },
        },
      })
      setActiveWorkspaceId('home-ws-1')
      const navigate = vi.fn()

      handleOpen('c1', [], navigate)

      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
      expect(navigate).not.toHaveBeenCalled()
    })

    it('navigates to project home first when it is not already active, then opens', async () => {
      // A previous test in this file may have left some OTHER workspace
      // active (there is no way to clear it back to null) — pin it to
      // something that is definitely not home-ws-1 rather than inherit
      // whatever the last test happened to leave.
      setActiveWorkspaceId('unrelated-ws')
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      useHomeTreeStore.setState({
        trees: {
          p1: {
            chats: [
              { id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 },
            ],
            folders: [],
          },
        },
      })
      // Stands in for the route change actually mounting the home workspace
      // view (workspace-view.tsx's own effect, which is what really flips
      // this) — not a sleep, `waitForActiveWorkspace`'s own real signal.
      const navigate = vi.fn(async () => {
        setActiveWorkspaceId('home-ws-1')
      })

      handleOpen('c1', [], navigate)

      await vi.waitFor(() => {
        expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c1')
      })
      expect(navigate).toHaveBeenCalledWith({
        to: '/ide/$projectId/home',
        params: { projectId: 'p1' },
      })
    })

    it('toggles fold for a home folder instead of opening it', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      useHomeTreeStore.setState({
        trees: { p1: { chats: [], folders: [{ id: 'f1', repoId: '', name: 'Notes', order: 0 }] } },
      })
      const toggle = vi.spyOn(useSidebarStore.getState(), 'toggleChatRow')
      const navigate = vi.fn()

      handleOpen('f1', [], navigate)

      expect(toggle).toHaveBeenCalledWith('f1')
      expect(navigate).not.toHaveBeenCalled()
    })

    it('resolves the right project among several visible home trees', () => {
      getHomeWorkspaceId.mockImplementation((projectId: string) =>
        projectId === 'p2' ? 'home-ws-2' : 'home-ws-1',
      )
      useHomeTreeStore.setState({
        trees: {
          p1: {
            chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: '', order: 0 }],
            folders: [],
          },
          p2: {
            chats: [{ id: 'c2', repoId: '', workspaceId: 'home-ws-2', title: '', order: 0 }],
            folders: [],
          },
        },
      })
      setActiveWorkspaceId('home-ws-2')
      const navigate = vi.fn()

      handleOpen('c2', [], navigate)

      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('c2')
      expect(navigate).not.toHaveBeenCalled()
    })
  })
})
