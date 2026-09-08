/**
 * Unit coverage for the handlers extracted from `sidebar-tree-panel.tsx`
 * (Task 8/29) into `space-content-actions.ts` (Task 30) so `SpaceScroller`
 * can share them across every project's panel. The logic itself is
 * unchanged — only its home moved — so these pin the same behavior the
 * panel's own (now-deleted) test file did, at the function level rather
 * than through a rendered tree.
 */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const {
  postWorkspace,
  createChat,
  createChatWithOwnWorktree,
  deleteChat,
  toastError,
  getHomeWorkspaceId,
} = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  createChat: vi.fn(() => Promise.resolve('chat-1')),
  createChatWithOwnWorktree: vi.fn(() => Promise.resolve('chat-1')),
  deleteChat: vi.fn(() => Promise.resolve()),
  toastError: vi.fn(),
  getHomeWorkspaceId: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  createChat,
  createChatWithOwnWorktree,
  deleteChat,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
// `handleOpen`'s home branch reads this directly (see `resolveHomeRow`) —
// the real resolver needs an async fetch+cache round trip these tests have
// no reason to exercise; `handleCreateHomeThread`'s own tests never needed
// this mock since they take `homeWorkspaceId` as a direct argument instead.
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({ getHomeWorkspaceId }))

import {
  resolveChatRow,
  resolveRow,
  handleOpen,
  handleTrash,
  handleCreate,
  handleCreateHomeThread,
} from '@/components/layout/space-content-actions'
import { getInitialState, useSidebarStore, type Chat, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
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

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useRemovalTrayStore.setState(getInitialRemovalState())
  useHomeTreeStore.setState({ trees: {} })
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
  it('navigates into a workspace row', () => {
    const navigate = vi.fn()
    const repos = [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })]
    handleOpen('ws-a', repos, navigate)
    expect(navigate).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
    })
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

    it('folds a BUBBLE instead of navigating — it owns no workspace to open', () => {
      const navigate = vi.fn()
      const toggle = vi.spyOn(useSidebarStore.getState(), 'toggleChatRow')
      handleOpen('c1', [withChat({ id: 'c1' })], navigate)
      expect(toggle).toHaveBeenCalledWith('c1')
      expect(navigate).not.toHaveBeenCalled()
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
        // The drop's merge would have grouped c1+c2 into one Recents entry.
        expect(windowPaneStore.getState().dormantArrangements).toEqual([])
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

/**
 * The two verbs a chat row must NOT answer wrongly.
 *
 * `handleCreate` used to fall through to a path built for a different row
 * kind and explain itself in that kind's words — worse than doing nothing,
 * because the explanation was false. `handleTrash` used to be the same
 * shape of bug fixed the other direction — a direct `deleteChat` call with
 * no removal-tray draft at all. Addendum §2 closes THAT gap instead: a chat
 * now goes through the exact same tray every other kind already did, so its
 * delete is no longer a special case.
 */
describe('a chat row does not borrow another row kind’s refusal', () => {
  const repoWithChat = () =>
    repo({ chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }] })

  it('handleTrash holds a chat in the removal tray — no direct deleteChat call', () => {
    useSidebarStore.setState({ repos: [repoWithChat()] })

    expect(handleTrash('c1')).toBe(true)

    expect(deleteChat).not.toHaveBeenCalled()
    const entries = useRemovalTrayStore.getState().entries
    expect(entries).toHaveLength(1)
    expect(entries[0]).toMatchObject({ kind: 'chat', id: 'c1', label: 'a chat' })
    // repo()'s default `defaultWorkspaceId` ('home-1') is the scoped
    // workspace id the DELETE request is addressed through once the hold
    // actually commits — a repo with no real `workspaces` entries.
    expect(entries[0].wsId).toBe('home-1')
    // A chat drains on the same 8s clock every non-cascading kind uses —
    // it does not wait on Cancel/Remove the way a repo/project does.
    expect(entries[0].deadlineAt).not.toBeNull()
  })

  it('handleCreate is SILENT — never the folder’s "has none to run it in"', () => {
    useSidebarStore.setState({ repos: [repoWithChat()] })
    handleCreate('c1', 'thread')
    expect(toastError).not.toHaveBeenCalled()
    expect(createChat).not.toHaveBeenCalled()
    expect(postWorkspace).not.toHaveBeenCalled()
  })
})

