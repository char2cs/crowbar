import { describe, expect, it, vi, beforeEach } from 'vitest'

const { createChat, createChatWithOwnWorktree, getHomeWorkspaceId } = vi.hoisted(() => ({
  createChat: vi.fn((id: string) => Promise.resolve(id)),
  createChatWithOwnWorktree: vi.fn((id: string) => Promise.resolve({ id, workspaceId: 'ws-new' })),
  getHomeWorkspaceId: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-chat-creates', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-chat-creates')>()),
  createChat,
  createChatWithOwnWorktree,
}))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

import { handleCreate } from '@/components/layout/create-actions'
import { deriveRows } from '@/features/workspace/tree/rows'
import { inputOf, repoRows } from '@/__tests__/__fixtures__/tree-rows'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { useCreateDraftStore } from '@/lib/store/create-draft'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

const navigate = vi.fn(() => Promise.resolve()) as unknown as Parameters<typeof handleCreate>[2]

// GET .../workspaces minted the owner ('owner-locked') on this read, but the
// chat list this repo rendered from predates the mint (it was fetched in
// parallel, and the mint's structural frames raced the socket handshake).
const repo = (): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'repo-alpha',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-main',
  defaultBranch: 'main',
  defaultOwningChatId: 'owner-main',
  workspaces: [
    {
      id: 'ws-locked',
      branch: 'release',
      age: '',
      order: 0,
      status: 'locked',
      owningChatId: 'owner-locked',
    },
  ],
  folders: [],
  chats: [],
})

beforeEach(() => {
  vi.clearAllMocks()
  __resetWorkspaceScopesForTest()
  useHomeTreeStore.setState({ trees: {} })
  useCreateDraftStore.setState({ draft: null })
  useSidebarStore.setState(getInitialState())
  useSidebarStore.getState().setRepos([repo()])
  useAgentProvidersStore.setState({
    status: 'ready',
    providers: [{ id: 'claude', name: 'Claude', enabled: true } as never],
  })
})

describe('owner known to the workspace DTO but absent from the chat list', () => {
  it('renders the locked branch by its owner id, labelled by its branch', () => {
    const rows = repoRows(repo())
    const row = rows.find((r) => r.id === 'owner-locked')
    expect(row).toMatchObject({ kind: 'branch', workspaceId: 'ws-locked', label: 'release' })
    expect(rows.map((r) => r.id)).not.toContain('ws-locked')
  })

  it('fork off that row parents the naming input at the rendered row, not at an unlisted chat', () => {
    handleCreate('owner-locked', 'workspace', navigate)
    const draft = useCreateDraftStore.getState().draft!
    const rendered = deriveRows(inputOf({ repos: [repo()], draft }))
    const ids = new Set(rendered.map((r) => r.id))
    // sidebar-tree.tsx roots any row whose parentId names no rendered row.
    expect(ids.has(draft.parentId)).toBe(true)
  })
})
