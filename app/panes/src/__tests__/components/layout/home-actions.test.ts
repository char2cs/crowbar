/** Creates in a project home: a thread under a home row, and the header's thread. */
import { act } from '@testing-library/react'
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
// `handleCreate`'s home branch reads this directly; the real resolver's fetch+cache
// round trip is not what these tests exercise.
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))
import * as chatPresentation from '@/features/agent/hooks/use-chat-presentation'
const presetChatLandingPresentation = vi
  .spyOn(chatPresentation, 'presetChatLandingPresentation')
  .mockImplementation(() => {})
import * as providersApi from '@/features/agent/api/agent-providers-api'
const listGlobalProviders = vi
  .spyOn(providersApi, 'listGlobalProviders')
  .mockImplementation(() => new Promise(() => {}))

import { handleCreate } from '@/components/layout/create-actions'
import { handleCreateHomeThread as openHomeDraft } from '@/components/layout/home-actions'
import { promoteDraftThread } from '@/components/layout/draft-promote'
import { loadDraft } from '@/features/agent/composer/lib/draft-persistence'
import { FIRST_PROMPT, draftPaneId } from '@/__tests__/__fixtures__/draft-thread'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { proxyMarkerError } from '@/lib/api/errors'
import { useSettingsStore } from '@/features/settings/store'
import { resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationsState } from '@/core/entities/operations'
import { operationRows } from '@/__tests__/__fixtures__/tree-rows'
import { ApiError } from '@/lib/errors/api-error'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { resetEntities, seedHomeTree } from '@/__tests__/__fixtures__/sidebar-entities'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const HOME_OF: Record<string, string> = {
  p1: 'home-ws-1',
  'p-local': 'home-local',
  'p-remote': 'home-remote',
}

afterEach(() => {
  // A GLOBAL store — a leaked 'terminal' default would silently arm every
  // later create in this file with a surface its own assertions never named.
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
})

beforeEach(() => {
  vi.clearAllMocks()
  getHomeWorkspaceId.mockImplementation((projectId: string) => HOME_OF[projectId])
  resetWindowPaneStoreForTests()
  resetEntities()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  resetOperations()
  useAgentProvidersStore.setState({ status: 'ready', providers: [], remote: {} })
})

const ANY = expect.any(String)
const opRows = (projectId = 'p1') => operationRows(projectId)
const mintedChatId = () => (createChat.mock.calls.at(-1) as unknown as [string])[0]

/** New thread: opens the blank draft pane. No chat exists yet. */
async function openThreadDraft(
  projectId: string,
  homeWorkspaceId: string,
  navigate: Parameters<typeof openHomeDraft>[2],
): Promise<void> {
  windowPaneStore.setState({ activeProjectId: projectId })
  await openHomeDraft(projectId, homeWorkspaceId, navigate)
}

/** The first send from the open draft. */
function sendDraft(typedSince = () => ''): Promise<void> {
  return promoteDraftThread(draftPaneId()!, FIRST_PROMPT, typedSince)
}

/** What a person does: open the New thread draft, then send its first message. */
async function handleCreateHomeThread(
  projectId: string,
  homeWorkspaceId: string,
  navigate: Parameters<typeof openHomeDraft>[2],
): Promise<void> {
  await openThreadDraft(projectId, homeWorkspaceId, navigate)
  await sendDraft()
}

// Project home rides no repo: Thread resolves through the project's own home workspace
// (as `handleOpen` does), and Fork stays a no-op, as there is no worktree to clone.
describe('a project-home chat row resolves Thread through its home workspace, and refuses Fork', () => {
  it('a home thread create draws its row from the operation', async () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    const existing = { id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 }
    seedHomeTree('p1', { chats: [existing], folders: [] })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    void handleCreate('c1', 'thread', vi.fn())

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
    await vi.waitFor(() => expect(Object.values(operationsState().ops)[0]?.state).toBe('succeeded'))
    // Succeeded, but the chat is not in the home tree yet: the row still stands in.
    expect(opRows()).toHaveLength(1)

    seedHomeTree('p1', {
      chats: [existing, { ...existing, id: chatId, title: '', parentId: 'c1' }],
      folders: [],
    })
    expect(opRows()).toEqual([])
  })

  it('presets Terminal for a project-home thread too, when asked for one', async () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    seedHomeTree('p1', {
      chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 }],
      folders: [],
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await handleCreate('c1', 'thread', vi.fn(), 'terminal')

    expect(presetChatLandingPresentation).toHaveBeenCalledExactlyOnceWith(
      mintedChatId(),
      'terminal',
    )
  })

  it('Fork is a silent no-op — no repo, no worktree to clone', async () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    seedHomeTree('p1', {
      chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'Existing', order: 0 }],
      folders: [],
    })
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await handleCreate('c1', 'workspace', vi.fn())

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(treeUiStore.getState().draft).toBeNull()
  })
})

