/** THE thread create: pending row, request, bounded wait, failure. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const {
  postWorkspace,
  createChat,
  createChatWithOwnWorktree,
  toastError,
  presetChatLandingPresentation,
} = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  createChat: vi.fn(() => Promise.resolve('chat-1')),
  createChatWithOwnWorktree: vi.fn(() => Promise.resolve('chat-1')),
  toastError: vi.fn(),
  presetChatLandingPresentation: vi.fn(),
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  createChat,
  createChatWithOwnWorktree,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
vi.mock('@/features/agent/hooks/use-chat-presentation', () => ({ presetChatLandingPresentation }))

import { handleCreate, confirmPendingCreateName } from '@/components/layout/create-actions'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
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

/** A 'workspace' create now asks for a name before it fires (the pending
 *  row's inline input) — this drives that confirm for tests written against
 *  the old immediate-fire behavior, finding the single 'naming' entry
 *  `handleCreate` just armed exactly the way the real input would. */
function confirmArmedBranchName(name = 'typed-branch'): void {
  const armed = usePendingCreatesStore.getState().entries.find((e) => e.status === 'naming')
  if (!armed) throw new Error('confirmArmedBranchName: no naming entry is armed')
  confirmPendingCreateName(armed.tempId, name)
}

// A created row that never arrives (the daemon failed after answering) must
// not spin forever: the wait is bounded at 30 s, then the create fails.
describe('a created row that never arrives', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    resetWindowPaneStoreForTests()
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
  })
  afterEach(() => {
    vi.useRealTimers()
    resetWindowPaneStoreForTests()
  })

  async function expectFailsAtThirtySeconds(): Promise<void> {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    await vi.advanceTimersByTimeAsync(29_999)
    expect(usePendingCreatesStore.getState().entries[0]?.status).toBe('creating')
    await vi.advanceTimersByTimeAsync(1)
    expect(usePendingCreatesStore.getState().entries[0]).toMatchObject({
      status: 'error',
      error: 'The new row never arrived from the daemon',
    })
    expect(toastError).toHaveBeenCalledWith(
      'Failed to start chat',
      'The new row never arrived from the daemon',
    )
    consoleError.mockRestore()
  }

  it('a repo thread fails after 30 s, and a late arrival no longer clears it', async () => {
    const ws = { id: 'ws-a', branch: 'alpha', age: '', order: 0 }
    useSidebarStore.setState({ repos: [repo({ workspaces: [ws] })] })
    setActiveWorkspaceId('ws-a')

    handleCreate('ws-a', 'thread', vi.fn())
    await expectFailsAtThirtySeconds()

    // The wait unsubscribed: the row landing late does not touch the entry.
    const chat = { id: 'chat-1', repoId: 'r1', workspaceId: 'ws-a', title: '', order: 0 }
    useSidebarStore.setState({
      repos: [repo({ workspaces: [ws], chats: [{ ...chat, parentId: 'ws-a' }] })],
    })
    expect(usePendingCreatesStore.getState().entries[0]?.status).toBe('error')
  })

  it('a project-home thread fails after 30 s', async () => {
    setActiveWorkspaceId('home-ws-1')
    await handleCreateHomeThread('p1', 'home-ws-1', vi.fn())
    await expectFailsAtThirtySeconds()
  })
})

// A create the daemon refused used to leave only a bare "failed" badge — the
// reason it stored on the entry was never toasted or logged, so a 404 "parent
// not found" and a 409 "no fork parent" were indistinguishable from a network
// drop, on screen and in the desktop log alike.
describe('a refused create says why', () => {
  beforeEach(() => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
  })

  it('thread: toasts and logs the daemon’s own reason', async () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    createChat.mockRejectedValueOnce(new Error('agent chat: parent ws-a: apperr: not found'))

    handleCreate('ws-a', 'thread', vi.fn())
    await Promise.resolve()
    await Promise.resolve()

    expect(usePendingCreatesStore.getState().entries[0]).toMatchObject({
      status: 'error',
      error: 'agent chat: parent ws-a: apperr: not found',
    })
    expect(toastError).toHaveBeenCalledWith(
      expect.any(String),
      'agent chat: parent ws-a: apperr: not found',
    )
    expect(consoleError).toHaveBeenCalled()
    consoleError.mockRestore()
  })

  it('fork: toasts and logs the daemon’s own reason', async () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    createChatWithOwnWorktree.mockRejectedValueOnce(new Error('agent chat: no fork parent'))

    handleCreate('ws-a', 'workspace', vi.fn())
    confirmArmedBranchName('test/test')
    await Promise.resolve()
    await Promise.resolve()

    expect(usePendingCreatesStore.getState().entries[0]).toMatchObject({
      status: 'error',
      label: 'test/test',
      error: 'agent chat: no fork parent',
    })
    expect(toastError).toHaveBeenCalledWith(expect.any(String), 'agent chat: no fork parent')
    expect(consoleError).toHaveBeenCalled()
    consoleError.mockRestore()
  })
})

