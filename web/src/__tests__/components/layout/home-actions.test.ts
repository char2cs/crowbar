/** Creates in a project home: a thread under a home row, and the header's thread. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const {
  postWorkspace,
  createChat,
  createChatWithOwnWorktree,
  toastError,
  getHomeWorkspaceId,
  presetChatLandingPresentation,
} = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  createChat: vi.fn(() => Promise.resolve('chat-1')),
  createChatWithOwnWorktree: vi.fn(() => Promise.resolve('chat-1')),
  toastError: vi.fn(),
  getHomeWorkspaceId: vi.fn(),
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
// `handleOpen`'s home branch reads this directly (see `resolveHomeRow`) —
// the real resolver needs an async fetch+cache round trip these tests have
// no reason to exercise; `handleCreateHomeThread`'s own tests never needed
// this mock since they take `homeWorkspaceId` as a direct argument instead.
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { handleCreate } from '@/components/layout/create-actions'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
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

// Regression, reported live: a project-home chat's Fork was offered with no
// git repo behind it at all ("why is it letting me create a branch from
// this where there isn't a git workspace"), and its Thread did nothing.
// Project home rides no repo (`resolveHomeRowScope`'s own doc) — Thread now
// resolves through the project's own home workspace (same rule
// `handleOpen` already follows), and Fork stays a no-op, matching a home
// FOLDER's own `ownsWorktree: false` — no worktree exists for either to
// clone.
describe('a project-home chat row resolves Thread through its home workspace, and refuses Fork', () => {
  it('Thread runs in the project’s home workspace, nested under the bubble itself, and clears once it lands', async () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 }],
          folders: [],
        },
      },
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('c1', 'thread', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith('home-ws-1', 'claude', 'c1', undefined)
    expect(usePendingCreatesStore.getState().entries).toMatchObject([
      { kind: 'chat', projectId: 'p1', parentId: 'c1' },
    ])
    await Promise.resolve()

    // Not cleared yet — the create's own promise resolved, but the real
    // chat has not been OBSERVED in the home tree store.
    expect(usePendingCreatesStore.getState().entries).toHaveLength(1)

    // Regression, reported live: a chat's MINT (the `Chat` aggregate) and its
    // PLACEMENT (a separate `Node` aggregate, home-scoped) are two sequential
    // backend writes, not one — the chat lifecycle hub broadcasts on the
    // first alone, so a reseed can land here with the chat already existing
    // but still parented at root (its `Chat.ParentID` zero value) before the
    // Node write has caught up. Clearing the pending row on existence alone
    // (the bug) handed rendering to this exact half-placed real row: it
    // rendered at the top of the list for a beat before snapping into the
    // folder — this reseed reproduces precisely that intermediate frame.
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [
            { id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 },
            { id: 'chat-1', repoId: '', workspaceId: 'home-ws-1', title: '', order: 1 },
          ],
          folders: [],
        },
      },
    })
    await Promise.resolve()

    // STILL not cleared — the landed chat's own `parentId` does not yet
    // match `c1`, the row it was actually created under.
    expect(usePendingCreatesStore.getState().entries).toHaveLength(1)

    // The SECOND, corrected reseed — the Node placement has now landed.
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [
            { id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 },
            {
              id: 'chat-1',
              repoId: '',
              workspaceId: 'home-ws-1',
              title: '',
              order: 0,
              parentId: 'c1',
            },
          ],
          folders: [],
        },
      },
    })
    await Promise.resolve()

    expect(usePendingCreatesStore.getState().entries).toEqual([])
  })

  it('presets Terminal for a project-home thread too, when asked for one', async () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 }],
          folders: [],
        },
      },
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('c1', 'thread', vi.fn(), 'terminal')
    await Promise.resolve()

    expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith('chat-1', 'terminal')
  })

  it('Fork is a silent no-op — no repo, no worktree to clone', () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 }],
          folders: [],
        },
      },
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('c1', 'workspace', vi.fn())

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(usePendingCreatesStore.getState().entries).toEqual([])
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

    expect(createChat).toHaveBeenCalledExactlyOnceWith('home-ws-1', 'claude', '', undefined)
  })

  it('presets the new chat onto Terminal when asked for one', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await handleCreateHomeThread('p1', 'home-ws-1', vi.fn(), 'terminal')

    expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith('chat-1', 'terminal')
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

  // The space header's Thread button is the ONE create path with no
  // `createInFlight` key and no pending row: nothing on screen changes until
  // the round trip lands, so a second click mints a second chat — the exact
  // double-mint `handleCreate`'s own guard exists to stop (its doc: "a user
  // who saw nothing happen clicked again").
  it('a second click while the first create is still in flight mints nothing extra', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    let settle!: (id: string) => void
    createChat.mockImplementationOnce(() => new Promise<string>((r) => (settle = r)))

    const first = handleCreateHomeThread('p1', 'home-ws-1', vi.fn())
    const second = handleCreateHomeThread('p1', 'home-ws-1', vi.fn())
    settle('chat-1')
    await Promise.all([first, second])

    expect(createChat).toHaveBeenCalledOnce()
  })

  it('draws a pending root row the instant it is clicked and clears it once the chat lands at root', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [{ id: 'c-old', repoId: '', workspaceId: 'home-ws-1', title: 'Old', order: 0 }],
          folders: [],
        },
      },
    })
    let settle!: (id: string) => void
    createChat.mockImplementationOnce(() => new Promise<string>((r) => (settle = r)))

    const done = handleCreateHomeThread('p1', 'home-ws-1', vi.fn())

    const [entry] = usePendingCreatesStore.getState().entries
    expect(entry).toMatchObject({
      kind: 'chat',
      projectId: 'p1',
      parentId: '',
      order: 1,
      workspaceId: 'home-ws-1',
      status: 'creating',
    })

    settle('chat-1')
    await done
    expect(usePendingCreatesStore.getState().entries[0]?.realId).toBe('chat-1')

    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [
            { id: 'c-old', repoId: '', workspaceId: 'home-ws-1', title: 'Old', order: 0 },
            { id: 'chat-1', repoId: '', workspaceId: 'home-ws-1', title: '', order: 1 },
          ],
          folders: [],
        },
      },
    })
    await Promise.resolve()

    expect(usePendingCreatesStore.getState().entries).toEqual([])
  })

  it('a refused create leaves the pending row in error carrying the daemon’s reason, and says it', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    createChat.mockRejectedValueOnce(new Error('agent chat: parent ws-1: apperr: not found'))

    await handleCreateHomeThread('p1', 'home-ws-1', vi.fn())

    expect(usePendingCreatesStore.getState().entries[0]).toMatchObject({
      status: 'error',
      error: 'agent chat: parent ws-1: apperr: not found',
    })
    expect(toastError).toHaveBeenCalledWith(
      expect.any(String),
      'agent chat: parent ws-1: apperr: not found',
    )
    expect(consoleError).toHaveBeenCalled()
    consoleError.mockRestore()
  })
})
