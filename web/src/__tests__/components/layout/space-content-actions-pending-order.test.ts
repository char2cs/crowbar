import { describe, expect, it, vi, beforeEach } from 'vitest'

const { createChat, getHomeWorkspaceId } = vi.hoisted(() => ({
  createChat: vi.fn(() => new Promise<string>(() => {})),
  getHomeWorkspaceId: vi.fn(() => 'home-ws-1'),
}))

vi.mock('@/features/agent/api/agent-api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/api/agent-api')>()),
  createChat,
}))
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => 'home-owner',
}))

import { handleCreate, handleCreateHomeThread } from '@/components/layout/space-content-actions'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { usePendingCreatesStore, getInitialPendingCreatesState } from '@/lib/store/pending-creates'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'home-1',
  defaultOwningChatId: 'c-home',
  defaultBranch: 'main',
  order: 1,
  workspaces: [],
  folders: [],
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useSidebarStore.setState(getInitialState())
  useHomeTreeStore.setState({ trees: {} })
  usePendingCreatesStore.setState(getInitialPendingCreatesState())
  useAgentProvidersStore.setState({
    status: 'ready',
    providers: [{ id: 'claude', enabled: true }] as never,
  })
})

/**
 * The pending row is drawn "at the exact slot the finished create will land
 * in" (pending-creates.ts). The daemon appends a create at NextSlot = the
 * COUNT OF EVERY row in the destination container, any kind (tree/sibling_tree.go;
 * the home level counts repo headers too). The thread path counts only `chat`
 * siblings and the header path only home chats/folders, so on any level that
 * also holds forks, folders or repos the placeholder sits slots above where
 * the real row lands, then jumps down when it appears.
 */
describe('a pending thread row is placed where the daemon will append it', () => {
  it('counts every sibling under a branch row — forks and folders included, not chats alone', () => {
    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [
            { id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-a' },
            {
              id: 'ws-b',
              branch: 'beta',
              age: '',
              order: 0,
              parentId: 'ws-a',
              owningChatId: 'c-b',
            },
          ],
          folders: [{ id: 'f1', repoId: 'r1', name: 'notes', parentId: 'ws-a', order: 1 }],
          chats: [
            {
              id: 'c-a',
              repoId: 'r1',
              workspaceId: 'ws-a',
              ownsWorktree: true,
              title: 'A',
              order: 0,
            },
            {
              id: 'c-b',
              repoId: 'r1',
              workspaceId: 'ws-b',
              ownsWorktree: true,
              title: 'B',
              order: 0,
            },
            {
              id: 'c-t',
              repoId: 'r1',
              workspaceId: 'ws-a',
              parentId: 'c-a',
              title: 'thread',
              order: 2,
            },
          ],
        }),
      ],
    })

    handleCreate('c-a', 'thread', vi.fn())

    const [entry] = usePendingCreatesStore.getState().entries
    expect(entry).toMatchObject({ kind: 'chat', parentId: 'c-a' })
    // ws-b (fork), f1 (folder) and c-t (thread) already sit under c-a.
    expect(entry.order).toBe(3)
  })

  it('counts the repo headers sharing the project-home root, not only home chats and folders', async () => {
    useSidebarStore.setState({ repos: [repo({ order: 1 })] })
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [
            {
              id: 'home-owner',
              repoId: '',
              workspaceId: 'home-ws-1',
              title: '',
              order: 0,
              ownsWorktree: true,
            },
            { id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 'one', order: 0 },
          ],
          folders: [{ id: 'f1', repoId: '', name: 'notes', order: 2 }],
        },
      },
    })

    void handleCreateHomeThread('p1', 'home-ws-1', vi.fn())
    await Promise.resolve()

    const [entry] = usePendingCreatesStore.getState().entries
    expect(entry).toMatchObject({ kind: 'chat', parentId: '' })
    // c1, the crowbar repo header and f1 share the home root; the owner is not a row.
    expect(entry.order).toBe(3)
  })
})
