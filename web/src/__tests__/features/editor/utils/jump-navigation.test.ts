import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { useJumpListStore } from '@/features/editor/stores/jump-list-store'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import { documentText } from '@/features/editor/documents/document-text'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'

const readDisk = vi.fn(async (_ref: DocumentRef) => 'reopened content')
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument: async (ref: DocumentRef) => ({ ref, text: await readDisk(ref) }),
}))

/**
 * Stand-in for the editor controller: whenever the root pane activates an
 * editor buffer, publish "the pane's editor shows it" — which is what
 * navigateToJumpEntry (via reveal) waits for before placing the cursor.
 */
const editor = {
  setSelection: vi.fn(),
  revealPositionInCenterIfOutsideViewport: vi.fn(),
  setScrollPosition: vi.fn(),
  focus: vi.fn(),
}
function publishShownBuffer() {
  const state = windowPaneStore.getState()
  const active = state.buffers.find((b) => b.id === state.panes[ROOT_PANE_ID]?.activeEditorTabId)
  if (!active?.path || !isEditorContent(active)) return
  activeEditors.set(ROOT_PANE_ID, {
    paneId: ROOT_PANE_ID,
    uri: documentUri(documentRefOf(active)),
    filePath: active.path,
    editor,
  })
}
windowPaneStore.subscribe(publishShownBuffer)

const { navigateToJumpEntry } = await import('@/features/editor/utils/jump-navigation')

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

const entryFor = (bufferId: string, filePath: string, workspaceId = 'w1') => ({
  bufferId,
  doc: workspaceRef(workspaceId, filePath),
  line: 1,
  column: 1,
  offset: 0,
  scrollTop: 0,
  scrollLeft: 0,
  timestamp: 0,
})

/**
 * The pane renders `pane.editorTabIds`-derived buffers only, so an
 * activeEditorTabId outside that list renders a blank editor. Every
 * navigation path must leave the target buffer BOTH active and a member of
 * the pane. Task 26: panes/buffers are window-level now — read
 * `windowPaneStore`, not the per-workspace `store`.
 */
const isRenderable = (paneId: string): boolean => {
  const pane = windowPaneStore.getState().panes[paneId]
  if (!pane?.activeEditorTabId) return false
  return pane.editorTabIds.includes(pane.activeEditorTabId)
}

function openInWorkspace(path: string, name: string, workspaceId = 'w1'): string {
  return windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    doc: workspaceRef(workspaceId, path),
    name,
  })
}

const activeBuffer = () => {
  const { buffers, panes } = windowPaneStore.getState()
  return buffers.find((b) => b.id === panes[ROOT_PANE_ID]?.activeEditorTabId)
}

describe('navigateToJumpEntry', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    setActiveWorkspaceId('w1')
  })

  it('shows the file when its tab is still open in the active pane', async () => {
    const id = openInWorkspace('/a.ts', 'a.ts')

    await navigateToJumpEntry(entryFor(id, '/a.ts'))

    expect(windowPaneStore.getState().panes[ROOT_PANE_ID]?.activeEditorTabId).toBe(id)
    expect(isRenderable(ROOT_PANE_ID)).toBe(true)
  })

  it('shows the file after its tab was closed but the buffer survives', async () => {
    const id = openInWorkspace('/a.ts', 'a.ts')
    // Tab closed, buffer still in the store — jump navigation finds it by id and
    // must re-attach it to the pane, not just mark it active.
    windowPaneStore.getState().paneActions.removeEditorTabFromPane(ROOT_PANE_ID, id)

    await navigateToJumpEntry(entryFor(id, '/a.ts'))

    expect(isRenderable(ROOT_PANE_ID)).toBe(true)
  })

  it('shows the file after it was fully closed and must be reopened from disk', async () => {
    // Nothing open: the entry points at a buffer that no longer exists.
    await navigateToJumpEntry(entryFor('gone', '/b.ts'))

    const pane = windowPaneStore.getState().panes[ROOT_PANE_ID]
    const active = windowPaneStore.getState().buffers.find((b) => b.id === pane?.activeEditorTabId)
    expect(active?.path).toBe('/b.ts')
    expect(isRenderable(ROOT_PANE_ID)).toBe(true)
  })

  it('reopens with the file’s real content, not an empty buffer', async () => {
    // Regression: this path used the `readFileContent` stub, which unconditionally
    // returned '' — the tab reappeared with the right name and no text in it.
    await navigateToJumpEntry(entryFor('gone', '/b.ts'))

    expect(documentText(workspaceRef('w1', '/b.ts'))).toBe('reopened content')
  })

  it('reads from the entry’s own workspace, not whichever is active later', async () => {
    // Linked worktrees of one repo share relative paths, so a late resolve can
    // load a sibling checkout's content into the buffer.
    readDisk.mockClear()
    await navigateToJumpEntry(entryFor('gone', '/b.ts'))

    expect(readDisk).toHaveBeenCalledWith(workspaceRef('w1', '/b.ts'))
  })

  // The jump list is a process-global singleton that survives a workspace
  // switch, and sibling worktrees hold the same relative paths. An entry names
  // its whole document, so it opens that document from any workspace.
  it('opens the entry’s own document after a switch to a sibling worktree', async () => {
    setActiveWorkspaceId('w2')
    readDisk.mockClear()

    const ok = await navigateToJumpEntry(entryFor('gone', '/src/app.ts', 'w1'))

    expect(ok).toBe(true)
    expect(readDisk).toHaveBeenCalledWith(workspaceRef('w1', '/src/app.ts'))
    expect(activeBuffer()).toMatchObject({ workspaceId: 'w1', path: '/src/app.ts' })
  })

  it('never reveals a same-path buffer of another workspace', async () => {
    const sibling = openInWorkspace('/src/app.ts', 'app.ts', 'w2')

    const ok = await navigateToJumpEntry(entryFor('gone', '/src/app.ts', 'w1'))

    expect(ok).toBe(true)
    expect(activeBuffer()?.id).not.toBe(sibling)
    expect(activeBuffer()).toMatchObject({ workspaceId: 'w1' })
    expect(documentText(workspaceRef('w1', '/src/app.ts'))).toBe('reopened content')
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
    resetWindowPaneStoreForTests()
    useJumpListStore.getState().actions.clear()
  })

  it('retargets the pending marker onto the reopened buffer', async () => {
    const jump = useJumpListStore.getState().actions
    jump.pushEntry(entryFor('buf-a', '/a.ts'))
    // /c.ts's tab was closed, so its recorded bufferId no longer resolves.
    const back = jump.goBack(entryFor('buf-c', '/c.ts'))!

    await navigateToJumpEntry(back)

    const reopenedId = windowPaneStore.getState().panes[ROOT_PANE_ID]?.activeEditorTabId
    expect(reopenedId).toBeTruthy()
    expect(reopenedId).not.toBe(back.bufferId)
    // This is what use-navigation-history asks one render later.
    expect(jump.consumeNavigationTarget(reopenedId!)).toBe(true)
  })

  it('clears the pending marker when the file can no longer be read', async () => {
    const jump = useJumpListStore.getState().actions
    jump.pushEntry(entryFor('buf-a', '/a.ts'))
    const back = jump.goBack(entryFor('buf-b', '/b.ts'))!
    readDisk.mockRejectedValueOnce(new Error('ENOENT'))

    const ok = await navigateToJumpEntry(back)

    expect(ok).toBe(false)
    expect(jump.consumeNavigationTarget(back.bufferId)).toBe(false)
  })
})
