import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

let store: ReturnType<typeof createWorkspaceStore>

vi.mock('@/features/workspace/stores/workspace-store-ref', () => ({
  getActiveWorkspaceStoreRef: () => store,
}))
const readWorkspaceFile = vi.fn(async (_wsId: string, _path: string) => 'reopened content')
vi.mock('@/features/file-system/controllers/platform', () => ({
  readWorkspaceFile: (wsId: string, path: string) => readWorkspaceFile(wsId, path),
}))
vi.mock('@/features/editor/extensions/api', () => ({
  editorAPI: { setCursorPosition: vi.fn() },
}))
vi.mock('@/features/editor/stores/ui-store', () => ({
  useEditorUIStore: {
    getState: () => ({
      actions: { setIsLspCompletionVisible: vi.fn(), setLastInputTimestamp: vi.fn() },
    }),
  },
}))
vi.mock('@/features/editor/stores/state-store', () => ({
  useEditorStateStore: { getState: () => ({ actions: { setScroll: vi.fn() } }) },
}))

const { navigateToJumpEntry } = await import('@/features/editor/utils/jump-navigation')

const entryFor = (bufferId: string, filePath: string, workspaceId = 'w1') => ({
  bufferId,
  filePath,
  workspaceId,
  line: 1,
  column: 1,
  offset: 0,
  scrollTop: 0,
  scrollLeft: 0,
  timestamp: 0,
})

/**
 * The pane renders `pane.bufferIds`-derived buffers only, so an activeBufferId
 * outside that list renders a blank editor. Every navigation path must leave the
 * target buffer BOTH active and a member of the pane.
 */
const isRenderable = (paneId: string): boolean => {
  const pane = store.getState().panes[paneId]
  if (!pane?.activeBufferId) return false
  return pane.bufferIds.includes(pane.activeBufferId)
}

describe('navigateToJumpEntry', () => {
  beforeEach(() => {
    store = createWorkspaceStore('w1')
  })

  it('shows the file when its tab is still open in the active pane', async () => {
    const id = store
      .getState()
      .bufferActions.openContent({ type: 'editor', path: '/a.ts', name: 'a.ts', content: 'a' })

    await navigateToJumpEntry(entryFor(id, '/a.ts'))

    expect(store.getState().panes[ROOT_PANE_ID]?.activeBufferId).toBe(id)
    expect(isRenderable(ROOT_PANE_ID)).toBe(true)
  })

  it('shows the file after its tab was closed but the buffer survives', async () => {
    const id = store
      .getState()
      .bufferActions.openContent({ type: 'editor', path: '/a.ts', name: 'a.ts', content: 'a' })
    // Tab closed, buffer still in the store — jump navigation finds it by id and
    // must re-attach it to the pane, not just mark it active.
    store.getState().paneActions.removeBufferFromPane(ROOT_PANE_ID, id, true)

    await navigateToJumpEntry(entryFor(id, '/a.ts'))

    expect(isRenderable(ROOT_PANE_ID)).toBe(true)
  })

  it('shows the file after it was fully closed and must be reopened from disk', async () => {
    // Nothing open: the entry points at a buffer that no longer exists.
    await navigateToJumpEntry(entryFor('gone', '/b.ts'))

    const pane = store.getState().panes[ROOT_PANE_ID]
    const active = store.getState().buffers.find((b) => b.id === pane?.activeBufferId)
    expect(active?.path).toBe('/b.ts')
    expect(isRenderable(ROOT_PANE_ID)).toBe(true)
  })

  it('reopens with the file’s real content, not an empty buffer', async () => {
    // Regression: this path used the `readFileContent` stub, which unconditionally
    // returned '' — the tab reappeared with the right name and no text in it.
    await navigateToJumpEntry(entryFor('gone', '/b.ts'))

    const pane = store.getState().panes[ROOT_PANE_ID]
    const active = store.getState().buffers.find((b) => b.id === pane?.activeBufferId)
    expect((active as { content?: string })?.content).toBe('reopened content')
  })

  it('reads from the entry’s own workspace, not whichever is active later', async () => {
    // Linked worktrees of one repo share relative paths, so a late resolve can
    // load a sibling checkout's content into the buffer.
    readWorkspaceFile.mockClear()
    await navigateToJumpEntry(entryFor('gone', '/b.ts'))

    expect(readWorkspaceFile).toHaveBeenCalledWith('w1', '/b.ts')
  })

  // The jump list is a process-GLOBAL singleton that survives a workspace
  // switch, and its file paths are workspace-RELATIVE. Sibling worktrees of one
  // repo hold the same relative paths with different content, so resolving an
  // entry against whatever workspace is active NOW silently opens the wrong
  // file under the right tab title. An entry names the workspace it was
  // recorded in; anything else must be refused, not guessed at.
  it('refuses an entry recorded in a DIFFERENT workspace than the active one', async () => {
    // Recorded while workspace w1 (branch feature/x) was active...
    const fromW1 = entryFor('gone', '/src/app.ts', 'w1')
    // ...but the user has since switched to the sibling worktree w2.
    store = createWorkspaceStore('w2')
    readWorkspaceFile.mockClear()

    const ok = await navigateToJumpEntry(fromW1)

    expect(ok).toBe(false)
    expect(readWorkspaceFile).not.toHaveBeenCalled()
    expect(store.getState().buffers).toHaveLength(0)
  })

  it('does not reveal a same-path buffer that belongs to the wrong workspace', async () => {
    store = createWorkspaceStore('w2')
    // w2 has its OWN /src/app.ts open — the tab title would look identical.
    store.getState().bufferActions.openContent({
      type: 'editor',
      path: '/src/app.ts',
      name: 'app.ts',
      content: 'b',
    })
    const paneBefore = store.getState().panes[ROOT_PANE_ID]?.activeBufferId

    const ok = await navigateToJumpEntry(entryFor('gone', '/src/app.ts', 'w1'))

    expect(ok).toBe(false)
    expect(store.getState().panes[ROOT_PANE_ID]?.activeBufferId).toBe(paneBefore)
  })

  it('still navigates when the entry names the workspace that is active', async () => {
    readWorkspaceFile.mockClear()
    const ok = await navigateToJumpEntry(entryFor('gone', '/b.ts', 'w1'))

    expect(ok).toBe(true)
    expect(readWorkspaceFile).toHaveBeenCalledWith('w1', '/b.ts')
  })

  // Pre-production graceful fallback: entries persisted (or recorded by a code
  // path not yet updated) without a workspaceId must still work rather than
  // being dropped — an unstamped entry names no workspace to disagree with.
  it('accepts a legacy entry that carries no workspaceId', async () => {
    readWorkspaceFile.mockClear()
    const legacy = entryFor('gone', '/b.ts') as Partial<ReturnType<typeof entryFor>>
    delete legacy.workspaceId

    const ok = await navigateToJumpEntry(legacy as ReturnType<typeof entryFor>)

    expect(ok).toBe(true)
    expect(readWorkspaceFile).toHaveBeenCalledWith('w1', '/b.ts')
  })
})

