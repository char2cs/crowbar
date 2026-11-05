/** The sidebar "+": forks (named, then minted) and threads off repo rows. */
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'

const { postWorkspace, createChat, createChatWithOwnWorktree, deleteChat, toastError } = vi.hoisted(
  () => ({
    postWorkspace: vi.fn(() => Promise.resolve()),
    createChat: vi.fn((id: string) => Promise.resolve(id)),
    createChatWithOwnWorktree: vi.fn((id: string) =>
      Promise.resolve({ id, workspaceId: 'ws-new' }),
    ),
    deleteChat: vi.fn(() => Promise.resolve<string[]>([])),
    toastError: vi.fn(),
  }),
)

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  postWorkspace,
}))
vi.mock('@/features/agent/api/agent-chat-creates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-chat-creates')>()),
  createChat,
  createChatWithOwnWorktree,
}))
import * as commandsApi from '@/features/agent/api/agent-chat-commands'
vi.spyOn(commandsApi, 'deleteChat').mockImplementation(deleteChat)
import * as providersApi from '@/features/agent/api/agent-providers-api'
const listGlobalProviders = vi
  .spyOn(providersApi, 'listGlobalProviders')
  .mockImplementation(() => new Promise(() => {}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))

import {
  handleCreate,
  confirmCreateDraft,
  cancelCreateDraft,
} from '@/components/layout/create-actions'
import { createThread } from '@/__tests__/__fixtures__/draft-thread'
import { handleTrash } from '@/components/layout/trash-actions'
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useSettingsStore } from '@/features/settings/store'
import { resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationsState } from '@/core/entities/operations'
import { operationRows } from '@/__tests__/__fixtures__/tree-rows'
import { ApiError } from '@/lib/errors/api-error'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { resetEntities, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
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
  // A GLOBAL store — a leaked 'terminal' default would silently arm every
  // later create in this file with a surface its own assertions never named.
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
})

beforeEach(() => {
  vi.clearAllMocks()
  resetEntities()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  resetOperations()
  // A create needs a provider, read from the global store: there is no workspace yet to scope by.
  useAgentProvidersStore.setState({ status: 'ready', providers: [], remote: {} })
})

const ANY = expect.any(String)

const offerProviders = (...providers: { id: string; enabled: boolean }[]) =>
  useAgentProvidersStore.setState({ status: 'ready', providers: providers as never })

/** Confirms the naming input `handleCreate` opened, as the row's input does. */
function confirmArmedBranchName(name = 'typed-branch', navigate = vi.fn()): void {
  if (!treeUiStore.getState().draft) throw new Error('no draft is open')
  confirmCreateDraft(name, navigate)
}

/** The chat id the client minted for the last own-worktree create. */
function mintedForkId(): string {
  return (createChatWithOwnWorktree.mock.calls.at(-1) as unknown as [string])[0]
}

