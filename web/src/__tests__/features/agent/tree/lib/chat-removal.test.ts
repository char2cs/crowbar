/**
 * Removing rows from the Chats tree, from plan to send.
 *
 * The rule this file exists to hold up is that the two kinds differ, and that
 * the difference survives into every surface:
 *
 *   - a CHAT cascades to the threads under it, because a thread reads its
 *     parent's turns and one left behind is a conversation reading a context
 *     that no longer exists;
 *   - a FOLDER promotes its contents, because it holds no turns and the chats
 *     outlive it.
 *
 * So the plan hides different rows for each, the veil says different things,
 * the preview draws the survivors in different places, and the send makes
 * different calls. All four are pinned below.
 *
 * The workspace store is REAL, through the same registry the panel uses — this
 * module is written against a live store, not a mock of one — and only the
 * daemon calls are stubbed.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { deleteChat, deleteChatFolder } = vi.hoisted(() => ({
  deleteChat: vi.fn(),
  deleteChatFolder: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({ deleteChat, deleteChatFolder }))

// The store's own persistence subscriptions, stubbed: a removal must not try to
// write a layout or an IndexedDB session in a jsdom test.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

import {
  applyPendingChatRemovals,
  describeChatRemoval,
  isChatRemoval,
  planChatRemoval,
  sendChatRemoval,
} from '@/features/agent/tree/lib/chat-removal'
import type { ChatDragSubject } from '@/features/agent/tree/lib/chat-drop'
import type { ChatLike, FolderLike } from '@/features/agent/tree/lib/chat-rows'
import type { RemovalDraft, RemovalEntry } from '@/lib/store/sidebar-removal'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'
import type { AgentChat, AgentChatFolder } from '@/features/agent/api/agent-api'

const WS = 'w-chat-removal'
/** Stands in for the artwork the daemon serves per provider. */
const CLAUDE_SVG = '<svg data-p="claude"></svg>'

// ── Fixtures ────────────────────────────────────────────────────────

const subject = (kind: 'chat' | 'chatFolder', id: string, parentId = ''): ChatDragSubject => ({
  kind,
  id,
  parentId,
})

const siblingsOf = (entries: Record<string, readonly string[]>) => new Map(Object.entries(entries))

function chat(id: string, parentId = '', order = 0, title = id): AgentChat {
  return {
    id,
    workspaceId: WS,
    title,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: 'claude',
    createdAt: '2026-01-01T00:00:00Z',
    parentId,
    order,
  }
}

function folder(id: string, parentId = '', order = 0, name = id): AgentChatFolder {
  return { id, workspaceId: WS, parentId, name, order }
}

const draft = (over: Partial<RemovalDraft> = {}): RemovalDraft => ({
  kind: 'chat',
  id: 'c1',
  label: 'First',
  wsId: WS,
  providerIcon: '',
  projectId: '',
  repoId: '',
  hiddenIds: ['c1'],
  extra: 0,
  fallbackWsId: null,
  ...over,
})

const entry = (over: Partial<RemovalDraft> = {}): RemovalEntry => ({
  ...draft(over),
  entryId: 'e1',
  deadlineAt: 0,
})

let store: WorkspaceStore

beforeEach(() => {
  store = getOrCreateWorkspaceStore(WS)
  deleteChat.mockReset().mockResolvedValue(undefined)
  deleteChatFolder.mockReset().mockResolvedValue([])
})

afterEach(() => {
  destroyWorkspaceStore(WS)
  vi.clearAllMocks()
})

// ── isChatRemoval ───────────────────────────────────────────────────

describe('isChatRemoval', () => {
  it('claims the two kinds this tree produces and nothing else', () => {
    // The tray is shared with the sidebar; this is what routes an entry to the
    // right commit path and to the right panel's footer.
    expect(isChatRemoval({ kind: 'chat' })).toBe(true)
    expect(isChatRemoval({ kind: 'chatFolder' })).toBe(true)
    expect(isChatRemoval({ kind: 'workspace' })).toBe(false)
    expect(isChatRemoval({ kind: 'folder' })).toBe(false)
    expect(isChatRemoval({ kind: 'repo' })).toBe(false)
    expect(isChatRemoval({ kind: 'project' })).toBe(false)
  })
})

// ── planChatRemoval ─────────────────────────────────────────────────

