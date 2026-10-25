/** Creates in a project home: a thread under a home row, and the header's thread. */
import { act } from '@testing-library/react'
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const {
  postWorkspace,
  createChat,
  createChatWithOwnWorktree,
  listGlobalProviders,
  toastError,
  getHomeWorkspaceId,
  presetChatLandingPresentation,
} = vi.hoisted(() => ({
  postWorkspace: vi.fn(() => Promise.resolve()),
  createChat: vi.fn(() => Promise.resolve('chat-1')),
  createChatWithOwnWorktree: vi.fn(() => Promise.resolve('chat-1')),
  listGlobalProviders: vi.fn((): Promise<unknown[]> => new Promise(() => {})),
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
  listGlobalProviders,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
vi.mock('@/features/agent/hooks/use-chat-presentation', () => ({ presetChatLandingPresentation }))
// `handleOpen`'s home branch reads this directly (`resolveHomeRow`); the real resolver's
// fetch+cache round trip is not what these tests exercise.
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { handleCreate } from '@/components/layout/create-actions'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { proxyMarkerError } from '@/lib/api/errors'
import { useProjectDataStore } from '@/lib/store/projects'
import { failed, idle, success } from '@/lib/loadable'
import type { Project } from '@/lib/types'
import { useSettingsStore } from '@/features/settings/store'
import { usePendingCreatesStore, getInitialPendingCreatesState } from '@/lib/store/pending-creates'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

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
  useAgentProvidersStore.setState({ status: 'ready', providers: [], remote: {} })
})

// Project home rides no repo: Thread resolves through the project's own home workspace
// (as `handleOpen` does), and Fork stays a no-op, as there is no worktree to clone.
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

    await handleCreate('c1', 'thread', vi.fn())

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

    await handleCreate('c1', 'thread', vi.fn(), 'terminal')
    await Promise.resolve()

    expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith('chat-1', 'terminal')
  })

  it('Fork is a silent no-op — no repo, no worktree to clone', async () => {
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

    await handleCreate('c1', 'workspace', vi.fn())

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(usePendingCreatesStore.getState().entries).toEqual([])
  })
})

// The tree foot's New thread (tree-foot.tsx) — NOT `handleCreate`,
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

  // The tree foot's New thread is the ONE create path with no
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
})

