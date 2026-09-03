/**
 * The Chats tree's write path: a drop, a new folder, a rename.
 *
 * The source's own doc comment states the invariants this file pins:
 *  - a drop is OPTIMISTIC first, then the calls go out IN ORDER (each `order`
 *    indexes the level as it stands once the previous call has landed);
 *  - a refusal snaps the WHOLE level back — the moved rows AND the siblings
 *    they displaced — and raises a toast;
 *  - a drop that changes nothing sends nothing;
 *  - the daemon's answer is folded back in, including the `shifted` rows a
 *    dense renumber moved that nobody asked about;
 *  - deleting a folder promotes its children and, on refusal, restores BOTH
 *    the folder and every child's placement;
 *  - createChatFolderRow hands back '' (and toasts) on a refusal, so no
 *    rename editor opens on a row that was never created.
 *
 * `@/features/agent/tree/lib/chat-drop` (chatDropPlan) runs for REAL — only the
 * daemon calls and the toast are mocked — so these tests exercise the actual
 * before/after/into arithmetic a real drag would trigger, not a stand-in.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { setChatPlacement, updateChatFolder, createChatFolder, deleteChatFolder, toastError } =
  vi.hoisted(() => ({
    setChatPlacement: vi.fn(),
    updateChatFolder: vi.fn(),
    createChatFolder: vi.fn(),
    deleteChatFolder: vi.fn(),
    toastError: vi.fn(),
  }))

vi.mock('@/features/agent/api/agent-api', () => ({
  setChatPlacement,
  updateChatFolder,
  createChatFolder,
  deleteChatFolder,
}))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

// The real workspace store, via the same registry the panel uses (see
// agent-chats-panel-rerender.test.tsx) — chat-tree-commit.ts is written
// against a live WorkspaceStore, not a mock of one. Its own inner
// persistence subscriptions are stubbed so a folder move doesn't try to
// write a layout / IndexedDB session in a jsdom test.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import {
  applyChatMoves,
  captureChatMoves,
  commitChatDrop,
  createChatFolderRow,
  groupIntoChatFolder,
  NEW_FOLDER_NAME,
  renameChatFolderRow,
} from '@/features/agent/tree/lib/chat-tree-commit'
import type { ChatDragSubject, ResolvedChatDrop } from '@/features/agent/tree/lib/chat-drop'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { AgentChat, AgentChatFolder } from '@/features/agent/api/agent-api'

const WS = 'w-chat-tree'

function chat(
  id: string,
  parentId: string,
  order: number,
  overrides: Partial<AgentChat> = {},
): AgentChat {
  return {
    id,
    workspaceId: WS,
    title: id,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: 'claude',
    createdAt: '2026-01-01T00:00:00Z',
    parentId,
    order,
    ...overrides,
  }
}

function folder(id: string, parentId: string, order: number, name = id): AgentChatFolder {
  return { id, workspaceId: WS, parentId, name, order }
}

/** A promise this test settles by hand, to prove call B did not fire before call A resolved. */
function deferred<T>() {
  let resolve!: (v: T) => void
  let reject!: (e: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

let store: WorkspaceStore

beforeEach(() => {
  store = getOrCreateWorkspaceStore(WS)
})

afterEach(() => {
  destroyWorkspaceStore(WS)
  vi.clearAllMocks()
})

describe('applyChatMoves', () => {
  it('paints a chat move via setAgentChatPlacement and a folder move via the merged upsert, in one call', () => {
    store.getState().seedAgentChats([chat('c1', '', 0)])
    store.getState().seedAgentChatFolders([folder('f1', '', 1)])

    applyChatMoves(store, [
      { id: 'c1', parentId: 'f1', order: 0 },
      { id: 'f1', parentId: '', order: 0 },
    ])

    expect(store.getState().agentChats.chats[0]).toMatchObject({ parentId: 'f1', order: 0 })
    expect(store.getState().agentChats.folders[0]).toMatchObject({ parentId: '', order: 0 })
  })

  it('skips the folders upsert entirely when every move in the batch is a chat', () => {
    store.getState().seedAgentChats([chat('c1', '', 0), chat('c2', '', 1)])

    // Neither id resolves to a folder, so the `next` batch built for
    // applyAgentChatFolders stays empty and that call must not fire.
    applyChatMoves(store, [
      { id: 'c1', parentId: '', order: 1 },
      { id: 'c2', parentId: '', order: 0 },
    ])

    expect(store.getState().agentChats.chats.find((c) => c.id === 'c1')).toMatchObject({ order: 1 })
    expect(store.getState().agentChats.chats.find((c) => c.id === 'c2')).toMatchObject({ order: 0 })
    expect(store.getState().agentChats.folders).toEqual([])
  })
})

describe('captureChatMoves', () => {
  it('captures the current placement of chats and folders alike', () => {
    store.getState().seedAgentChats([chat('c1', 'f1', 3)])
    store.getState().seedAgentChatFolders([folder('f1', '', 0)])

    const out = captureChatMoves(store, [{ id: 'c1' }, { id: 'f1' }])

    expect(out).toEqual([
      { id: 'c1', parentId: 'f1', order: 3 },
      { id: 'f1', parentId: '', order: 0 },
    ])
  })

  it('skips a row that is not in the store rather than inventing a placement for it', () => {
    const out = captureChatMoves(store, [{ id: 'ghost' }])
    expect(out).toEqual([])
  })

  it('grounds a row whose parentId is truly absent (undefined), not merely "", to the root', () => {
    // A row built before the tree existed, or off an older store snapshot,
    // may never have had parentId set at all — distinct from '' (root,
    // explicitly). Both must capture the same way.
    store.getState().seedAgentChats([{ ...chat('c1', '', 0), parentId: undefined }])
    const out = captureChatMoves(store, [{ id: 'c1' }])
    expect(out).toEqual([{ id: 'c1', parentId: '', order: 0 }])
  })
})

describe('commitChatDrop', () => {
  // Root level: c1, c2, f1 — used by most of the scenarios below.
  function seedRootLevel() {
    store.getState().seedAgentChats([chat('c1', '', 0), chat('c2', '', 1)])
    store.getState().seedAgentChatFolders([folder('f1', '', 2, 'F1')])
  }

  it('sends nothing and touches nothing when the drop would change nothing', async () => {
    seedRootLevel()
    const subjects: ChatDragSubject[] = [{ id: 'c1', kind: 'chat', parentId: '' }]
    // c1 is already immediately before c2 — dropping it "before c2" again is
    // the commonest drag accident and must not become a write.
    const target: ResolvedChatDrop = { id: 'c2', kind: 'chat', parentId: '', mode: 'before' }
    const siblings = new Map([['', ['c1', 'c2', 'f1']]])

    await commitChatDrop(store, WS, subjects, target, siblings)

    expect(setChatPlacement).not.toHaveBeenCalled()
    expect(updateChatFolder).not.toHaveBeenCalled()
    expect(toastError).not.toHaveBeenCalled()
    expect(store.getState().agentChats.chats.map((c) => [c.id, c.order])).toEqual([
      ['c1', 0],
      ['c2', 1],
    ])
    expect(store.getState().agentChats.folders[0]).toMatchObject({ order: 2 })
  })

  it('paints the level OPTIMISTICALLY before any call resolves, then fires the calls IN ORDER (chat, then folder) — never concurrently', async () => {
    seedRootLevel()
    // Drag c1 AND f1 together, dropping them before c2. Multi-row so the
    // calls array holds one chat call and one folder call, in that order.
    const subjects: ChatDragSubject[] = [
      { id: 'c1', kind: 'chat', parentId: '' },
      { id: 'f1', kind: 'chatFolder', parentId: '' },
    ]
    const target: ResolvedChatDrop = { id: 'c2', kind: 'chat', parentId: '', mode: 'before' }
    const siblings = new Map([['', ['c1', 'c2', 'f1']]])

    const chatCall = deferred<{ chat: AgentChat; shifted: AgentChatFolder[] }>()
    setChatPlacement.mockReturnValueOnce(chatCall.promise)
    updateChatFolder.mockResolvedValueOnce({ folder: folder('f1', '', 1), shifted: [] })

    const commitPromise = commitChatDrop(store, WS, subjects, target, siblings)

    // The optimistic paint already landed — c1, f1, c2 renumbered to 0,1,2 —
    // synchronously, before the first daemon call has even resolved.
    expect(store.getState().agentChats.chats.find((c) => c.id === 'c1')).toMatchObject({
      parentId: '',
      order: 0,
    })
    expect(store.getState().agentChats.folders.find((f) => f.id === 'f1')).toMatchObject({
      parentId: '',
      order: 1,
    })
    expect(store.getState().agentChats.chats.find((c) => c.id === 'c2')).toMatchObject({
      parentId: '',
      order: 2,
    })

    // The chat call fired; the folder call must NOT have — it is next in
    // line, waiting on the chat call to land first.
    expect(setChatPlacement).toHaveBeenCalledTimes(1)
    expect(updateChatFolder).not.toHaveBeenCalled()

    chatCall.resolve({ chat: chat('c1', '', 0), shifted: [] })
    await commitPromise

    expect(updateChatFolder).toHaveBeenCalledTimes(1)
  })

  it('folds the daemon chat answer AND the shifted folder rows its renumber moved back into the store', async () => {
    seedRootLevel()
    const subjects: ChatDragSubject[] = [{ id: 'c2', kind: 'chat', parentId: '' }]
    // Drop c2 INTO the (currently empty) folder f1.
    const target: ResolvedChatDrop = { id: 'f1', kind: 'chatFolder', parentId: '', mode: 'into' }
    const siblings = new Map([
      ['', ['c1', 'c2', 'f1']],
      ['f1', []],
    ])

    setChatPlacement.mockResolvedValueOnce({
      // The server's answer disagrees with the local guess (a different
      // title) — proves the fold-back OVERWRITES the optimistic paint
      // rather than the client's own guess winning.
      chat: chat('c2', 'f1', 0, { title: 'C2 (server)' }),
      // f2 was never in this store before — the renumber a sibling folder
      // level triggered is applied as an INSERT via applyAgentChatFolders.
      shifted: [folder('f2', '', 9)],
    })

    await commitChatDrop(store, WS, subjects, target, siblings)

    const c2 = store.getState().agentChats.chats.find((c) => c.id === 'c2')
    expect(c2).toMatchObject({ parentId: 'f1', order: 0, title: 'C2 (server)' })
    expect(store.getState().agentChats.folders.find((f) => f.id === 'f2')).toMatchObject({
      order: 9,
    })
  })

  it('a refusal snaps the WHOLE level back — the moved rows AND the siblings they displaced — and raises a toast with the daemon message', async () => {
    seedRootLevel()
    const subjects: ChatDragSubject[] = [{ id: 'c1', kind: 'chat', parentId: '' }]
    // Drop c1 after f1 (the last row) — displaces c2 and f1 down a slot each,
    // though only c1 itself is ever sent to the daemon.
    const target: ResolvedChatDrop = { id: 'f1', kind: 'chatFolder', parentId: '', mode: 'after' }
    const siblings = new Map([['', ['c1', 'c2', 'f1']]])

    setChatPlacement.mockRejectedValueOnce(new Error('placement refused'))

    await commitChatDrop(store, WS, subjects, target, siblings)

    expect(toastError).toHaveBeenCalledWith('placement refused')
    // Every row is back where it started — including c2 and f1, which never
    // had a call of their own sent for the daemon to refuse.
    expect(store.getState().agentChats.chats.find((c) => c.id === 'c1')).toMatchObject({ order: 0 })
    expect(store.getState().agentChats.chats.find((c) => c.id === 'c2')).toMatchObject({ order: 1 })
    expect(store.getState().agentChats.folders.find((f) => f.id === 'f1')).toMatchObject({
      order: 2,
    })
  })

  it('a non-Error refusal falls back to a generic toast message', async () => {
    seedRootLevel()
    const subjects: ChatDragSubject[] = [{ id: 'c1', kind: 'chat', parentId: '' }]
    const target: ResolvedChatDrop = { id: 'f1', kind: 'chatFolder', parentId: '', mode: 'after' }
    const siblings = new Map([['', ['c1', 'c2', 'f1']]])

    setChatPlacement.mockRejectedValueOnce('nope') // a rejection that is not an Error

    await commitChatDrop(store, WS, subjects, target, siblings)

    expect(toastError).toHaveBeenCalledWith('That move was refused')
  })
})

describe('createChatFolderRow', () => {
  it('creates the folder under parentId and applies both the folder and everything its renumber shifted', async () => {
    store.getState().seedAgentChatFolders([folder('f1', '', 0)])
    createChatFolder.mockResolvedValueOnce({
      folder: folder('fNew', '', 1, NEW_FOLDER_NAME),
      shifted: [folder('f1', '', 0)],
    })

    const id = await createChatFolderRow(store, WS, '')

    expect(createChatFolder).toHaveBeenCalledWith(WS, NEW_FOLDER_NAME, '')
    expect(id).toBe('fNew')
    expect(store.getState().agentChats.folders.find((f) => f.id === 'fNew')?.name).toBe(
      NEW_FOLDER_NAME,
    )
  })

  it('returns "" and toasts on refusal — so the caller opens no rename editor on a row that was never created', async () => {
    store.getState().seedAgentChatFolders([folder('f1', '', 0)])
    createChatFolder.mockRejectedValueOnce(new Error('quota reached'))

    const id = await createChatFolderRow(store, WS, '')

    expect(id).toBe('')
    expect(toastError).toHaveBeenCalledWith('quota reached')
    // Nothing was written to the store on a refusal.
    expect(store.getState().agentChats.folders.map((f) => f.id)).toEqual(['f1'])
  })

  it('a non-Error refusal falls back to a generic toast message', async () => {
    createChatFolder.mockRejectedValueOnce({ some: 'object' })
    const id = await createChatFolderRow(store, WS, '')
    expect(id).toBe('')
    expect(toastError).toHaveBeenCalledWith('Failed to create the folder')
  })
})

describe('renameChatFolderRow', () => {
  it('is a no-op for a folder id the store does not hold', async () => {
    await renameChatFolderRow(store, WS, 'ghost', 'New name')
    expect(updateChatFolder).not.toHaveBeenCalled()
  })

  it('is a no-op when the name is unchanged', async () => {
    store.getState().seedAgentChatFolders([folder('f1', '', 0, 'F1')])
    await renameChatFolderRow(store, WS, 'f1', 'F1')
    expect(updateChatFolder).not.toHaveBeenCalled()
  })

  it('renames OPTIMISTICALLY, then reconciles with the daemon answer (and its shifted rows)', async () => {
    store.getState().seedAgentChatFolders([folder('f1', '', 0, 'F1')])
    const call = deferred<{ folder: AgentChatFolder; shifted: AgentChatFolder[] }>()
    updateChatFolder.mockReturnValueOnce(call.promise)

    const p = renameChatFolderRow(store, WS, 'f1', 'F2')

    // Before the daemon has answered, the row already reads "F2".
    expect(store.getState().agentChats.folders.find((f) => f.id === 'f1')?.name).toBe('F2')

    call.resolve({ folder: folder('f1', '', 0, 'F2 (server)'), shifted: [folder('f9', '', 3)] })
    await p

    expect(store.getState().agentChats.folders.find((f) => f.id === 'f1')?.name).toBe('F2 (server)')
    expect(store.getState().agentChats.folders.find((f) => f.id === 'f9')).toMatchObject({
      order: 3,
    })
  })

  it('restores the ORIGINAL folder and toasts when the daemon refuses the rename', async () => {
    store.getState().seedAgentChatFolders([folder('f1', 'p1', 7, 'F1')])
    updateChatFolder.mockRejectedValueOnce(new Error('name taken'))

    await renameChatFolderRow(store, WS, 'f1', 'F2')

    expect(toastError).toHaveBeenCalledWith('name taken')
    expect(store.getState().agentChats.folders.find((f) => f.id === 'f1')).toMatchObject({
      name: 'F1',
      parentId: 'p1',
      order: 7,
    })
  })

  it('a non-Error refusal falls back to a generic toast message', async () => {
    store.getState().seedAgentChatFolders([folder('f1', '', 0, 'F1')])
    updateChatFolder.mockRejectedValueOnce('nope')

    await renameChatFolderRow(store, WS, 'f1', 'F2')

    expect(toastError).toHaveBeenCalledWith('Failed to rename the folder')
  })
})

// ── Grouping: the ONLY way a folder is made here ─────────────────────
describe('groupIntoChatFolder', () => {
  it('makes the folder a SIBLING of the rows it collects, then files them in order', async () => {
    createChatFolder.mockResolvedValue({ folder: folder('f-new', 'p1', 0), shifted: [] })
    setChatPlacement.mockImplementation((_ws: string, id: string) =>
      Promise.resolve({ chat: chat(id, 'f-new', 0), shifted: [] }),
    )
    store.getState().seedAgentChats([chat('t1', 'p1', 0), chat('t2', 'p1', 1)])

    const id = await groupIntoChatFolder(store, WS, [
      { kind: 'chat', id: 't1', parentId: 'p1' },
      { kind: 'chat', id: 't2', parentId: 'p1' },
    ])

    expect(id).toBe('f-new')
    // Where they already live — not the root they would have to be dragged back from.
    expect(createChatFolder).toHaveBeenCalledWith(WS, NEW_FOLDER_NAME, 'p1')
    expect(setChatPlacement.mock.calls.map((c) => [c[1], c[2]])).toEqual([
      ['t1', { parentId: 'f-new', order: 0 }],
      ['t2', { parentId: 'f-new', order: 1 }],
    ])
  })

  it('treats a subject with no container as living at the root', async () => {
    createChatFolder.mockResolvedValue({ folder: folder('f-new', '', 0), shifted: [] })
    setChatPlacement.mockResolvedValue({ chat: chat('c1', 'f-new', 0), shifted: [] })
    store.getState().seedAgentChats([chat('c1', '', 0)])

    await groupIntoChatFolder(store, WS, [{ kind: 'chat', id: 'c1' }])

    expect(createChatFolder).toHaveBeenCalledWith(WS, NEW_FOLDER_NAME, '')
  })

  it('does nothing at all for an empty selection', async () => {
    expect(await groupIntoChatFolder(store, WS, [])).toBe('')
    expect(createChatFolder).not.toHaveBeenCalled()
  })

  it('files nothing when the folder itself was refused', async () => {
    createChatFolder.mockRejectedValue(new Error('nope'))
    store.getState().seedAgentChats([chat('c1', '', 0)])

    expect(await groupIntoChatFolder(store, WS, [{ kind: 'chat', id: 'c1', parentId: '' }])).toBe(
      '',
    )
    // No editor opens on a row that does not exist, and no row is moved into one.
    expect(setChatPlacement).not.toHaveBeenCalled()
  })

  it('puts the rows back but KEEPS the folder when filing is refused', async () => {
    createChatFolder.mockResolvedValue({ folder: folder('f-new', '', 0), shifted: [] })
    setChatPlacement.mockRejectedValue('nope')
    store.getState().seedAgentChats([chat('c1', '', 3)])

    const id = await groupIntoChatFolder(store, WS, [{ kind: 'chat', id: 'c1', parentId: '' }])

    // The folder exists on the server now; deleting it would be a second write
    // racing the one that just failed.
    expect(id).toBe('f-new')
    expect(store.getState().agentChats.folders.map((f) => f.id)).toEqual(['f-new'])
    expect(store.getState().agentChats.chats[0]).toMatchObject({ parentId: '', order: 3 })
    expect(toastError).toHaveBeenCalledWith('Those rows could not be filed')
  })
})