/**
 * The "this activation came from Back/Forward" handshake names a BUFFER id, but
 * reopening a closed file mints a NEW one. Left unretargeted the recorder does
 * not recognise the activation it sees, records it as a fresh navigation, and
 * `pushEntry` truncates the forward branch — so one Back into a closed file
 * permanently breaks Forward and sends the next Back to the wrong file.
 */
describe('navigateToJumpEntry — the Back/Forward handshake survives a reopen', () => {
  beforeEach(() => {
    store = createWorkspaceStore('w1')
    useJumpListStore.getState().actions.clear()
  })

  it('retargets the pending marker onto the reopened buffer', async () => {
    const jump = useJumpListStore.getState().actions
    jump.pushEntry(entryFor('buf-a', '/a.ts'))
    // /c.ts's tab was closed, so its recorded bufferId no longer resolves.
    const back = jump.goBack(entryFor('buf-c', '/c.ts'))!

    await navigateToJumpEntry(back)

    const reopenedId = store.getState().panes[ROOT_PANE_ID]?.activeBufferId
    expect(reopenedId).toBeTruthy()
    expect(reopenedId).not.toBe(back.bufferId)
    // This is what use-navigation-history asks one render later.
    expect(jump.consumeNavigationTarget(reopenedId!)).toBe(true)
  })

  it('clears the pending marker when the entry is refused, so it cannot swallow a later navigation', async () => {
    const jump = useJumpListStore.getState().actions
    jump.pushEntry(entryFor('buf-a', '/a.ts'))
    const back = jump.goBack(entryFor('buf-b', '/b.ts'))!
    // The user switched workspaces between pressing Back and this resolving.
    store = createWorkspaceStore('w2')

    const ok = await navigateToJumpEntry(back)

    expect(ok).toBe(false)
    // A marker left pending here silently suppresses the next genuine visit to
    // that buffer id.
    expect(jump.consumeNavigationTarget(back.bufferId)).toBe(false)
  })

  it('clears the pending marker when the file can no longer be read', async () => {
    const jump = useJumpListStore.getState().actions
    jump.pushEntry(entryFor('buf-a', '/a.ts'))
    const back = jump.goBack(entryFor('buf-b', '/b.ts'))!
    readWorkspaceFile.mockRejectedValueOnce(new Error('ENOENT'))

    const ok = await navigateToJumpEntry(back)

    expect(ok).toBe(false)
    expect(jump.consumeNavigationTarget(back.bufferId)).toBe(false)
  })
})
