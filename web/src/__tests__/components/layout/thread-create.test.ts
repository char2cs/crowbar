/** THE thread create: a local operation whose row stands in until the chat lands. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const { createChat, createChatWithOwnWorktree, toastError } = vi.hoisted(() => ({
  createChat: vi.fn((id: string) => Promise.resolve(id)),
  createChatWithOwnWorktree: vi.fn((id: string) => Promise.resolve({ id, workspaceId: 'ws-new' })),
  toastError: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-chat-creates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-chat-creates')>()),
  createChat,
  createChatWithOwnWorktree,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
import * as chatPresentation from '@/features/agent/hooks/use-chat-presentation'
const presetChatLandingPresentation = vi
  .spyOn(chatPresentation, 'presetChatLandingPresentation')
  .mockImplementation(() => {})

import { handleCreate, confirmCreateDraft } from '@/components/layout/create-actions'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationRows } from '@/__tests__/__fixtures__/tree-rows'
import { ApiError } from '@/lib/errors/api-error'
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
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  useHomeTreeStore.setState({ trees: {} })
  treeUiStore.setState({ draft: null })
  resetOperations()
  // Create-workspace now needs a PROVIDER (the new atomic endpoint starts a
  // CLI, unlike the old chat-less postWorkspace) — the global provider store
  // (agent-providers-store.ts), not a per-workspace one, since there is no
  // workspace yet to scope a per-workspace read through.
  useAgentProvidersStore.setState({ status: 'ready', providers: [] })
})

const ANY = expect.any(String)

function confirmArmedBranchName(name = 'typed-branch'): void {
  if (!treeUiStore.getState().draft) throw new Error('no draft is open')
  confirmCreateDraft(name, vi.fn())
}

/** The chat id the client minted for the last thread create. */
function mintedChatId(): string {
  return (createChat.mock.calls.at(-1) as unknown as [string])[0]
}

const opRows = () => operationRows('p1')

describe('a refused thread create', () => {
  it('keeps its row with the daemon’s own reason', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    createChat.mockImplementationOnce(() =>
      Promise.reject(new ApiError('parent ws-a not found', 404, 'not_found')),
    )

    handleCreate('ws-a', 'thread', vi.fn())

    await vi.waitFor(() =>
      expect(opRows()).toMatchObject([
        {
          kind: 'chat',
          parentId: 'ws-a',
          operation: { state: 'failed', problem: { message: 'parent ws-a not found' } },
        },
      ]),
    )
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

    expect(createChat).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'ws-a',
      'claude',
      'ws-a',
      undefined,
    )
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

    await vi.waitFor(() =>
      expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith(
        mintedChatId(),
        'terminal',
      ),
    )
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

  it('a thread create draws its row from the operation and opens at once', async () => {
    resetWindowPaneStoreForTests()
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })],
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('ws-a')

    handleCreate('ws-a', 'thread', vi.fn())
    const chatId = mintedChatId()
    expect(opRows()).toMatchObject([{ id: chatId, kind: 'chat', parentId: 'ws-a', working: true }])

    // Opened on the daemon's answer, before the chat's row has arrived.
    await vi.waitFor(() =>
      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe(chatId),
    )
    expect(opRows()).toHaveLength(1)

    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }],
          chats: [
            {
              id: chatId,
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
    expect(opRows()).toEqual([])
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

    await vi.waitFor(() =>
      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe(mintedChatId()),
    )
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

    await vi.waitFor(() =>
      expect(
        Object.values(windowPaneStore.getState().panes).some((p) => p.chatId === mintedChatId()),
      ).toBe(true),
    )
    const newPane = Object.values(windowPaneStore.getState().panes).find(
      (p) => p.chatId === mintedChatId(),
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
    expect(activePane?.chatId).toBe(mintedChatId())
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
    await vi.waitFor(() =>
      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe(mintedChatId()),
    )
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

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'ws-a', 'claude', 'c1', undefined)
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

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'ws-a', 'codex', 'c1', 'terminal')
    // Created on the terminal ⇒ lands on it.
    await vi.waitFor(() =>
      expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith(
        mintedChatId(),
        'terminal',
      ),
    )
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

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'ws-a', 'claude', 'c1', undefined)
    expect(presetChatLandingPresentation).not.toHaveBeenCalled()
  })

  it('Fork forks the ground workspace’s OWNING BRANCH, never the bubble’s own id', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    useSidebarStore.setState({ repos: [forkRepo()] })

    handleCreate('c1', 'workspace', vi.fn())
    expect(treeUiStore.getState().draft).toMatchObject({ parentId: 'c-owner' })
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
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
    expect(createChat).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'home-1',
      'claude',
      'c1',
      undefined,
    )

    handleCreate('c1', 'workspace', vi.fn())
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'p1',
      'r1',
      'claude',
      'home-1',
      'typed-branch',
    )
  })
})