describe('starting a thread on an empty folder', () => {
  it('says why instead of silently doing nothing', () => {
    useSidebarStore.setState({
      repos: [repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })],
    })

    handleCreate('f1', 'thread', vi.fn())

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

    handleCreate('ws-a', 'thread', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude', 'ws-a', undefined)
  })

  // THE BUG: "can't start chats directly on a CLI, it always obligates me to
  // use the native chat" — no creation entry point could land a single new
  // chat on Terminal without flipping chatIsDefaultPresentation globally. An
  // optional 4th `presentation` arg is the fix: handleCreate presets the
  // landed chat's surface before opening it, same seam an in-pane pick uses.
  it('presets the new chat onto Terminal when asked for one', async () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('ws-a', 'thread', vi.fn(), 'terminal')
    await Promise.resolve()

    expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith('chat-1', 'terminal')
  })

  it('never presets a surface for an ordinary create', async () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('ws-a', 'thread', vi.fn())
    await Promise.resolve()

    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
  })

  // Regression, reported live: a repo-scoped thread's placement is ALSO a
  // separate Node write from its mint (sidebar-placement-unification Task 8
  // widened this off home-only) — the chat lifecycle hub broadcasts on the
  // mint alone, so a reseed can land here with the chat already existing but
  // still parented at root. `chatHasLanded` used to clear the pending row on
  // existence alone; it must now wait for the chat's own `parentId` to match
  // too, or the real (misplaced) row renders before self-correcting a beat
  // later — the exact shape already fixed for home threads and forks.
  it('does not clear the pending row until the landed chat’s own placement matches — not merely once it exists', async () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('ws-a', 'thread', vi.fn())
    await Promise.resolve()

    // Not cleared yet — the create's own promise resolved, but the real
    // chat has not been OBSERVED in the store at all.
    expect(usePendingCreatesStore.getState().entries).toHaveLength(1)

    // The chat lands, but still parented at root — its placement write has
    // not caught up yet. Must still stay pending.
    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
          chats: [{ id: 'chat-1', repoId: 'r1', workspaceId: 'ws-a', title: '', order: 0 }],
        }),
      ],
    })
    await Promise.resolve()

    expect(usePendingCreatesStore.getState().entries).toHaveLength(1)

    // The placement write catches up — parentId now matches the workspace
    // this thread was created under — and the pending row finally clears.
    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
          chats: [
            {
              id: 'chat-1',
              repoId: 'r1',
              workspaceId: 'ws-a',
              title: '',
              order: 0,
              parentId: 'ws-a',
            },
          ],
        }),
      ],
    })
    await Promise.resolve()

    expect(usePendingCreatesStore.getState().entries).toEqual([])
  })

  // Live-reported: "that new chat entity should be focused... it's just
  // adding the row" — a freshly started thread updated the sidebar tree but
  // never became the thing on screen, unlike `handleCreateHomeThread`'s own,
  // already-correct "opens the moment it exists" contract for a project-home
  // thread. This pins the repo-scoped path now matching it.
  it('opens the new thread into a pane immediately when its workspace is already active', async () => {
    resetWindowPaneStoreForTests()
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('ws-a')
    const navigate = vi.fn()

    handleCreate('ws-a', 'thread', navigate)
    await Promise.resolve()

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('chat-1')
    expect(navigate).not.toHaveBeenCalled()
  })

  // Live-reported: "Creating a thread works, but the autoopen just go
  // straight into that thread, to then come back to the latest chat I had
  // opened." Root cause: the pane store's `activeProjectId` starts null every
  // load and is set for the first time by ide-shell.tsx's route-driven
  // effect calling `setActiveProject` — which can commit AFTER a thread
  // created in that gap has already opened its own brand-new view (this
  // handler's `addPane` path, reached because the active pane already holds
  // a chat). `setActiveProject`'s bootstrap used to trust a persisted
  // `activeViewByProject` pointer unconditionally and swap the just-opened
  // thread back out for whatever chat that pointer named. See
  // `pane-slice.project-scope.test.ts`'s §8 case for the underlying
  // store-level mechanism this pins end-to-end through the real create flow.
  it('a thread opened while an existing chat occupies the pane survives a delayed project bootstrap', async () => {
    resetWindowPaneStoreForTests()
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('ws-a')
    // The chat the user is already "in" — the new thread must open as a
    // row of its own, never into this pane.
    windowPaneStore.getState().paneActions.openChat('existing-chat')
    // hydrate.ts's own answer for last session, landed before ide-shell.tsx's
    // bootstrap effect has run even once this session — `activeProjectId` is
    // still null at this point, exactly as it is right after hydrate.
    windowPaneStore.setState((s) => ({
      views: { ...s.views, [ROOT_PANE_ID]: { ...s.views[ROOT_PANE_ID], projectId: 'p1' } },
      activeViewByProject: { ...s.activeViewByProject, p1: ROOT_PANE_ID },
    }))
    const navigate = vi.fn()

    handleCreate('ws-a', 'thread', navigate)
    await Promise.resolve()

    const newPane = Object.values(windowPaneStore.getState().panes).find(
      (p) => p.chatId === 'chat-1',
    )
    expect(newPane).toBeDefined()
    expect(windowPaneStore.getState().activeViewId).toBe(newPane!.viewId)

    // The route resolves and ide-shell.tsx's effect finally fires — the
    // first `setActiveProject` call this session.
    windowPaneStore.getState().paneActions.setActiveProject('p1')

    // The thread that was already, correctly, on screen must still be
    // showing — not silently reverted to the existing chat.
    expect(windowPaneStore.getState().activeViewId).toBe(newPane!.viewId)
    const activePane = windowPaneStore.getState().panes[windowPaneStore.getState().activePaneId]
    expect(activePane?.chatId).toBe('chat-1')
  })

  it('navigates to the workspace first when it is not yet the active one, then opens the new thread', async () => {
    resetWindowPaneStoreForTests()
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('ws-other')
    const navigate = vi.fn(() => Promise.resolve())

    handleCreate('ws-a', 'thread', navigate)
    await Promise.resolve()
    await Promise.resolve()

    expect(navigate).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
    })
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe('chat-1')
  })
})

