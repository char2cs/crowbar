import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

// Invariant (spec §7-E): the cursor lands on the right line after a
// cross-file reveal. The old jump paths placed it from a 100–150 ms timer by
// a character offset computed from whatever buffer was active then — wrong
// file on a slow switch, wrong line on a large one. reveal() waits for the
// pane's editor to show the target model, however long that takes.

const { readDisk } = vi.hoisted(() => ({ readDisk: vi.fn() }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  // The daemon answers an absolute path outside every workspace as a host document.
  readDocument: async (target: DocumentTarget) => {
    const ref = 'absolutePath' in target ? { authority: 'host', path: target.absolutePath } : target
    return { ref, text: await readDisk(ref) }
  },
}))

import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { activeEditors } from '@/features/editor/lib/active-editor-context'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import { documentUri } from '@/features/editor/documents/document-ref'
import { revealInEditor } from '@/features/editor/lib/reveal'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { workspaceRef, type DocumentRef, type DocumentTarget } from '@/lib/api/documents'
import { toast } from '@/features/window/stores/toast-store'

function fakeEditor() {
  return {
    setSelection: vi.fn(),
    revealPositionInCenterIfOutsideViewport: vi.fn(),
    focus: vi.fn(),
  }
}

function openBuffer(path: string): string {
  return windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    doc: workspaceRef('ws-1', path),
    name: path,
  })
}

/** Publish "the pane now shows `doc`", the way the editor controller does. */
function showInPane(paneId: string, path: string, editor: unknown, workspaceId = 'ws-1') {
  show(paneId, workspaceRef(workspaceId, path), editor)
}
function show(paneId: string, doc: DocumentRef, editor: unknown) {
  activeEditors.set(paneId, { paneId, uri: documentUri(doc), filePath: doc.path, editor })
}

// Opening a closed file opens its Monaco model.
beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

beforeEach(() => {
  vi.clearAllMocks()
  resetWindowPaneStoreForTests()
  activeEditors.clear(windowPaneStore.getState().activePaneId)
})

describe('revealInEditor', () => {
  it('opens a closed file and places the cursor only once the editor shows it', async () => {
    openBuffer('src/app.ts')
    readDisk.mockResolvedValue('target text')
    const paneId = windowPaneStore.getState().activePaneId
    const editor = fakeEditor()

    const pending = revealInEditor({
      doc: workspaceRef('ws-1', 'src/lib/target.ts'),
      position: { line: 1200, character: 4 },
    })
    await vi.waitFor(() => expect(readDisk).toHaveBeenCalled())
    // The pane still shows the previous file: nothing may be placed yet.
    showInPane(paneId, 'src/app.ts', editor)
    expect(editor.setSelection).not.toHaveBeenCalled()

    await vi.waitFor(() => {
      const state = windowPaneStore.getState()
      const active = state.buffers.find((b) => b.id === state.panes[paneId]?.activeEditorTabId)
      expect(active?.path).toBe('src/lib/target.ts')
    })
    showInPane(paneId, 'src/lib/target.ts', editor)

    expect(await pending).toBe(editor)
    expect(readDisk).toHaveBeenCalledWith(workspaceRef('ws-1', 'src/lib/target.ts'))
    expect(editor.setSelection).toHaveBeenCalledWith({
      startLineNumber: 1201,
      startColumn: 5,
      endLineNumber: 1201,
      endColumn: 5,
    })
    expect(editor.revealPositionInCenterIfOutsideViewport).toHaveBeenCalledWith({
      lineNumber: 1201,
      column: 5,
    })
  })

  it('switches to an already-open buffer without re-reading it', async () => {
    openBuffer('src/lib/target.ts')
    openBuffer('src/app.ts')
    const paneId = windowPaneStore.getState().activePaneId
    const editor = fakeEditor()

    const pending = revealInEditor({ doc: workspaceRef('ws-1', 'src/lib/target.ts') })
    showInPane(paneId, 'src/lib/target.ts', editor)

    expect(await pending).toBe(editor)
    expect(readDisk).not.toHaveBeenCalled()
  })

  it('waits while the pane shows another workspace’s file at the same path', async () => {
    openBuffer('src/lib/target.ts')
    const paneId = windowPaneStore.getState().activePaneId
    const sibling = fakeEditor()
    const target = fakeEditor()
    showInPane(paneId, 'src/lib/target.ts', sibling, 'ws-2')

    const pending = revealInEditor({ doc: workspaceRef('ws-1', 'src/lib/target.ts') })
    showInPane(paneId, 'src/lib/target.ts', target)

    expect(await pending).toBe(target)
    expect(sibling.focus).not.toHaveBeenCalled()
  })

  it('gives up (null) when the pane moves to another tab before the target shows', async () => {
    const other = openBuffer('src/other.ts')
    openBuffer('src/lib/target.ts')
    const paneId = windowPaneStore.getState().activePaneId

    const pending = revealInEditor({
      doc: workspaceRef('ws-1', 'src/lib/target.ts'),
      position: { line: 3, character: 0 },
    })
    windowPaneStore.getState().paneActions.activateEditorTabInPane(paneId, other)

    expect(await pending).toBeNull()
  })

  it('a failed read tells the user and reveals nothing', async () => {
    const toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
    readDisk.mockRejectedValue(new Error('path escapes the workspace'))

    expect(await revealInEditor({ doc: workspaceRef('ws-1', 'src/missing.ts') })).toBeNull()
    expect(toastError).toHaveBeenCalledWith('Failed to open file', 'missing.ts')
    toastError.mockRestore()
  })

  it('reveals an absolute path outside every workspace as a host document', async () => {
    readDisk.mockResolvedValue('host text')
    const paneId = windowPaneStore.getState().activePaneId
    const editor = fakeEditor()

    const pending = revealInEditor({
      doc: { absolutePath: '/tmp/s5/probe.ts' },
      position: { line: 2, character: 0 },
    })
    await vi.waitFor(() => expect(readDisk).toHaveBeenCalled())
    show(paneId, { authority: 'host', path: '/tmp/s5/probe.ts' }, editor)

    expect(await pending).toBe(editor)
    expect(editor.setSelection).toHaveBeenCalledWith(
      expect.objectContaining({ startLineNumber: 3 }),
    )
  })
})