describe('a chat row does not borrow another row kind’s refusal', () => {
  const repoWithChat = () =>
    repo({ chats: [{ id: 'c1', repoId: 'r1', title: 'a chat', order: 0 }] })

  it('handleTrash holds a chat in the removal tray — no direct deleteChat call', () => {
    seedRepos([repoWithChat()])

    expect(handleTrash('c1')).toBe(true)

    expect(deleteChat).not.toHaveBeenCalled()
    const entries = treeUiStore.getState().removals
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

  it('handleCreate is SILENT — never the folder’s "has none to run it in" — for a bubble with no ground at all', () => {
    seedRepos([repoWithChat()])
    handleCreate('c1', 'thread', vi.fn())
    expect(toastError).not.toHaveBeenCalled()
    expect(createChat).not.toHaveBeenCalled()
    expect(postWorkspace).not.toHaveBeenCalled()
  })
})

// "Create workspace" mints the workspace AND its first chat in one call. The
// parentId may be a WORKSPACE id with no owning chat: the daemon resolves it
// (api/tests TestRegression_SidebarForkWithWorkspaceIdAsParent).
describe('creating a workspace off the repo-home row', () => {
  it('calls the atomic own-worktree endpoint, not postWorkspace', async () => {
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([repo()])

    handleCreate('home-1', 'workspace', vi.fn())
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
    expect(postWorkspace).not.toHaveBeenCalled()
  })

  it('falls back to the clicked row id for a workspace that names no owning chat', async () => {
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([repo({ workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0 }] })])

    handleCreate('ws-a', 'workspace', vi.fn())
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'p1',
      'r1',
      'claude',
      'ws-a',
      'typed-branch',
    )
  })

  it('picks the first ENABLED provider from the global provider store', async () => {
    offerProviders({ id: 'disabled-one', enabled: false }, { id: 'codex', enabled: true })
    seedRepos([repo()])

    handleCreate('home-1', 'workspace', vi.fn())
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'p1',
      'r1',
      'codex',
      'home-1',
      'typed-branch',
    )
  })

  it('is a silent no-op with no enabled provider loaded yet', async () => {
    seedRepos([repo()])

    handleCreate('home-1', 'workspace', vi.fn())
    await Promise.resolve()

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(postWorkspace).not.toHaveBeenCalled()
  })

  it('a second click while the first create is still in flight mints nothing extra', async () => {
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([repo()])

    handleCreate('home-1', 'workspace', vi.fn())
    handleCreate('home-1', 'workspace', vi.fn())
    handleCreate('home-1', 'workspace', vi.fn())
    // The naming lock itself proves the point (only ONE naming entry armed no
    // matter how many "+" clicks landed) — confirming it is what turns that
    // into a network assertion.
    confirmArmedBranchName()
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledOnce()
  })

  it('releases the guard once the request settles, so the NEXT click is honored', async () => {
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([repo()])

    handleCreate('home-1', 'workspace', vi.fn())
    confirmArmedBranchName('first')
    await Promise.resolve()
    await Promise.resolve()
    await Promise.resolve()
    handleCreate('home-1', 'workspace', vi.fn())
    confirmArmedBranchName('second')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledTimes(2)
  })

  it('a different row is never blocked by another row’s in-flight create', async () => {
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([repo(), repo({ id: 'r2', projectId: 'p2', defaultWorkspaceId: 'home-2' })])

    handleCreate('home-1', 'workspace', vi.fn())
    confirmArmedBranchName('first')
    handleCreate('home-2', 'workspace', vi.fn())
    confirmArmedBranchName('second')
    await Promise.resolve()

    expect(createChatWithOwnWorktree).toHaveBeenCalledTimes(2)
  })
})

