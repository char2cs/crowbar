import { beforeEach, describe, expect, it, vi } from 'vitest'

const deleteChat = vi.fn().mockResolvedValue([])
vi.mock('@/features/agent/api/agent-chat-commands', () => ({
  deleteChat: (...a: unknown[]) => deleteChat(...a),
}))
const deleteRepo = vi.fn().mockResolvedValue(undefined)
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  deleteProject: vi.fn(),
  deleteRepo: (...a: unknown[]) => deleteRepo(...a),
}))
const deleteHomeFolder = vi.fn().mockResolvedValue([])
vi.mock('@/lib/api/sidebar-placement', () => ({
  deleteFolder: vi.fn().mockResolvedValue([]),
  deleteHomeFolder: (...a: unknown[]) => deleteHomeFolder(...a),
}))

import { commitRemoval, flushDrainingRemovals } from '@/components/layout/removal-commit'
import {
  expectMigratingShown,
  expectReported,
  migratingRefusal,
  refusal,
  UNWORDED,
  watchToasts,
} from '@/__tests__/__fixtures__/reports'
import { toastManager } from '@/lib/toast-manager'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import { useLocalOperationsStore } from '@/lib/store/local-operations'
import { selectEntityProblem, operationsState, type Operation } from '@/lib/store/operations'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore, type RemovalEntry } from '@/features/workspace/tree/tree-ui-store'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { ApiError, type WorkAtRisk } from '@/lib/errors/api-error'
import { DISCARD_WORK_INIT } from '@/lib/api'
import type { Problem } from '@/lib/errors/problem'

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
let toastError: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  deleteChat.mockClear().mockResolvedValue([])
  deleteRepo.mockClear().mockResolvedValue({ items: [] })
  deleteHomeFolder.mockClear().mockResolvedValue([])
  toastError?.mockRestore()
  toastError = watchToasts()
  resetOperations()
  __resetWorkspaceScopesForTest()
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  useFolderSignalStore.setState({ generations: {}, seededRepoIds: new Set<string>() } as never)
  useSidebarStore.setState({ repos: [repo()] })
  useHomeTreeStore.setState({ trees: {} })
  resetOperations()
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

  // The create's spinner row would otherwise stand in for the deleted chat forever.
  it('dismisses the succeeded create that made what it removed', async () => {
    const created: Operation = {
      ...{ id: 'op-create', kind: 'branch.create', subject: { kind: 'repo', id: 'r1' } },
      ...{ projectId: 'p1', label: 'Create feature/one', state: 'succeeded', dismissed: false },
      items: [{ ref: { kind: 'chat', id: 'chat-fork' }, label: 'feature/one', state: 'succeeded' }],
      ...{ version: 1, createdAt: '', updatedAt: '', local: true },
    }
    const other: Operation = { ...created, id: 'op-other', items: [] }
    useLocalOperationsStore.setState({ ops: { [created.id]: created, [other.id]: other } })

    await commitRemoval(entry(), context)

    expect(operationsState().ops['op-create']?.dismissed).toBe(true)
    expect(operationsState().ops['op-other']?.dismissed).toBe(false)
  })

  // A partly failed import keeps its failed sibling's reason until the user acts on it.
  it('leaves a partly failed create alone when one of its successes is removed', async () => {
    const partial: Operation = {
      ...{ id: 'op-import', kind: 'branch.import', subject: { kind: 'repo', id: 'r1' } },
      ...{ projectId: 'p1', label: 'Import 2 branches', state: 'failed', dismissed: false },
      items: [
        { ref: { kind: 'chat', id: 'chat-fork' }, label: 'feature/one', state: 'succeeded' },
        { ref: { kind: 'chat', id: 'chat-bad' }, label: 'bad/x', state: 'failed' },
      ],
      ...{ version: 1, createdAt: '', updatedAt: '', local: true },
    }
    useLocalOperationsStore.setState({ ops: { [partial.id]: partial } })

    await commitRemoval(entry(), context)

    expect(operationsState().ops['op-import']?.dismissed).toBe(false)
  })

  it('a removal whose fallback is the repo home leaves for it', async () => {
    const navigate = vi.fn()
    await commitRemoval(entry({ fallbackWsId: 'ws-home' }), {
      activeWorkspaceId: 'ws-fork',
      navigate,
    })

    expect(navigate).toHaveBeenCalledExactlyOnceWith({
      projectId: 'p1',
      repoId: 'r1',
      wsId: 'ws-home',
    })
  })

  it('un-hides the rows and says why when the delete is refused', async () => {
    deleteChat.mockRejectedValueOnce(refusal('worktree is busy'))
    treeUiStore.getState().hold([
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

    expect(treeUiStore.getState().hiddenIds.has('ws-fork')).toBe(false)
    expect(treeUiStore.getState().hiddenIds.has('chat-fork')).toBe(false)
    expectReported(toastError, "Couldn't remove feature/one", 'worktree is busy')
    deleteChat.mockRejectedValueOnce(UNWORDED)
    await commitRemoval(entry({ entryId: 'e2' }), context)
    expectReported(toastError, "Couldn't remove feature/one", 'request failed')
  })

  it('a delete refused while the daemon migrates shows the migrating state, not a toast', async () => {
    deleteChat.mockRejectedValueOnce(migratingRefusal())
    await commitRemoval(entry(), context)
    expectMigratingShown(toastError)
  })

  it('names the chats the delete took beyond the selection, not those it held', async () => {
    const child = { id: 'chat-child', repoId: 'r1', title: 'Child', order: 1 }
    useSidebarStore.setState({ repos: [repo({ chats: [...repo().chats!, child] })] })
    deleteChat.mockResolvedValueOnce(['chat-child'])

    await commitRemoval(entry(), context)

    expect(toastError).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'info', title: 'Also removed 1 chat', description: 'Child' }),
    )
    deleteChat.mockResolvedValueOnce(['chat-fork'])
    await commitRemoval(entry({ entryId: 'e2' }), context)
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

// Regression: project-home chat/folder removal previously never reached
// this module at all (`handleTrash` refused it outright, reported live as
// "Can't delete X yet") — `removal-plan.ts` now builds a real draft for one
// (`repoId: ''`), and these pin that THIS module fires the right DELETE for
// it: `deleteChat`/`deleteHomeFolder` scoped by project/home-workspace, not
// the repo-scoped calls a `repoId: ''` entry would otherwise 404 or (worse,
// per handleTrash's own doc on the folder-list bleed) misresolve against.
describe('committing a project-home removal', () => {
  it('a home chat deletes through its own home workspace, never bumping a repo signal that has none to bump', async () => {
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 't', order: 0 }],
          folders: [],
        },
      },
    })
    const before = useFolderSignalStore.getState().generations['r1'] ?? 0

    await commitRemoval(
      entry({
        kind: 'chat',
        id: 'c1',
        projectId: 'p1',
        repoId: '',
        wsId: 'home-ws-1',
        hiddenIds: ['c1'],
      }),
      context,
    )

    expect(deleteChat).toHaveBeenCalledExactlyOnceWith('home-ws-1', 'c1')
    expect(toastError).not.toHaveBeenCalled()
    // bumpRepoTree's own no-op guard: there is no such repo.
    expect(useFolderSignalStore.getState().generations['r1'] ?? 0).toBe(before)
  })

  it('a home folder deletes through deleteHomeFolder, and its tombstone is applied directly (no live channel to wait on)', async () => {
    useHomeTreeStore.setState({
      trees: { p1: { chats: [], folders: [{ id: 'f1', repoId: '', name: 'Notes', order: 0 }] } },
    })

    await commitRemoval(
      entry({
        kind: 'folder',
        id: 'f1',
        label: 'Notes',
        projectId: 'p1',
        repoId: '',
        wsId: '',
        hiddenIds: ['f1'],
      }),
      context,
    )

    expect(deleteHomeFolder).toHaveBeenCalledExactlyOnceWith('p1', 'f1')
    expect(useHomeTreeStore.getState().trees.p1?.folders).toEqual([])
  })

  it('un-hides a home chat and says why when its delete is refused', async () => {
    deleteChat.mockRejectedValueOnce(new Error('not found'))
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [{ id: 'c1', repoId: '', workspaceId: 'home-ws-1', title: 't', order: 0 }],
          folders: [],
        },
      },
    })
    treeUiStore.getState().hold([
      {
        kind: 'chat',
        id: 'c1',
        label: 't',
        projectId: 'p1',
        repoId: '',
        wsId: 'home-ws-1',
        providerIcon: '',
        hiddenIds: ['c1'],
        extra: 0,
        fallbackWsId: null,
      },
    ])

    await commitRemoval(
      entry({
        kind: 'chat',
        id: 'c1',
        projectId: 'p1',
        repoId: '',
        wsId: 'home-ws-1',
        hiddenIds: ['c1'],
      }),
      context,
    )

    expect(treeUiStore.getState().hiddenIds.has('c1')).toBe(false)
    expect(toastError).toHaveBeenCalledOnce()
  })
})