describe('planChatRemoval', () => {
  const chats: ChatLike[] = [
    { id: 'p1', title: 'Parent', activeProviderId: 'claude', createdAt: '' },
    { id: 't1', title: 'Thread', activeProviderId: 'claude', createdAt: '' },
    { id: 'c9', title: 'Loner', activeProviderId: 'claude', createdAt: '' },
  ]
  const folders: FolderLike[] = [{ id: 'f1', name: 'Spikes' }]
  const providerIcons = new Map([['claude', CLAUDE_SVG]])
  // p1 holds a folder, and the thread is filed INSIDE that folder: the subtree
  // is two levels deep, which is the case an order assertion can actually fail.
  const siblings = siblingsOf({ '': ['p1', 'c9', 'f1'], p1: ['fin'], fin: ['t1'] })

  const plan = (subjects: ChatDragSubject[]) =>
    planChatRemoval(subjects, { wsId: WS, chats, folders, providerIcons, siblings })

  it('plans nothing for nothing', () => {
    expect(plan([])).toEqual([])
  })

  it('takes a chat’s whole subtree, deepest first with the chat last', () => {
    const [only] = plan([subject('chat', 'p1')])
    // The ORDER is the delete order: a parent deleted before its children leaves
    // them pointing at nothing while the requests are in flight.
    expect(only.hiddenIds).toEqual(['t1', 'fin', 'p1'])
    expect(only).toMatchObject({ kind: 'chat', id: 'p1', label: 'Parent', wsId: WS, extra: 2 })
  })

  it('takes a folder alone — its contents are promoted, not deleted', () => {
    const [only] = plan([subject('chatFolder', 'f1')])
    expect(only).toMatchObject({ kind: 'chatFolder', id: 'f1', label: 'Spikes', extra: 0 })
    expect(only.hiddenIds).toEqual(['f1'])
  })

  it('names a chat with no threads as taking nothing with it', () => {
    expect(plan([subject('chat', 'c9')])[0]).toMatchObject({ extra: 0, hiddenIds: ['c9'] })
  })

  it('carries the chat’s PROVIDER artwork, so the tray can draw the row’s own mark', () => {
    // The tray lives in components/layout and cannot reach into this feature for
    // the provider list, so the mark travels on the draft. Taken from the SAME
    // map the rows draw from — a second lookup is a second chance to disagree.
    expect(plan([subject('chat', 'c9')])[0].providerIcon).toBe(CLAUDE_SVG)
  })

  it('carries nothing when the chat’s provider has gone', () => {
    // The tray falls back to the chat glyph there, exactly as the row does.
    const gone = planChatRemoval([subject('chat', 'c9')], {
      wsId: WS,
      chats,
      folders,
      providerIcons: new Map(),
      siblings,
    })
    expect(gone[0].providerIcon).toBe('')
  })

  it('carries no artwork for a FOLDER — it is not a conversation', () => {
    expect(plan([subject('chatFolder', 'f1')])[0].providerIcon).toBe('')
  })

  it('is addressed to the workspace, never to a project or a repo', () => {
    // The two fields the sidebar's rows are addressed by mean nothing here, and
    // a chat's delete route is workspace-nested.
    expect(plan([subject('chat', 'c9')])[0]).toMatchObject({
      wsId: WS,
      projectId: '',
      repoId: '',
      fallbackWsId: null,
    })
  })

  it('does not hold a row that is already inside another subject’s subtree', () => {
    // Two tray rows for one disappearance is the failure this prevents.
    const drafts = plan([subject('chat', 'p1'), subject('chat', 't1', 'fin')])
    expect(drafts.map((d) => d.id)).toEqual(['p1'])
  })

  it('holds the outer row even when the inner one was collected first', () => {
    const drafts = plan([subject('chat', 't1', 'fin'), subject('chat', 'p1')])
    // The inner row claims only itself, so the outer one is still a removal —
    // and it re-hides the inner row, which is what the commit will do anyway.
    expect(drafts.map((d) => d.id)).toEqual(['t1', 'p1'])
  })

  it('skips a row the store no longer holds', () => {
    // A `deleted` frame landing between the gesture and the release. A tray row
    // naming nothing is worse than no tray row.
    expect(plan([subject('chat', 'ghost')])).toEqual([])
    expect(plan([subject('chatFolder', 'ghost')])).toEqual([])
  })
})

// ── describeChatRemoval ─────────────────────────────────────────────

