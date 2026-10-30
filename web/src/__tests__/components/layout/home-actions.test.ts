/** Creates in a project home: a thread under a home row, and the header's thread. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const { createChat, createChatWithOwnWorktree, toastError, getHomeWorkspaceId } = vi.hoisted(
  () => ({
    createChat: vi.fn((id: string) => Promise.resolve(id)),
    createChatWithOwnWorktree: vi.fn(),
    toastError: vi.fn(),
    getHomeWorkspaceId: vi.fn(),
  }),
)

vi.mock('@/features/agent/api/agent-chat-creates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-chat-creates')>()),
  createChat,
  createChatWithOwnWorktree,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
// `handleCreate`'s home branch reads this directly; the real resolver needs an
// async fetch+cache round trip these tests have no reason to exercise.
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))
import * as chatPresentation from '@/features/agent/hooks/use-chat-presentation'
const presetChatLandingPresentation = vi
  .spyOn(chatPresentation, 'presetChatLandingPresentation')
  .mockImplementation(() => {})

import { handleCreate } from '@/components/layout/create-actions'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { useCreateDraftStore } from '@/lib/store/create-draft'
import { useOperationsStore } from '@/lib/store/operations'
import { rowsFromOperations } from '@/components/sidebar/lib/rows-from-operations'
import { ApiError } from '@/lib/errors/api-error'
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
  useCreateDraftStore.setState({ draft: null })
  useOperationsStore.setState({ ops: {}, latestFor: {} })
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
})

const ANY = expect.any(String)
const opRows = () =>
  rowsFromOperations(Object.values(useOperationsStore.getState().ops), { projectId: 'p1' })
const mintedChatId = () => (createChat.mock.calls.at(-1) as unknown as [string])[0]

// Regression, reported live: a project-home chat's Fork was offered with no
// git repo behind it at all ("why is it letting me create a branch from
// this where there isn't a git workspace"), and its Thread did nothing.
// Project home rides no repo (`resolveHomeRowScope`'s own doc) — Thread now
// resolves through the project's own home workspace (same rule
// `handleOpen` already follows), and Fork stays a no-op, matching a home
// FOLDER's own `ownsWorktree: false` — no worktree exists for either to
// clone.
describe('a project-home chat row resolves Thread through its home workspace, and refuses Fork', () => {
  it('a home thread create draws its row from the operation', async () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    const existing = { id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 }
    useHomeTreeStore.setState({ trees: { p1: { chats: [existing], folders: [] } } })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    handleCreate('c1', 'thread', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'home-ws-1',
      'claude',
      'c1',
      undefined,
    )
    const chatId = mintedChatId()
    expect(opRows()).toMatchObject([{ id: chatId, kind: 'chat', parentId: 'c1', working: true }])
    await vi.waitFor(() =>
      expect(Object.values(useOperationsStore.getState().ops)[0]?.state).toBe('succeeded'),
    )
    // Succeeded, but the chat is not in the home tree yet: the row still stands in.
    expect(opRows()).toHaveLength(1)

    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [existing, { ...existing, id: chatId, title: '', parentId: 'c1' }],
          folders: [],
        },
      },
    })
    expect(opRows()).toEqual([])
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

    await vi.waitFor(() =>
      expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith(
        mintedChatId(),
        'terminal',
      ),
    )
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
    expect(useCreateDraftStore.getState().draft).toBeNull()
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

    expect(createChat).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'home-ws-1',
      'claude',
      '',
      undefined,
    )
  })

  it('presets the new chat onto Terminal when asked for one', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await handleCreateHomeThread('p1', 'home-ws-1', vi.fn(), 'terminal')

    expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith(
      mintedChatId(),
      'terminal',
    )
  })

  it('opens straight into a pane when the home workspace is already active', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('home-ws-1')
    const navigate = vi.fn()

    await handleCreateHomeThread('p1', 'home-ws-1', navigate)

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe(mintedChatId())
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

  it('a refused create keeps its row with the daemon’s reason and opens nothing', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    createChat.mockImplementationOnce(() =>
      Promise.reject(new ApiError('parent ws-1 not found', 404, 'not_found')),
    )
    const navigate = vi.fn()

    await handleCreateHomeThread('p1', 'home-ws-1', navigate)

    expect(navigate).not.toHaveBeenCalled()
    await vi.waitFor(() =>
      expect(opRows()).toMatchObject([
        {
          parentId: null,
          operation: { state: 'failed', problem: { message: 'parent ws-1 not found' } },
        },
      ]),
    )
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
    settle(mintedChatId())
    await Promise.all([first, second])

    expect(createChat).toHaveBeenCalledOnce()
  })
})
