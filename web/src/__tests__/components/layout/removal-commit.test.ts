import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/persistence/sidebar-ui', () => ({
  saveSidebarUI: vi.fn().mockResolvedValue(undefined),
  loadSidebarUI: vi.fn().mockResolvedValue(null),
}))

const deleteChat = vi.fn().mockResolvedValue(undefined)
vi.mock('@/features/agent/api/agent-api', () => ({
  deleteChat: (...a: unknown[]) => deleteChat(...a),
}))
vi.mock('@/lib/api', () => ({ deleteProject: vi.fn(), deleteRepo: vi.fn() }))
vi.mock('@/lib/api/sidebar-placement', () => ({ deleteFolder: vi.fn() }))

const toastError = vi.fn()
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastError(...a) },
}))

import { commitRemoval } from '@/components/layout/removal-commit'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import {
  useRemovalTrayStore,
  getInitialRemovalState,
  type RemovalEntry,
} from '@/lib/store/sidebar-removal'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'

/**
 * COMMITTING A REMOVAL — the one step that destroys anything.
 *
 * Two things had to be true for "I deleted these chats and they came back",
 * and both are pinned here:
 *
 *   1. the DELETE has to be addressed correctly, from a source of truth that
 *      is actually populated for a row the user has only ever SEEN;
 *   2. something has to tell the sidebar to re-read its rows afterwards, or
 *      the chat half of the deleted row paints again the moment the optimistic
 *      hide releases.
 */

function repo(over: Partial<Repo> = {}): Repo {
  return {
    id: 'r1',
    projectId: 'p1',
    name: 'checkout',
    avatarLabel: 'C',
    avatarColor: 'avatar-amber',
    order: 0,
    defaultWorkspaceId: 'ws-home',
    workspaces: [
      { id: 'ws-fork', branch: 'feature/one', age: '', order: 0, owningChatId: 'chat-fork' },
    ],
    chats: [
      {
        id: 'chat-fork',
        repoId: 'r1',
        type: 'chat',
        workspaceId: 'ws-fork',
        ownsWorktree: true,
        title: 'One',
        order: 0,
      },
    ],
    ...over,
  }
}

function entry(over: Partial<RemovalEntry> = {}): RemovalEntry {
  return {
    entryId: 'e1',
    kind: 'workspace',
    id: 'ws-fork',
    label: 'feature/one',
    projectId: 'p1',
    repoId: 'r1',
    wsId: '',
    providerIcon: '',
    hiddenIds: ['ws-fork', 'chat-fork'],
    extra: 0,
    fallbackWsId: null,
    deadlineAt: Date.now(),
    ...over,
  }
}

const context = { activeWorkspaceId: '', navigate: vi.fn() }

beforeEach(() => {
  deleteChat.mockClear().mockResolvedValue(undefined)
  toastError.mockClear()
  __resetWorkspaceScopesForTest()
  useRemovalTrayStore.setState(getInitialRemovalState())
  useFolderSignalStore.setState({ generations: {}, seededRepoIds: new Set<string>() } as never)
  useSidebarStore.setState({ repos: [repo()] })
})

describe('committing a workspace removal', () => {
  /**
   * THE FALSE NEGATIVE. `getOwningChatId` reads `workspace-scope.ts`'s own
   * module-level registry — a SECOND copy of "who owns this workspace",
   * written on navigation and on seed. A workspace the user has only ever seen
   * as a row, never opened, can legitimately be absent from it. Asking it first
   * turned that absence into a rejected delete on a perfectly valid row; the
   * row had already been optimistically hidden, so the removal looked like it
   * worked right up until the next reseed brought it straight back.
   *
   * No scope is recorded in this test AT ALL, which is the point.
   */
  it('addresses the owning chat from the sidebar tree, with no scope recorded', async () => {
    await commitRemoval(entry(), context)

    expect(deleteChat).toHaveBeenCalledExactlyOnceWith('ws-fork', 'chat-fork')
    expect(toastError).not.toHaveBeenCalled()
  })

  /** The same answer via `Chat.ownsWorktree`, for the window where the
   *  `Workspace` record has not landed but the chat has. */
  it('falls back to the chat’s own ownership claim when the Workspace has no owningChatId', async () => {
    const half = repo()
    half.workspaces = [{ id: 'ws-fork', branch: 'feature/one', age: '', order: 0 }]
    useSidebarStore.setState({ repos: [half] })

    await commitRemoval(entry(), context)

    expect(deleteChat).toHaveBeenCalledExactlyOnceWith('ws-fork', 'chat-fork')
  })

  /**
   * Without this the daemon really did take the chat and the ROW stayed on
   * screen: `openRepoTreeSubscription` reseeds `crowbar_chats` only when this
   * generation moves, and the only thing that normally moves it is a chat frame
   * arriving for a MOUNTED workspace of the repo. The sidebar deletes from
   * routes where no workspace of that repo is mounted at all.
   */
  it('bumps the repo’s tree signal so the chat row actually leaves', async () => {
    const before = useFolderSignalStore.getState().generations['r1'] ?? 0

    await commitRemoval(entry(), context)

    expect(useFolderSignalStore.getState().generations['r1'] ?? 0).toBeGreaterThan(before)
  })

  it('bumps it for a bare chat removal too', async () => {
    const before = useFolderSignalStore.getState().generations['r1'] ?? 0

    await commitRemoval(
      entry({ kind: 'chat', id: 'chat-bubble', wsId: 'ws-home', hiddenIds: ['chat-bubble'] }),
      context,
    )

    expect(deleteChat).toHaveBeenCalledExactlyOnceWith('ws-home', 'chat-bubble')
    expect(useFolderSignalStore.getState().generations['r1'] ?? 0).toBeGreaterThan(before)
  })

  it('un-hides the rows and says why when the delete is refused', async () => {
    deleteChat.mockRejectedValueOnce(new Error('worktree is busy'))
    useRemovalTrayStore.getState().hold([
      {
        kind: 'workspace',
        id: 'ws-fork',
        label: 'feature/one',
        projectId: 'p1',
        repoId: 'r1',
        wsId: '',
        providerIcon: '',
        hiddenIds: ['ws-fork', 'chat-fork'],
        extra: 0,
        fallbackWsId: null,
      },
    ])

    await commitRemoval(entry(), context)

    expect(useRemovalTrayStore.getState().hiddenIds.has('ws-fork')).toBe(false)
    expect(useRemovalTrayStore.getState().hiddenIds.has('chat-fork')).toBe(false)
    expect(toastError).toHaveBeenCalledOnce()
  })

  it('still refuses when nothing anywhere knows the owning chat', async () => {
    useSidebarStore.setState({
      repos: [repo({ workspaces: [{ id: 'ws-fork', branch: 'x', age: '', order: 0 }], chats: [] })],
    })

    await commitRemoval(entry(), context)

    expect(deleteChat).not.toHaveBeenCalled()
    expect(toastError).toHaveBeenCalledOnce()
  })
})