describe('creating a workspace off a REGULAR fork row', () => {
  const forkRepo = () =>
    repo({
      workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-owner' }],
      chats: [
        { id: 'c-owner', repoId: 'r1', type: 'chat', workspaceId: 'ws-a', title: '', order: 0 },
      ],
    })

  it('names the workspace’s OWNING CHAT, never the clicked row id', async () => {
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([forkRepo()])

    handleCreate('ws-a', 'workspace', vi.fn())
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

  // The daemon sends the branch's entities before the operation's succeeded.
  it('a branch create draws its row from the operation and opens the chat once it succeeded', async () => {
    resetWindowPaneStoreForTests()
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([forkRepo()])
    const navigate = vi.fn(() => Promise.resolve())

    handleCreate('ws-a', 'workspace', navigate)
    confirmArmedBranchName('feature/x', navigate)
    const chatId = mintedForkId()
    await vi.waitFor(() => expect(Object.values(operationsState().ops)[0]?.state).toBe('succeeded'))

    const rows = operationRows('p1')
    expect(rows).toMatchObject([
      { id: chatId, kind: 'branch', label: 'feature/x', parentId: 'c-owner', working: true },
    ])

    seedRepos([
      repo({
        workspaces: [
          { id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-owner' },
          { id: 'ws-new', branch: 'feature/x', age: '', order: 1, owningChatId: chatId },
        ],
        chats: [
          { id: 'c-owner', repoId: 'r1', workspaceId: 'ws-a', title: '', order: 0 },
          {
            id: chatId,
            repoId: 'r1',
            workspaceId: 'ws-new',
            parentId: 'c-owner',
            title: '',
            order: 0,
          },
        ],
      }),
    ])

    await vi.waitFor(() =>
      expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.chatId).toBe(chatId),
    )
    expect(navigate).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-new' },
    })
    expect(operationRows('p1')).toEqual([])
  })

  it('a branch create the daemon refuses keeps its row with the reason', async () => {
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([forkRepo()])
    createChatWithOwnWorktree.mockImplementationOnce(() =>
      Promise.reject(new ApiError('feature/x already exists', 409, 'conflict')),
    )

    handleCreate('ws-a', 'workspace', vi.fn())
    confirmArmedBranchName('feature/x')

    await vi.waitFor(() =>
      expect(operationRows('p1')).toMatchObject([
        {
          label: 'feature/x',
          working: false,
          operation: { state: 'failed', problem: { message: 'feature/x already exists' } },
        },
      ]),
    )
  })

  // The naming row sits at the OWNING CHAT parent: a real sibling's parentId is
  // the parent's rendered id, never the raw workspace id.
  it('confirming a name starts the create; cancelling clears the draft', async () => {
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([forkRepo()])

    handleCreate('ws-a', 'workspace', vi.fn())
    expect(treeUiStore.getState().draft).toMatchObject({
      kind: 'branch',
      parentId: 'c-owner',
      repoId: 'r1',
    })
    cancelCreateDraft()
    expect(treeUiStore.getState().draft).toBeNull()
    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()

    handleCreate('ws-a', 'workspace', vi.fn())
    confirmArmedBranchName('feature/y')
    expect(treeUiStore.getState().draft).toBeNull()
    expect(createChatWithOwnWorktree).toHaveBeenCalledExactlyOnceWith(
      ANY,
      ANY,
      'p1',
      'r1',
      'claude',
      'c-owner',
      'feature/y',
    )
  })

  it('its thread "+" still runs in the workspace, not in the owning chat', async () => {
    offerProviders({ id: 'claude', enabled: true })
    seedRepos([forkRepo()])

    await createThread('ws-a', vi.fn())

    expect(createChat).toHaveBeenCalledExactlyOnceWith(ANY, ANY, 'ws-a', 'claude', 'ws-a', 'chat')
  })

  it('says why instead of silently doing nothing when no provider is enabled', async () => {
    offerProviders({ id: 'claude', enabled: false })
    seedRepos([forkRepo()])

    await createThread('ws-a', vi.fn())

    expect(createChat).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
  })

  it('says why instead of silently doing nothing when a fork finds no provider', () => {
    seedRepos([forkRepo()])

    handleCreate('ws-a', 'workspace', vi.fn())

    expect(createChatWithOwnWorktree).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
  })
})

// A provider list belongs to a daemon: a fork on a remote host reads that host's list
// on its first use, and says so — once — when the host does not answer.
describe('forking a workspace of a project on a remote host', () => {
  const remoteRepo = () =>
    repo({
      projectId: 'p-remote',
      workspaces: [{ id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-owner' }],
      chats: [
        { id: 'c-owner', repoId: 'r1', type: 'chat', workspaceId: 'ws-a', title: '', order: 0 },
      ],
    })
  const offered = [{ id: 'codex-remote', enabled: true }] as never

  beforeEach(() => {
    resetHostWorld()
    pairRemoteHost('host-1')
    loadProjects(projectOn('p-remote', 'host-1'))
    seedRepos([remoteRepo()])
  })
  afterEach(() => resetHostWorld())

  it('reads the host’s providers on the first click, then opens the naming input with one of them', async () => {
    listGlobalProviders.mockResolvedValueOnce(offered)

    await handleCreate('ws-a', 'workspace', vi.fn())
    confirmArmedBranchName()

    expect(listGlobalProviders).toHaveBeenCalledExactlyOnceWith('host-1')
    expect(createChatWithOwnWorktree.mock.calls[0]?.slice(2, 5)).toEqual([
      'p-remote',
      'r1',
      'codex-remote',
    ])
    expect(toastError).not.toHaveBeenCalled()
  })

  it('names the host it could not reach once, opens no naming input, and lets the next click retry', async () => {
    listGlobalProviders.mockRejectedValueOnce(new Error('offline'))

    await Promise.all([
      handleCreate('ws-a', 'workspace', vi.fn()),
      handleCreate('ws-a', 'workspace', vi.fn()),
    ])

    expect(toastError).toHaveBeenCalledExactlyOnceWith('Couldn’t reach host-1', expect.any(String))
    expect(treeUiStore.getState().draft).toBeNull()

    listGlobalProviders.mockResolvedValueOnce(offered)
    await handleCreate('ws-a', 'workspace', vi.fn())
    expect(treeUiStore.getState().draft).toMatchObject({ kind: 'branch', parentId: 'c-owner' })
  })
})