// A repo or project delete is an operation; the rows wait on it.
describe('committing a repo removal', () => {
  const hideRepo = () => treeUiStore.setState({ hiddenIds: new Set(['r1']) })
  const repoEntry = () =>
    entry({ kind: 'repo', id: 'r1', label: 'checkout', hiddenIds: ['r1'], deadlineAt: null })
  const removeOp = (opId: string, version: number, over: Partial<Operation> = {}): Operation => ({
    id: opId,
    kind: 'repo.delete',
    subject: { kind: 'repo', id: 'r1' },
    projectId: 'p1',
    label: 'Remove checkout',
    state: 'running',
    items: [{ ref: { kind: 'repo', id: 'r1' }, label: 'checkout', state: 'running' }],
    dismissed: false,
    version,
    createdAt: '2026-10-30T10:00:00Z',
    updatedAt: '2026-10-30T10:00:00Z',
    ...over,
  })
  const sentOpId = (): string => deleteRepo.mock.calls[0][2] as string

  beforeEach(() => {
    deleteRepo.mockImplementation(async (_p: string, _r: string, opId: string) => ({
      operation: removeOp(opId, 1),
    }))
  })

  it('a remove releases the tray once the repo is gone', async () => {
    hideRepo()
    await commitRemoval(repoEntry(), context)
    streamOperation(removeOp(sentOpId(), 2, { state: 'succeeded' }))
    expect(treeUiStore.getState().hiddenIds.has('r1')).toBe(true)

    useSidebarStore.setState({ repos: [] })

    await vi.waitFor(() => expect(treeUiStore.getState().hiddenIds.has('r1')).toBe(false))
    expect(toastError).not.toHaveBeenCalled()
  })

  it('a failed remove restores the row and reports the reason', async () => {
    hideRepo()
    await commitRemoval(repoEntry(), context)
    const reason: Problem = { code: 'internal', message: 'worktree wedged' }
    streamOperation(removeOp(sentOpId(), 2, { state: 'failed', error: reason }))

    await vi.waitFor(() => expect(treeUiStore.getState().hiddenIds.has('r1')).toBe(false))
    expect(selectEntityProblem(operationsState(), 'r1')).toEqual(reason)
    // The operations watcher tells it, once.
    expect(toastError).not.toHaveBeenCalled()
  })
})

