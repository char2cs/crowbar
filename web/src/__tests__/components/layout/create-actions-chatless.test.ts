import { describe, expect, it, vi, beforeEach } from 'vitest'

const { createChat, createChatWithOwnWorktree, toastError, getHomeWorkspaceId, apiFetch } =
  vi.hoisted(() => ({
    createChat: vi.fn(() => Promise.resolve('chat-1')),
    createChatWithOwnWorktree: vi.fn(() => Promise.resolve('chat-1')),
    toastError: vi.fn(),
    getHomeWorkspaceId: vi.fn(),
    apiFetch: vi.fn(() => Promise.resolve(undefined)),
  }))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch,
}))
vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  createChat,
  createChatWithOwnWorktree,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: toastError, success: vi.fn(), info: vi.fn() },
}))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { handleCreate, confirmPendingCreateName } from '@/components/layout/create-actions'
import { performSetWorkspaceLock, performRenameRow } from '@/components/sidebar/lib/row-actions'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { usePendingCreatesStore, getInitialPendingCreatesState } from '@/lib/store/pending-creates'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

const navigate = vi.fn(() => Promise.resolve()) as unknown as Parameters<typeof handleCreate>[2]

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'repo-alpha',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-main',
  defaultBranch: 'main',
  defaultOwningChatId: '',
  workspaces: [
    { id: 'ws-locked', branch: 'release', age: '', order: 0, status: 'locked', owningChatId: '' },
    { id: 'ws-fork', branch: 'feat', age: '', order: 1, status: 'new', owningChatId: '' },
  ],
  folders: [],
  chats: [],
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  __resetWorkspaceScopesForTest()
  useHomeTreeStore.setState({ trees: {} })
  usePendingCreatesStore.setState(getInitialPendingCreatesState())
  useSidebarStore.setState(getInitialState())
  useSidebarStore.getState().setRepos([repo()])
  useAgentProvidersStore.setState({
    status: 'ready',
    providers: [{ id: 'claude', name: 'Claude', enabled: true } as never],
  })
})

describe('chatless workspace rows — create verbs send the WORKSPACE id as the chat parent', () => {
  it('thread on a chatless repo header posts parentId = the default workspace id', () => {
    handleCreate('ws-main', 'thread', navigate)
    expect(createChat).toHaveBeenCalledWith('ws-main', 'claude', 'ws-main', undefined)
  })

  it('thread on a chatless locked branch posts parentId = that workspace id', () => {
    handleCreate('ws-locked', 'thread', navigate)
    expect(createChat).toHaveBeenCalledWith('ws-locked', 'claude', 'ws-locked', undefined)
  })

  it('new branch off a chatless repo header posts parentId = the default workspace id', () => {
    handleCreate('ws-main', 'workspace', navigate)
    const armed = usePendingCreatesStore.getState().entries.find((e) => e.status === 'naming')!
    expect(armed.parentId).toBe('ws-main')
    confirmPendingCreateName(armed.tempId, 'test/test')
    expect(createChatWithOwnWorktree).toHaveBeenCalledWith(
      'p1',
      'r1',
      'claude',
      'ws-main',
      'test/test',
    )
  })

  it('new branch off a chatless locked branch posts parentId = that workspace id', () => {
    handleCreate('ws-locked', 'workspace', navigate)
    const armed = usePendingCreatesStore.getState().entries.find((e) => e.status === 'naming')!
    confirmPendingCreateName(armed.tempId, 'test/test')
    expect(createChatWithOwnWorktree).toHaveBeenCalledWith(
      'p1',
      'r1',
      'claude',
      'ws-locked',
      'test/test',
    )
  })
})

// Every worktree verb is chat-keyed, and the daemon mints an owner on the
// first read of a chatless workspace — so a row with none recorded is a list
// still landing. The verb sends nothing and says so in the user's words, not
// the scope registry's.
describe('workspace rows whose owning chat has not been recorded yet', () => {
  it('unlock sends nothing and says the chat is still loading', async () => {
    await performSetWorkspaceLock('ws-locked', false)
    expect(apiFetch).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledWith("Can't unlock release yet — its chat is still loading")
  })

  it('rename sends nothing and says the chat is still loading', async () => {
    await performRenameRow('ws-fork', 'renamed')
    expect(apiFetch).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledWith("Can't rename feat yet — its chat is still loading")
  })
})