// Regression: `Chat.workspaceId` already names the GROUND workspace a bubble
// borrows from its ancestor (the same field `openableWorkspaceOf` uses to
// open it) — so once a bubble names one, its Fork/Thread resolve through
// THAT workspace instead of silently doing nothing. Before this, Thread on
// any grounded bubble was a dead button, and Fork was offered on every
// bubble with no real target at all — reported live: "why is it letting me
// create a branch from this where there isn't a git workspace associated?"
describe('a bubble chat row resolves Fork/Thread through its GROUND workspace', () => {
  const forkRepo = () =>
    repo({
      workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-owner' }],
      chats: [
        {
          id: 'c-owner',
          repoId: 'r1',
          ownsWorktree: true,
          workspaceId: 'ws-a',
          title: '',
          order: 0,
        },
        {
          id: 'c1',
          repoId: 'r1',
          workspaceId: 'ws-a',
          parentId: 'c-owner',
          title: 'a thread',
          order: 0,
        },
      ],
    })

  it('Thread runs in the ground workspace, nested under the bubble itself', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('c1', 'thread', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude', 'c1', undefined)
  })

  // THE BUG: the Thread button named no surface at all, so with "native chats"
  // off (default landing surface Terminal) the daemon forked the provider's own
  // default face — for codex an api transport with NO PTY — and the pane then
  // asked for a terminal view that had never been created. The user's default
  // now reaches the CREATE, not just the display.
  it("Thread derives Terminal from the user's default when the caller names no surface", async () => {
    useSettingsStore.setState((state) => ({
      settings: { ...state.settings, chatIsDefaultPresentation: false },
    }))
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [
        { id: 'codex', enabled: true, hasTerminal: true, terminalStartHere: true },
      ] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('c1', 'thread', vi.fn())
    await Promise.resolve()

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'codex', 'c1', 'terminal')
    // Created on the terminal ⇒ lands on it.
    expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith('chat-1', 'terminal')
  })

  // The gate holds on this path too: a provider that never declared its
  // terminal a landing surface creates exactly as it did before.
  it('Thread still names no surface for a provider that cannot start on its terminal', async () => {
    useSettingsStore.setState((state) => ({
      settings: { ...state.settings, chatIsDefaultPresentation: false },
    }))
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true, hasTerminal: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('c1', 'thread', vi.fn())
    await Promise.resolve()

    expect(createChat).toHaveBeenCalledExactlyOnceWith('ws-a', 'claude', 'c1', undefined)
    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
  })

  it('Fork forks the ground workspace’s OWNING BRANCH, never the bubble’s own id', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('c1', 'workspace', vi.fn())
    expect(usePendingCreatesStore.getState().entries).toMatchObject([{ parentId: 'c-owner' }])
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'c-owner',
      'typed-branch',
    )
  })

  // The ground can ALSO be the repo's own home workspace, which — unlike an
  // ordinary fork — is never in `repo.workspaces` for an `owningChatId` to
  // be read off. Exercises the same `resolveHomeOwnerId` fallback a direct
  // click on the home row itself already resolves to.
  it('a bubble grounded in the repo HOME workspace resolves through the home row’s own id', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({
      repos: [
        repo({ chats: [{ id: 'c1', repoId: 'r1', workspaceId: 'home-1', title: 't', order: 0 }] }),
      ],
    })

    handleCreate('c1', 'thread', vi.fn())
    expect(createChat).toHaveBeenCalledExactlyOnceWith('home-1', 'claude', 'c1', undefined)

    handleCreate('c1', 'workspace', vi.fn())
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      'claude',
      'home-1',
      'typed-branch',
    )
  })
})