// Task 8: "create workspace" now mints the workspace AND its first chat
// atomically (POST .../chats {ownWorktree: true}) instead of the old
// chat-less postWorkspace — a bare branch row today, with a separate child
// chat row only once something ELSE later starts a conversation in it. One
// call now produces both at once (model spec §4.1, "one command replaces
// every create path").
describe('creating a workspace off the repo-home row', () => {
  it('calls the atomic own-worktree endpoint, not postWorkspace', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'home-1',
    )
    expect(postWorkspace).not.toHaveBeenCalled()
  })

  // The clicked row's own id is the fallback, not the rule — see the regular-fork
  // block below, where the workspace names a real owning chat to place by.
  it('falls back to the clicked row id for a workspace that names no owning chat', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })

    handleCreate('ws-a', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'claude', 'ws-a')
  })

  it('picks the first ENABLED provider from the global provider store', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [
        { id: 'disabled-one', enabled: false },
        { id: 'codex', enabled: true },
      ] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith('p1', 'r1', 'codex', 'home-1')
  })

  it('is a silent no-op with no enabled provider loaded yet', async () => {
    useAgentProvidersStore.setState({ status: 'ready', providers: [] })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(postWorkspace).not.toHaveBeenCalled()
  })

  // Regression: a burst of clicks on one row's "+" (the exact shape of "the fork
  // button does nothing" — no visible feedback between click and the row appearing
  // made a user click again) used to mint one chat AND one runner per click. Most of
  // those runners lost the concurrent-worktree-fork startup race and left a chat
  // with a real id and zero conversation, ever — permanently unresumable. One
  // request in flight per row closes this at its source.
  it('a second click while the first create is still in flight mints nothing extra', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace')
    handleCreate('home-1', 'workspace')
    handleCreate('home-1', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledOnce()
  })

  it('releases the guard once the request settles, so the NEXT click is honored', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [repo()] })

    handleCreate('home-1', 'workspace')
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
    handleCreate('home-1', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledTimes(2)
  })

  it('a different row is never blocked by another row’s in-flight create', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({
      repos: [repo(), repo({ id: 'r2', projectId: 'p2', defaultWorkspaceId: 'home-2' })],
    })

    handleCreate('home-1', 'workspace')
    handleCreate('home-2', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledTimes(2)
  })
})

/**
 * A REGULAR fork is the one row whose id is NOT the id the daemon places by.
 * Its owning chat is `type: 'chat'` (`tree/backfill.go`'s `owningChatType`) and
 * is already drawn as its own conversation beside it, so the row cannot take
 * that id the way a locked branch's does — one id would land on two rows, one
 * of them its own parent. The workspace names it instead
 * (`WorkspaceDTO.owningChatId`), and the create reads it from there.
 */
describe('creating a workspace off a REGULAR fork row', () => {
  const forkRepo = () =>
    repo({
      workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-owner' }],
      chats: [
        { id: 'c-owner', repoId: 'r1', type: 'chat', workspaceId: 'ws-a', title: '', order: 0 },
      ],
    })

  it('names the workspace’s OWNING CHAT, never the clicked row id', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'workspace')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'c-owner',
    )
  })

  // The thread half is a different question with a different answer: it posts
  // to that workspace's chats mount, so it wants the WORKSPACE and never a
  // chat id.
  //
  // Providers come from the GLOBAL store now, not the per-workspace one this
  // used to seed — see `enabledProvider` in space-content-actions.ts. Seeding
  // the workspace store was itself the shape of the bug: only a MOUNTED
  // workspace ever fills that copy, so on a row the user has never opened the
  // real click found `providers: []` and returned with no request at all.
  it('its thread "+" still runs in the workspace, not in the owning chat', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'thread')

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude')
  })

  // The regression that made "Thread does nothing" reproducible: a workspace
  // with NO store of its own (never mounted — exactly what a sidebar row for an
  // unopened workspace is) must still start a thread, because the provider list
  // is machine-level and has nothing to do with which workspace is on screen.
  it('starts a thread on a workspace that has never been mounted', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'thread')

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude')
  })

  // A precondition that stops the click has to SAY so. Silence here is
  // indistinguishable from a dead button, which is how both create affordances
  // came to be reported as doing nothing.
  it('says why instead of silently doing nothing when no provider is enabled', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: false }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'thread')

    expect(createChat).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
  })

  it('says why instead of silently doing nothing when a fork finds no provider', () => {
    useAgentProvidersStore.setState({ status: 'ready', providers: [] as never })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('ws-a', 'workspace')

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
  })

  /**
   * THE OTHER HALF OF "THE FORK BUTTON DOES NOTHING": measured live, the POST
   * went out and the daemon really did mint the chat and its worktree — the
   * repo's chat count moved — and the sidebar never drew a row for it.
   *
   * `app-sync-provider.tsx`'s `openRepoTreeSubscription` reseeds `crowbar_chats`
   * on exactly one trigger, this repo's generation moving, and the only thing
   * that normally moves it is a chat frame arriving for a MOUNTED workspace of
   * the repo. Its own comment records the assumption that made that safe — "a
   * chat can only be created, renamed or moved from a surface that has that
   * workspace mounted" — which the sidebar's own Fork/Thread buttons broke.
   */
  it('bumps the repo’s tree signal after a fork so the new row is drawn', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })
    const before = useFolderSignalStore.getState().generations['r1'] ?? 0

    handleCreate('ws-a', 'workspace')
    await Promise.resolve()
    await Promise.resolve()

    expect(useFolderSignalStore.getState().generations['r1'] ?? 0).toBeGreaterThan(before)
  })

  it('bumps it after a thread too', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })
    const before = useFolderSignalStore.getState().generations['r1'] ?? 0

    handleCreate('ws-a', 'thread')
    await Promise.resolve()
    await Promise.resolve()

    expect(useFolderSignalStore.getState().generations['r1'] ?? 0).toBeGreaterThan(before)
  })
})