describe('describeChatRemoval', () => {
  it('says "drop here" while the zone is merely available', () => {
    // The pane is up for the whole drag, and for most of it the pointer is
    // somewhere else — telling the user to release then is an instruction to do
    // the one thing that reorders.
    expect(describeChatRemoval([draft()], false)).toEqual({
      title: 'Drop here to remove First',
      detail: 'You will have 8 seconds to undo',
      armed: false,
    })
  })

  it('says "release" once a release really would remove', () => {
    expect(describeChatRemoval([draft()])).toEqual({
      title: 'Release to remove First',
      detail: 'You will have 8 seconds to undo',
      armed: true,
    })
  })

  it('counts what a chat takes with it', () => {
    expect(describeChatRemoval([draft({ extra: 3 })]).detail).toBe(
      '3 nested rows go with it · You will have 8 seconds to undo',
    )
  })

  it('counts a single nested row in the singular', () => {
    expect(describeChatRemoval([draft({ extra: 1 })]).detail).toBe(
      '1 nested row goes with it · You will have 8 seconds to undo',
    )
  })

  it('says the OTHER rule for a folder — its contents survive it', () => {
    expect(describeChatRemoval([draft({ kind: 'chatFolder', label: 'Spikes', extra: 0 })])).toEqual(
      {
        title: 'Release to remove Spikes',
        detail: 'Its contents move up one level · You will have 8 seconds to undo',
        armed: true,
      },
    )
  })

  it('counts a multiselection rather than naming one row of it', () => {
    expect(describeChatRemoval([draft(), draft({ id: 'c2', label: 'Second' })])).toEqual({
      title: 'Release to remove 2 rows',
      detail: 'You will have 8 seconds to undo',
      armed: true,
    })
  })
})

// ── applyPendingChatRemovals ────────────────────────────────────────

describe('applyPendingChatRemovals', () => {
  const chats: ChatLike[] = [
    { id: 'c1', title: 'c1', activeProviderId: 'claude', createdAt: '', parentId: 'f1' },
    { id: 'c2', title: 'c2', activeProviderId: 'claude', createdAt: '', parentId: '' },
  ]
  const folders: FolderLike[] = [
    { id: 'f1', name: 'f1', parentId: 'outer' },
    { id: 'outer', name: 'outer', parentId: '' },
  ]

  it('hands back the very arrays it was given when nothing is held', () => {
    // Identity, not equality: the panel memoises its tree build on these, and a
    // fresh array per render would rebuild the tree on every frame of the feed.
    const out = applyPendingChatRemovals(chats, folders, new Set())
    expect(out.chats).toBe(chats)
    expect(out.folders).toBe(folders)
  })

  it('drops a held chat and leaves everything else alone', () => {
    const out = applyPendingChatRemovals(chats, folders, new Set(['c1']))
    expect(out.chats.map((c) => c.id)).toEqual(['c2'])
    expect(out.folders.map((f) => f.id)).toEqual(['f1', 'outer'])
  })

  it('PROMOTES a held folder’s contents to where the folder sat', () => {
    const out = applyPendingChatRemovals(chats, folders, new Set(['f1']))
    // Not re-rooted: the tree grounds a row whose parent names nothing at the
    // root, which is a different place from the folder's own parent — and a
    // promise the commit would not keep.
    expect(out.chats.find((c) => c.id === 'c1')?.parentId).toBe('outer')
    expect(out.folders.map((f) => f.id)).toEqual(['outer'])
  })

  it('walks past a held ANCESTOR to the outermost row still on screen', () => {
    const out = applyPendingChatRemovals(chats, folders, new Set(['f1', 'outer']))
    expect(out.chats.find((c) => c.id === 'c1')?.parentId).toBe('')
    expect(out.folders).toEqual([])
  })

  it('re-homes a held folder’s child FOLDER as well as its chats', () => {
    const nested: FolderLike[] = [...folders, { id: 'inner', name: 'inner', parentId: 'f1' }]
    const out = applyPendingChatRemovals(chats, nested, new Set(['f1']))
    expect(out.folders.find((f) => f.id === 'inner')?.parentId).toBe('outer')
  })

  it('terminates on a parent cycle among held folders', () => {
    // The store is fed by a wire we do not control, and a projection that hangs
    // on bad data is a projection that hangs.
    const cyclic: FolderLike[] = [
      { id: 'a', name: 'a', parentId: 'b' },
      { id: 'b', name: 'b', parentId: 'a' },
    ]
    const out = applyPendingChatRemovals([], cyclic, new Set(['a', 'b']))
    expect(out.folders).toEqual([])
  })

  it('grounds a held folder that never had a parentId at all', () => {
    const rootless: FolderLike[] = [{ id: 'f1', name: 'f1' }]
    const kids: ChatLike[] = [
      { id: 'c1', title: 'c1', activeProviderId: 'claude', createdAt: '', parentId: 'f1' },
    ]
    const out = applyPendingChatRemovals(kids, rootless, new Set(['f1']))
    expect(out.chats[0].parentId).toBe('')
  })

  it('leaves a SURVIVOR that never had a parentId where it is', () => {
    // An absent parentId and a root parentId are the same thing, and neither is
    // a folder id — so a row that never had one must not be re-homed by a
    // lookup that grounds it to ''.
    const rootless: ChatLike[] = [
      { id: 'c1', title: 'c1', activeProviderId: 'claude', createdAt: '' },
    ]
    const out = applyPendingChatRemovals(rootless, [{ id: 'f1', name: 'f1' }], new Set(['f1']))
    expect(out.chats[0]).toBe(rootless[0])
    expect(out.chats[0].parentId).toBeUndefined()
  })
})