// A provider list belongs to a daemon: a chat started in a remote project's home is
// started with a provider THAT host offers, never one only the local daemon has.
describe('a project-home thread on an install that has paired a remote host', () => {
  const offered = (id: string, enabled = true) => ({ id, enabled }) as never
  const started = () =>
    (createChat.mock.calls as unknown as [string, string][]).map(([wsId, providerId]) => [
      wsId,
      providerId,
    ])

  beforeEach(() => {
    resetHostWorld()
    pairRemoteHost('host-1')
    loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [offered('claude')],
      remote: { 'host-1': [offered('codex-remote')] },
    })
  })
  afterEach(() => {
    vi.restoreAllMocks()
    destroyHostRuntime('host-1')
    resetHostWorld()
  })

  it('picks each project’s provider from its own host’s list', async () => {
    await handleCreateHomeThread('p-local', 'home-local', vi.fn())
    await handleCreateHomeThread('p-remote', 'home-remote', vi.fn())

    expect(started()).toEqual([
      ['home-local', 'claude'],
      ['home-remote', 'codex-remote'],
    ])
  })

  it('refuses, rather than borrowing the local list, when the remote host has none enabled', async () => {
    useAgentProvidersStore.setState({ remote: { 'host-1': [offered('codex-remote', false)] } })

    await handleCreateHomeThread('p-remote', 'home-remote', vi.fn())

    expect(createChat).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
    expect(listGlobalProviders).not.toHaveBeenCalled()
  })

  it('reads that host’s list on the first click and starts the chat with it', async () => {
    useAgentProvidersStore.setState({ remote: {} })
    listGlobalProviders.mockResolvedValueOnce([offered('codex-remote')])

    await handleCreateHomeThread('p-remote', 'home-remote', vi.fn())

    expect(listGlobalProviders).toHaveBeenCalledExactlyOnceWith('host-1')
    expect(started()).toEqual([['home-remote', 'codex-remote']])
    expect(toastError).not.toHaveBeenCalled()
  })

  it.each([
    ['could not reach', new Error('offline'), 'Couldn’t reach host-1'],
    [
      'found with another identity',
      proxyMarkerError('host-identity-changed', 'host-1'),
      'host-1 is presenting a different identity — confirm it before pairing again',
    ],
  ])('names the host it %s, once however often the button is clicked', async (_, err, said) => {
    useAgentProvidersStore.setState({ remote: {} })
    listGlobalProviders.mockRejectedValueOnce(err)

    await Promise.all([
      handleCreateHomeThread('p-remote', 'home-remote', vi.fn()),
      handleCreateHomeThread('p-remote', 'home-remote', vi.fn()),
    ])

    expect(createChat).not.toHaveBeenCalled()
    expect(listGlobalProviders).toHaveBeenCalledOnce()
    expect(toastError).toHaveBeenCalledExactlyOnceWith(said, expect.any(String))
  })

  // A refused create errors its pending row and goes nowhere, in its own words or its host's.
  const notFound = 'agent chat: parent ws-1: apperr: not found'
  it.each([
    ['local', new Error(notFound), notFound, ['Failed to start chat', notFound]],
    [
      'remote',
      proxyMarkerError('host-unreachable', 'host-1'),
      'Couldn’t reach host-1',
      ['Couldn’t reach host-1', 'Check that it is online in Settings → Hosts.'],
    ],
  ])(
    'fails a %s thread the daemon refused, on its row and in a toast',
    async (where, err, row, toast) => {
      vi.spyOn(console, 'error').mockImplementation(() => {})
      createChat.mockRejectedValueOnce(err)
      const navigate = vi.fn()

      await handleCreateHomeThread(`p-${where}`, `home-${where}`, navigate)

      expect(usePendingCreatesStore.getState().entries).toMatchObject([
        { status: 'error', error: row },
      ])
      expect(toastError).toHaveBeenCalledExactlyOnceWith(...toast)
      expect(navigate).not.toHaveBeenCalled()
    },
  )

  it('names the host by its name, never its id, also when it was removed while being asked', async () => {
    resetHostWorld()
    pairRemoteHost('host-1', 'Studio Mac')
    loadProjects(projectOn('p-remote', 'host-1'))
    useAgentProvidersStore.setState({ remote: {} })
    let fail!: (reason: Error) => void
    listGlobalProviders.mockReturnValueOnce(new Promise((_, reject) => (fail = reject)))

    const click = handleCreateHomeThread('p-remote', 'home-remote', vi.fn())
    act(() => useHostConfigStore.getState().removeHost('host-1'))
    fail(new Error('gone'))
    await click

    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Couldn’t reach Studio Mac',
      expect.any(String),
    )
  })

  // Only a loaded list can say a project's host is gone; before that a local project is merely
  // not known yet, or the read failed, and Hosts would be the wrong advice.
  it.each([
    [
      'sends an unconfigured host’s project to Hosts',
      success([projectOn('p', 'host-2')]),
      /^This project’s host is not available \| .*Hosts/,
    ],
    ['says a loading list is loading', idle(), /^Still loading your projects \| (?!.*Hosts)/],
    [
      'says a failed read failed',
      failed<Project[]>(new Error('down')),
      /^Couldn’t load your projects \|/,
    ],
  ])('%s, never a retry', async (_, data, said) => {
    useProjectDataStore.setState({ data })

    await handleCreateHomeThread('p', 'home', vi.fn())

    expect(createChat).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
    expect(toastError.mock.calls[0]?.join(' | ')).toMatch(said)
  })
})
