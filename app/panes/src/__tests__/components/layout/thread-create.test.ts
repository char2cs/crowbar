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
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { createThread, draftPaneId, FIRST_PROMPT } from '@/__tests__/__fixtures__/draft-thread'
import { promoteDraftThread } from '@/components/layout/draft-promote'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationRows } from '@/__tests__/__fixtures__/tree-rows'
import { ApiError } from '@/lib/errors/api-error'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import { enabledProvider } from '@/components/layout/thread-create'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

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
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
})

beforeEach(() => {
  vi.clearAllMocks()
  resetEntities()
  resetWindowPaneStoreForTests()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  resetOperations()
  useAgentProvidersStore.setState({ status: 'ready', providers: [], remote: {} })
})

const ANY = expect.any(String)

function confirmArmedBranchName(name = 'typed-branch'): void {
  if (!treeUiStore.getState().draft) throw new Error('no draft is open')
  confirmCreateDraft(name, vi.fn())
}

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
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])
    createChat.mockImplementationOnce(() =>
      Promise.reject(new ApiError('parent ws-a not found', 404, 'not_found')),
    )

    await createThread('ws-a', vi.fn())

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
    seedRepos([repo({ folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }] })])

    handleCreate('f1', 'thread', vi.fn())

    expect(toastError).toHaveBeenCalledOnce()
    expect(createChat).not.toHaveBeenCalled()
  })
})

describe('starting a thread on a remote host with no provider enabled', () => {
  afterEach(() => resetHostWorld())

  it('names the host, and says its providers are enabled on it', () => {
    pairRemoteHost('h1', 'homelab')
    loadProjects(projectOn('p-remote', 'h1'))
    useAgentProvidersStore.setState({ remote: { h1: [{ id: 'codex', enabled: false }] as never } })

    expect(enabledProvider('p-remote')).toBeNull()
    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'No agent provider is enabled on homelab',
      'Enable one on homelab itself: Settings → Providers sets up only this computer.',
    )
  })
})

describe('starting a thread on a real workspace', () => {
  it('creates a chat with the first enabled provider', async () => {
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await createThread('ws-a', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'ws-a', 'claude', 'ws-a', 'chat')
  })

  it('presets the new chat onto Terminal when asked for one', async () => {
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await createThread('ws-a', vi.fn(), 'terminal')
    await Promise.resolve()

    await vi.waitFor(() =>
      expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith(
        mintedChatId(),
        'terminal',
      ),
    )
  })

  it('lands a draft on Chat, where its first prompt is sent, for an agent with no terminal', async () => {
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true, hasTerminal: false }] as never,
    })
    useSettingsStore.setState((state) => ({
      settings: { ...state.settings, chatIsDefaultPresentation: false },
    }))

    await createThread('ws-a', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'ws-a', 'claude', 'ws-a', 'chat')
    await vi.waitFor(() =>
      expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith(mintedChatId(), 'chat'),
    )
  })

  it('starts a draft on the agent its first prompt was written for', async () => {
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [
        { id: 'claude', enabled: true },
        { id: 'codex', enabled: true },
      ] as never,
    })

    await handleCreate('ws-a', 'thread', vi.fn())
    await promoteDraftThread(draftPaneId() ?? '', { ...FIRST_PROMPT, provider: 'codex' }, () => '')

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'ws-a', 'codex', 'ws-a', 'chat')
  })

  it('a thread create draws its row from the operation and opens at once', async () => {
    resetWindowPaneStoreForTests()
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('ws-a')

    await createThread('ws-a', vi.fn())
    const chatId = mintedChatId()
    expect(opRows()).toMatchObject([{ id: chatId, kind: 'chat', parentId: 'ws-a', working: true }])

    await vi.waitFor(() =>
      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe(chatId),
    )
    expect(opRows()).toHaveLength(1)

    seedRepos([
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
    ])
    expect(opRows()).toEqual([])
  })

  it('opens the new thread into a pane immediately when its workspace is already active', async () => {
    resetWindowPaneStoreForTests()
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('ws-a')
    const navigate = vi.fn()

    await createThread('ws-a', navigate)
    await Promise.resolve()

    await vi.waitFor(() =>
      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe(mintedChatId()),
    )
    expect(navigate).not.toHaveBeenCalled()
  })

  it('a thread opened while an existing chat occupies the pane survives a delayed project bootstrap', async () => {
    resetWindowPaneStoreForTests()
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('ws-a')
    windowPaneStore.getState().paneActions.openChat('existing-chat')
    windowPaneStore.setState((s) => ({
      views: { ...s.views, [ROOT_PANE_ID]: { ...s.views[ROOT_PANE_ID], projectId: 'p1' } },
      activeViewByProject: { ...s.activeViewByProject, p1: ROOT_PANE_ID },
    }))
    const navigate = vi.fn()

    await createThread('ws-a', navigate)
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

    windowPaneStore.getState().paneActions.setActiveProject('p1')

    expect(windowPaneStore.getState().activeViewId).toBe(newPane!.viewId)
    const activePane = windowPaneStore.getState().panes[windowPaneStore.getState().activePaneId]
    expect(activePane?.chatId).toBe(mintedChatId())
  })

  it('navigates to the workspace first when it is not yet the active one, then opens the new thread', async () => {
    resetWindowPaneStoreForTests()
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    setActiveWorkspaceId('ws-other')
    const navigate = vi.fn(() => Promise.resolve())

    await createThread('ws-a', navigate)
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
    seedRepos([forkRepo()])

    handleCreate('c1', 'thread', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'ws-a', 'claude', 'c1', undefined)
  })

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
    seedRepos([forkRepo()])

    handleCreate('c1', 'thread', vi.fn())
    await Promise.resolve()

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'ws-a', 'codex', 'c1', 'terminal')
    await vi.waitFor(() =>
      expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith(
        mintedChatId(),
        'terminal',
      ),
    )
  })

  it('Thread still names no surface for a provider that cannot start on its terminal', async () => {
    useSettingsStore.setState((state) => ({
      settings: { ...state.settings, chatIsDefaultPresentation: false },
    }))
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true, hasTerminal: true }] as never,
    })
    seedRepos([forkRepo()])

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
    seedRepos([forkRepo()])

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
      undefined,
    )
  })

  it('a bubble grounded in the repo HOME workspace resolves through the home row’s own id', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    seedRepos([
      repo({ chats: [{ id: 'c1', repoId: 'r1', workspaceId: 'home-1', title: 't', order: 0 }] }),
    ])

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
      undefined,
    )
  })
})
