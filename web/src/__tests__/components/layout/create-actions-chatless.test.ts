import { describe, expect, it, vi, beforeEach } from 'vitest'

const { createChat, createChatWithOwnWorktree, getHomeWorkspaceId, apiFetch } = vi.hoisted(() => ({
  createChat: vi.fn((id: string) => Promise.resolve(id)),
  createChatWithOwnWorktree: vi.fn((id: string) => Promise.resolve({ id, workspaceId: 'ws-new' })),
  getHomeWorkspaceId: vi.fn(),
  apiFetch: vi.fn(() => Promise.resolve(undefined)),
}))

vi.mock('@/lib/api/fetch', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/fetch')>()),
  apiFetch,
}))
vi.mock('@/features/agent/api/agent-chat-creates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-chat-creates')>()),
  createChat,
  createChatWithOwnWorktree,
}))
import { toast } from '@/features/window/stores/toast-store'
const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { handleCreate, confirmCreateDraft } from '@/components/layout/create-actions'
import { performSetWorkspaceLock, performRenameRow } from '@/components/sidebar/lib/row-actions'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'
import type { Repo } from '@/core/entities/repo-types'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

const ANY = expect.any(String)
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
  treeUiStore.setState({ draft: null })
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
    expect(createChat).toHaveBeenCalledWith(ANY, ANY, 'ws-main', 'claude', 'ws-main', undefined)
  })

  it('thread on a chatless locked branch posts parentId = that workspace id', () => {
    handleCreate('ws-locked', 'thread', navigate)
    expect(createChat).toHaveBeenCalledWith(ANY, ANY, 'ws-locked', 'claude', 'ws-locked', undefined)
  })

  it('new branch off a chatless repo header posts parentId = the default workspace id', () => {
    handleCreate('ws-main', 'workspace', navigate)
    expect(treeUiStore.getState().draft?.parentId).toBe('ws-main')
    confirmCreateDraft('test/test', navigate)
    expect(createChatWithOwnWorktree).toHaveBeenCalledWith(
      ANY,
      ANY,
      'p1',
      'r1',
      'claude',
      'ws-main',
      'test/test',
    )
  })

  it('new branch off a chatless locked branch posts parentId = that workspace id', () => {
    handleCreate('ws-locked', 'workspace', navigate)
    confirmCreateDraft('test/test', navigate)
    expect(createChatWithOwnWorktree).toHaveBeenCalledWith(
      ANY,
      ANY,
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