describe('a removal sent as the page unloads', () => {
  it('an unload-time remove that fails is reported', async () => {
    const add = vi.spyOn(toastManager, 'add')
    deleteChat.mockRejectedValueOnce(new ApiError('daemon is gone', 500))
    treeUiStore.getState().hold([entry()])

    flushDrainingRemovals()

    expect(deleteChat).toHaveBeenCalledWith('ws-fork', 'chat-fork', { keepalive: true })
    await vi.waitFor(() =>
      expect(add).toHaveBeenCalledWith(
        expect.objectContaining({ title: 'Remove', description: 'daemon is gone' }),
      ),
    )
    add.mockRestore()
  })
})

// The daemon refuses a delete that would destroy work existing nowhere else.
// That refusal is a question for the user, never a silent failure or a retry.
describe('a removal refused over work at risk', () => {
  const lost: WorkAtRisk[] = [
    { workspaceId: 'ws-fork', branch: 'feature/one', uncommittedFiles: 2, unmergedCommits: 1 },
  ]
  const refusal = () =>
    new ApiError('delete would destroy work', 409, 'work_at_risk', { workAtRisk: lost })

  it('goes back in the tray, rows still hidden, waiting on an answer with the list', async () => {
    deleteChat.mockRejectedValueOnce(refusal())
    treeUiStore.getState().hold([entry()])
    const [held] = treeUiStore.getState().removals

    await commitRemoval(held, context)

    expect(treeUiStore.getState().removals).toHaveLength(1)
    const [asked] = treeUiStore.getState().removals
    expect(asked?.deadlineAt).toBeNull()
    expect(asked?.atRisk).toEqual(lost)
    expect(treeUiStore.getState().hiddenIds.has('ws-fork')).toBe(true)
    expect(toastError).not.toHaveBeenCalled()
  })

  it('once confirmed, is sent with consent to discard exactly that work', async () => {
    await commitRemoval(entry({ atRisk: lost, deadlineAt: null }), context)
    await commitRemoval(
      entry({ kind: 'repo', id: 'r1', hiddenIds: ['r1'], atRisk: lost, deadlineAt: null }),
      context,
    )

    expect(deleteChat).toHaveBeenCalledExactlyOnceWith('ws-fork', 'chat-fork', DISCARD_WORK_INIT)
    expect(deleteRepo).toHaveBeenCalledExactlyOnceWith(
      'p1',
      'r1',
      expect.any(String),
      DISCARD_WORK_INIT,
    )
  })

  it('an ordinary removal never carries that consent', async () => {
    await commitRemoval(entry({ kind: 'repo', id: 'r1', hiddenIds: ['r1'] }), context)

    expect(deleteRepo).toHaveBeenCalledExactlyOnceWith('p1', 'r1', expect.any(String))
  })
})