// ── sendChatRemoval ─────────────────────────────────────────────────

describe('sendChatRemoval — a chat and its subtree', () => {
  function seedThread() {
    store.getState().seedAgentChats([chat('p1'), chat('t1', 'fin'), chat('c9', '', 1)])
    store.getState().seedAgentChatFolders([folder('fin', 'p1')])
  }

  const subtreeEntry = entry({ id: 'p1', label: 'Parent', hiddenIds: ['t1', 'fin', 'p1'] })

  it('deletes every CHAT in the subtree, deepest first, and the folders with them', async () => {
    seedThread()

    await sendChatRemoval(subtreeEntry)

    expect(deleteChat.mock.calls.map((c) => c[1])).toEqual(['t1', 'p1'])
    // The nested folder is dropped from the store but never DELETEd: the chat's
    // own delete cascades to it server-side.
    expect(store.getState().agentChats.folders).toEqual([])
    expect(store.getState().agentChats.chats.map((c) => c.id)).toEqual(['c9'])
  })

  it('empties the store BEFORE the requests go out', async () => {
    seedThread()
    let seenDuringFlight: string[] = []
    deleteChat.mockImplementation(async () => {
      seenDuringFlight = store.getState().agentChats.chats.map((c) => c.id)
    })

    await sendChatRemoval(subtreeEntry)

    // This is what lets the tray stop hiding the ids the moment the send
    // resolves: there is nothing left to flash back while a tombstone finds its
    // way over the wire.
    expect(seenDuringFlight).toEqual(['c9'])
  })

  it('passes the caller’s RequestInit through — the unload flush needs keepalive', async () => {
    seedThread()
    await sendChatRemoval(subtreeEntry, { keepalive: true })
    expect(deleteChat).toHaveBeenCalledWith(WS, 't1', { keepalive: true })
  })

  it('clears the doomed chat off its pane without touching an unrelated pane', async () => {
    seedThread()
    const st = store.getState()
    const paneP = st.activePaneId
    st.paneActions.setPaneChat(paneP, 'p1', 'runner-p1')
    const paneQ = st.paneActions.splitPane(paneP, 'vertical')!
    st.paneActions.setPaneChat(paneQ, 'c9', 'runner-c9')

    await sendChatRemoval(subtreeEntry)

    // p1 is doomed — its pane's chat slot falls back to the empty stage.
    expect(store.getState().panes[paneP]?.chatId).toBeNull()
    // c9 survives — its pane is untouched.
    expect(store.getState().panes[paneQ]?.chatId).toBe('c9')
    expect(store.getState().panes[paneQ]?.runnerId).toBe('runner-c9')
  })

  it('restores the chats, the folders and the panes holding them when the daemon refuses', async () => {
    seedThread()
    store.getState().setActiveAgentChatId('p1')
    const pane = store.getState().activePaneId
    store.getState().paneActions.setPaneChat(pane, 'p1', 'runner-p1')
    deleteChat.mockRejectedValue(new Error('nope'))

    await expect(sendChatRemoval(subtreeEntry)).rejects.toThrow('nope')

    expect(
      store
        .getState()
        .agentChats.chats.map((c) => c.id)
        .sort(),
    ).toEqual(['c9', 'p1', 't1'])
    expect(store.getState().agentChats.folders.map((f) => f.id)).toEqual(['fin'])
    // The optimistic clear took the pane's chat with it; a chat snapping back
    // into the list with its pane silently emptied is not a restore.
    expect(store.getState().panes[pane]?.chatId).toBe('p1')
    expect(store.getState().panes[pane]?.runnerId).toBe('runner-p1')
    expect(store.getState().agentChats.activeChatId).toBe('p1')
  })

  it('does not give a pane a chat it never had, nor re-activate a chat that was not', async () => {
    seedThread()
    store.getState().setActiveAgentChatId('c9')
    const pane = store.getState().activePaneId
    deleteChat.mockRejectedValue(new Error('nope'))

    await expect(sendChatRemoval(subtreeEntry)).rejects.toThrow('nope')

    expect(store.getState().panes[pane]?.chatId).toBeNull()
    expect(store.getState().agentChats.activeChatId).toBe('c9')
  })

  it('restores a subtree that held no folders', async () => {
    store.getState().seedAgentChats([chat('p1'), chat('t1', 'p1')])
    deleteChat.mockRejectedValue(new Error('nope'))

    await expect(sendChatRemoval(entry({ id: 'p1', hiddenIds: ['t1', 'p1'] }))).rejects.toThrow(
      'nope',
    )

    expect(
      store
        .getState()
        .agentChats.chats.map((c) => c.id)
        .sort(),
    ).toEqual(['p1', 't1'])
  })
})