// The tree foot's New thread (tree-foot.tsx) — NOT `handleCreate`, which resolves its
// parentId against the repo rows and has no notion of project home at all.
describe('handleCreateHomeThread', () => {
  afterEach(() => {
    resetWindowPaneStoreForTests()
  })

  it('creates against the resolved HOME workspace id, never a repo row', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await handleCreateHomeThread('p1', 'home-ws-1', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'home-ws-1', 'claude', '', 'chat')
  })

  // One default: with Terminal preferred, the home New thread starts on the terminal at once.
  it("starts on the user's default terminal surface, opened in the pane", async () => {
    useSettingsStore.setState((state) => ({
      settings: { ...state.settings, chatIsDefaultPresentation: false },
    }))
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [
        { id: 'claude', enabled: true, hasTerminal: true, terminalStartHere: true },
      ] as never,
    })
    setActiveWorkspaceId('home-ws-1')

    await openHomeDraft('p1', 'home-ws-1', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'home-ws-1',
      'claude',
      '',
      'terminal',
    )
    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe(mintedChatId())
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

  it('a refused create keeps its row with the daemon’s reason and leaves the draft to resend', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    createChat.mockImplementationOnce(() =>
      Promise.reject(new ApiError('parent ws-1 not found', 404, 'not_found')),
    )

    await handleCreateHomeThread('p1', 'home-ws-1', vi.fn())

    await vi.waitFor(() =>
      expect(opRows()).toMatchObject([
        {
          parentId: null,
          operation: { state: 'failed', problem: { message: 'parent ws-1 not found' } },
        },
      ]),
    )
    expect(draftPaneId()).toBeDefined()
  })

  // New thread opens a blank draft and creates nothing: the chat, its sidebar
  // row and its backend record all wait for the first send.
  it('opens a draft pane on New thread without creating a chat or a sidebar row', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })

    await openThreadDraft('p1', 'home-ws-1', vi.fn())

    expect(draftPaneId()).toBeDefined()
    expect(createChat).not.toHaveBeenCalled()
    expect(opRows()).toEqual([])
  })

  it('leaves nothing behind when the draft is abandoned', async () => {
    await openThreadDraft('p1', 'home-ws-1', vi.fn())

    windowPaneStore.getState().paneActions.closePane(draftPaneId()!)

    expect(createChat).not.toHaveBeenCalled()
    expect(opRows()).toEqual([])
    expect(draftPaneId()).toBeUndefined()
  })

  it('turns the draft into the chat in the same pane', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    await openThreadDraft('p1', 'home-ws-1', vi.fn())
    const paneId = draftPaneId()!

    await sendDraft()

    expect(windowPaneStore.getState().panes[paneId]?.chatId).toBe(mintedChatId())
  })

  // The draft's box goes when its chat opens: what was typed after the send is
  // that chat's unsent draft, for the chat's own box to pick up.
  it('hands what was typed after the send to the new chat as its draft', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    await openThreadDraft('p1', 'home-ws-1', vi.fn())

    await sendDraft(() => 'and one more thing')

    expect(loadDraft(mintedChatId())).toBe('and one more thing')
  })

  // A person who saw nothing happen taps send again: one chat, not two.
  it('a second send while the first create is still in flight mints nothing extra', async () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true }] as never,
    })
    let settle!: (id: string) => void
    createChat.mockImplementationOnce(() => new Promise<string>((r) => (settle = r)))
    await openThreadDraft('p1', 'home-ws-1', vi.fn())

    const first = sendDraft()
    const second = sendDraft()
    await vi.waitFor(() => expect(createChat).toHaveBeenCalled())
    settle(mintedChatId())
    await Promise.all([first, second])

    expect(createChat).toHaveBeenCalledOnce()
  })
})

// A provider list belongs to a daemon: a chat started in a remote project's home is
// started with a provider THAT host offers, never one only the local daemon has.
describe('a project-home thread on an install that has paired a remote host', () => {
  const offered = (id: string, enabled = true) => ({ id, enabled }) as never
  const started = () =>
    (createChat.mock.calls as unknown as string[][]).map((call) => [call[2], call[3]])

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

  // A refused create fails its operation and leaves the draft, on either host.
  it.each([
    ['local', new ApiError('parent ws-1 not found', 404, 'not_found')],
    ['remote', proxyMarkerError('host-unreachable', 'host-1')],
  ])('fails a %s thread the daemon refused, keeping the draft', async (where, err) => {
    createChat.mockRejectedValueOnce(err)

    await handleCreateHomeThread(`p-${where}`, `home-${where}`, vi.fn())

    await vi.waitFor(() =>
      expect(opRows(`p-${where}`)).toMatchObject([{ operation: { state: 'failed' } }]),
    )
    expect(draftPaneId()).toBeDefined()
  })

  it('names the host by its name, never its id, also when it was removed while being asked', async () => {
    resetHostWorld()
    pairRemoteHost('host-1', 'Studio Mac')
    loadProjects(projectOn('p-remote', 'host-1'))
    useAgentProvidersStore.setState({ remote: {} })
    let fail!: (reason: Error) => void
    listGlobalProviders.mockReturnValueOnce(new Promise((_, reject) => (fail = reject)))

    await openThreadDraft('p-remote', 'home-remote', vi.fn())
    const click = sendDraft()
    act(() => useHostConfigStore.getState().removeHost('host-1'))
    fail(new Error('gone'))
    await click

    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Couldn’t reach Studio Mac',
      expect.any(String),
    )
  })

  // Only a synced list can say a project's host is gone; before that a project is merely
  // not known yet, and Hosts would be the wrong advice.
  it.each([
    [
      'sends an unconfigured host’s project to Hosts',
      () => loadProjects(projectOn('p', 'host-2')),
      /^This project’s host is not available \| .*Hosts/,
    ],
    [
      'says a loading list is loading',
      () => resetHostWorld(),
      /^Still loading your projects \| (?!.*Hosts)/,
    ],
  ])('%s, never a retry', async (_, arrange, said) => {
    arrange()
    pairRemoteHost('host-1')

    await handleCreateHomeThread('p', 'home', vi.fn())

    expect(createChat).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
    expect(toastError.mock.calls[0]?.join(' | ')).toMatch(said)
  })
})