// The sidebar header's Thread button (space-header.tsx) — NOT `handleCreate`,
// which resolves its parentId against the repo-scoped sidebar store and has
// no notion of project home at all. This is the fix for the regression where
// that button landed threads on a REPO's home row instead of the project's.
describe('handleCreateHomeThread', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
  })
  afterEach(() => {
    resetWindowPaneStoreForTests()
  })

  it('creates against the resolved HOME workspace id, never a repo row', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await handleCreateHomeThread('p1', 'home-ws-1', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith('home-ws-1', 'claude')
  })

  it('opens straight into a pane when the home workspace is already active', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('home-ws-1')
    const navigate = vi.fn()

    await handleCreateHomeThread('p1', 'home-ws-1', navigate)

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('chat-1')
    expect(navigate).not.toHaveBeenCalled()
  })

  it('says why instead of silently doing nothing when no provider is enabled', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: false }] as never,
    })

    await handleCreateHomeThread('p1', 'home-ws-1', vi.fn())

    expect(createChat).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
  })

  it('toasts and does not navigate when the create request fails', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    createChat.mockRejectedValueOnce(new Error('boom'))
    const navigate = vi.fn()

    await handleCreateHomeThread('p1', 'home-ws-1', navigate)

    expect(toastError).toHaveBeenCalledOnce()
    expect(navigate).not.toHaveBeenCalled()
  })
})

describe('starting a thread on an empty folder', () => {
  it('says why instead of silently doing nothing', () => {
    useSidebarStore.setState({
      repos: [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })],
    })

    handleCreate('f1', 'thread')

    expect(toastError).toHaveBeenCalledOnce()
    expect(createChat).not.toHaveBeenCalled()
  })
})

describe('starting a thread on a real workspace', () => {
  it('creates a chat with the first enabled provider', () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    // The GLOBAL provider list — providers are machine-level, and a
    // per-workspace copy only exists once that workspace has been mounted.
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('ws-a', 'thread')

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude')
  })
})

describe('handleTrash', () => {
  it('holds a real workspace row in the removal tray, and reports it', () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })

    expect(handleTrash('ws-a')).toBe(true)

    expect(useRemovalTrayStore.getState().entries).toHaveLength(1)
    expect(useRemovalTrayStore.getState().entries[0]?.id).toBe('ws-a')
  })

  it('is a no-op for the repo-home row (no matching row for planRemoval to draft), and reports it', () => {
    useSidebarStore.setState({ repos: [repo()] })

    expect(handleTrash('home-1')).toBe(false)

    expect(useRemovalTrayStore.getState().entries).toEqual([])
  })

  // The literal live-caught bug: the daemon's ListInRepo never filters by
  // the repo id in its own URL (fetchFolders's own doc — a known, unfixed
  // backend leniency), so a home folder bleeds into every REPO's own
  // folders array too, stamped with THAT repo's id. `resolveRow`'s
  // repo-scoped walk found this FALSE match, and the removal tray then
  // committed a real DELETE against a repo that had no business resolving
  // it at all — silently destroying a home folder dragged onto the trash
  // target. Home rows must be refused here before that walk ever runs,
  // regardless of what a repo's own (bled-into) folders array claims.
  it('refuses a home folder even when a repo’s (backend-leniency-bled) folders array also claims its id', () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    useHomeTreeStore.setState({
      trees: {
        p1: { chats: [], folders: [{ id: 'home-folder-1', repoId: '', name: 'x', order: 0 }] },
      },
    })
    useSidebarStore.setState({
      repos: [repo({ folders: [{ id: 'home-folder-1', repoId: 'r1', name: 'x', order: 0 }] })],
    })

    expect(handleTrash('home-folder-1')).toBe(false)

    expect(useRemovalTrayStore.getState().entries).toEqual([])
  })

  // Task 25 review round 1, Important: a user-locked, non-home workspace
  // still shows a trash button (only the project-home row hides it), but
  // `draftFor` refuses to draft a locked workspace — the caller (the
  // delete-confirm dialog's onConfirm) needs this reported so it can tell
  // the user rather than silently swallowing a click it just walked them
  // through a confirmation for.
  it('is a no-op for a locked (non-home) workspace, and reports it', () => {
    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [
            { id: 'ws-locked', branch: 'locked-one', age: '', order: 0, status: 'locked' },
          ],
        }),
      ],
    })

    expect(handleTrash('ws-locked')).toBe(false)

    expect(useRemovalTrayStore.getState().entries).toEqual([])
  })

  // Addendum §2: a chat's delete now holds in the SAME removal tray every
  // other kind uses — `resolveChatRow` is still consulted before `resolveRow`
  // ever sees the id, but the outcome is a held `RemovalEntry`, not an
  // immediate `deleteChat` call.
  describe('a chat row', () => {
    it('holds a bubble chat in the removal tray, scoped through any workspace of its own repo', () => {
      useSidebarStore.setState({
        repos: [
          repo({
            defaultWorkspaceId: undefined,
            workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
            // No `workspaceId` — a bubble, not a worktree chat.
            chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }],
          }),
        ],
      })

      expect(handleTrash('c1')).toBe(true)

      expect(deleteChat).not.toHaveBeenCalled()
      const entries = useRemovalTrayStore.getState().entries
      expect(entries).toHaveLength(1)
      expect(entries[0]).toMatchObject({ kind: 'chat', id: 'c1', wsId: 'ws-a' })
    })

    it('refuses when the repo has no workspace at all to scope the request through', () => {
      useSidebarStore.setState({
        repos: [
          repo({
            defaultWorkspaceId: undefined,
            workspaces: [],
            chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }],
          }),
        ],
      })

      expect(handleTrash('c1')).toBe(false)

      expect(deleteChat).not.toHaveBeenCalled()
      expect(useRemovalTrayStore.getState().entries).toEqual([])
    })
  })
})