describe('sendChatRemoval — a folder', () => {
  const folderEntry = entry({ kind: 'chatFolder', id: 'f1', label: 'Spikes', hiddenIds: ['f1'] })

  function seedFolder() {
    store.getState().seedAgentChatFolders([folder('f1', '', 0), folder('f2', 'f1', 1)])
    store.getState().seedAgentChats([chat('c1', 'f1', 0), chat('c9', '', 1)])
  }

  it('is a no-op for a folder the store does not hold', async () => {
    await sendChatRemoval(folderEntry)
    expect(deleteChatFolder).not.toHaveBeenCalled()
  })

  it('deletes it, PROMOTES its chat and folder children, and applies the further shift', async () => {
    seedFolder()
    deleteChatFolder.mockResolvedValueOnce([folder('f9', '', 5)])

    await sendChatRemoval(folderEntry)

    expect(deleteChatFolder).toHaveBeenCalledWith(WS, 'f1', undefined)
    expect(store.getState().agentChats.folders.map((f) => f.id)).not.toContain('f1')
    expect(store.getState().agentChats.folders.find((f) => f.id === 'f2')?.parentId).toBe('')
    expect(store.getState().agentChats.chats.find((c) => c.id === 'c1')?.parentId).toBe('')
    // The unrelated chat never touched the folder — untouched.
    expect(store.getState().agentChats.chats.find((c) => c.id === 'c9')?.parentId).toBe('')
    expect(store.getState().agentChats.folders.find((f) => f.id === 'f9')).toMatchObject({
      order: 5,
    })
  })

  it('deletes cleanly when the daemon reports no further shift', async () => {
    seedFolder()
    deleteChatFolder.mockResolvedValueOnce([])

    await sendChatRemoval(folderEntry)

    expect(store.getState().agentChats.folders.map((f) => f.id)).toEqual(['f2'])
  })

  it('restores BOTH the folder and every child’s placement when the daemon refuses', async () => {
    seedFolder()
    deleteChatFolder.mockRejectedValueOnce(new Error('folder not empty'))

    await expect(sendChatRemoval(folderEntry)).rejects.toThrow('folder not empty')

    expect(store.getState().agentChats.folders.find((f) => f.id === 'f1')).toMatchObject({
      parentId: '',
      order: 0,
    })
    // Both children are back UNDER f1, not left promoted at the root with the
    // folder back around nothing.
    expect(store.getState().agentChats.folders.find((f) => f.id === 'f2')).toMatchObject({
      parentId: 'f1',
      order: 1,
    })
    expect(store.getState().agentChats.chats.find((c) => c.id === 'c1')).toMatchObject({
      parentId: 'f1',
      order: 0,
    })
  })

  it('does not mistake a row with an absent parentId for one of the folder’s children', async () => {
    // The orphan scan tests (row.parentId ?? '') === folderId. A row that never
    // had parentId set grounds to '' (root) just as an explicit '' would — it
    // must read as unrelated to f1, not as a stray match on the empty string.
    store
      .getState()
      .seedAgentChatFolders([
        folder('f1', '', 0),
        { ...folder('f-stray', '', 1), parentId: undefined },
      ])
    store
      .getState()
      .seedAgentChats([chat('c1', 'f1', 0), { ...chat('c-stray', '', 1), parentId: undefined }])

    await sendChatRemoval(folderEntry)

    expect(store.getState().agentChats.chats.find((c) => c.id === 'c1')?.parentId).toBe('')
    expect(
      store.getState().agentChats.folders.find((f) => f.id === 'f-stray')?.parentId,
    ).toBeUndefined()
    expect(
      store.getState().agentChats.chats.find((c) => c.id === 'c-stray')?.parentId,
    ).toBeUndefined()
  })

  it('passes the caller’s RequestInit through', async () => {
    seedFolder()
    await sendChatRemoval(folderEntry, { keepalive: true })
    expect(deleteChatFolder).toHaveBeenCalledWith(WS, 'f1', { keepalive: true })
  })
})