/**
 * A `branch` row's id is the id of the CHAT that owns its workspace
 * (`rows-from-repo.ts`), which puts it in the chat id space while making it no
 * chat at all. Every dispatcher here picks its behaviour by which space an id
 * falls in, so each one has to be able to tell the two apart — the bug this
 * closes is a locked branch's "+" going silently inert because `resolveChatRow`
 * matched its row and returned early.
 */
describe('a branch row is addressed by its owning chat, and is still a workspace', () => {
  const branchRow = (id: string, workspaceId: string): Chat => ({
    id,
    repoId: 'r1',
    type: 'branch',
    workspaceId,
    title: '',
    order: 0,
  })

  const lockedRepo = () =>
    repo({
      workspaces: [
        {
          id: 'ws-locked',
          branch: 'develop',
          age: '',
          status: 'locked',
          owningChatId: 'develop-row',
        },
        { id: 'ws-open', branch: 'feature/x', age: '' },
      ],
      chats: [branchRow('home-row', 'home-1'), branchRow('develop-row', 'ws-locked')],
    })

  it('is not a chat row', () => {
    expect(resolveChatRow([lockedRepo()], 'develop-row')).toBeNull()
  })

  it('resolves to the WORKSPACE it draws, so drag and removal see one id space', () => {
    const found = resolveRow([lockedRepo()], 'develop-row')
    expect(found?.subject).toMatchObject({ kind: 'workspace', id: 'ws-locked', locked: true })
  })

  it('the repo-home row resolves to the default workspace', () => {
    expect(resolveRow([lockedRepo()], 'home-row')?.subject).toMatchObject({
      kind: 'workspace',
      id: 'home-1',
    })
  })

  it('its "+" creates a workspace under the OWNING CHAT id — the id the daemon places by', () => {
    useSidebarStore.setState({ repos: [lockedRepo()] })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('develop-row', 'workspace')

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'develop-row',
    )
  })

  it('its thread "+" runs in the WORKSPACE, not in the row id', () => {
    useSidebarStore.setState({ repos: [lockedRepo()] })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('develop-row', 'thread')

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-locked', 'claude')
  })

  it('its trash takes the WORKSPACE path — refused as locked, never deleteChat', () => {
    useSidebarStore.setState({ repos: [lockedRepo()] })

    // A branch row can only ever be a locked branch or a repo home, and
    // `planRemoval`'s `draftFor` refuses both — so the tray staying empty is
    // the REFUSAL, and on its own it is indistinguishable from doing nothing.
    // The ordinary workspace below is what tells those two apart: the same
    // call, in the same repo, does reach the tray.
    expect(handleTrash('develop-row')).toBe(false)
    expect(deleteChat).not.toHaveBeenCalled()
    expect(useRemovalTrayStore.getState().entries).toEqual([])

    expect(handleTrash('ws-open')).toBe(true)
    expect(useRemovalTrayStore.getState().entries).toHaveLength(1)
    expect(deleteChat).not.toHaveBeenCalled()
  })
})
