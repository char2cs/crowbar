import { beforeEach, describe, expect, it, vi } from 'vitest'

// Invariant (spec §7-E): the cursor lands on the right line after a
// cross-file reveal. The old jump paths placed it from a 100–150 ms timer by
// a character offset computed from whatever buffer was active then — wrong
// file on a slow switch, wrong line on a large one. reveal() waits for the
// pane's editor to show the target model, however long that takes.

const { readWorkspaceFile } = vi.hoisted(() => ({ readWorkspaceFile: vi.fn() }))
vi.mock('@/features/file-system/controllers/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/file-system/controllers/platform')>()),
  readWorkspaceFile,
}))

import { createActiveEditorRegistry } from '@/features/editor/lib/active-editor-context'
const registry = createActiveEditorRegistry()
vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@/features/workspace/stores/workspace-store-registry')
  >()),
  getWorkspaceStore: () => ({ activeEditorRegistry: registry }),
  getActiveWorkspaceStore: () => null,
}))

import { revealInEditor } from '@/features/editor/lib/reveal'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

function fakeEditor() {
  return {
    setSelection: vi.fn(),
    revealPositionInCenterIfOutsideViewport: vi.fn(),
    setScrollPosition: vi.fn(),
    focus: vi.fn(),
  }
}

function openBuffer(path: string): string {
  return windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    path,
    name: path,
    content: `content of ${path}`,
    workspaceId: 'ws-1',
  })
}

/** Publish "the pane now shows `path`", the way the editor controller does. */
function showInPane(paneId: string, path: string, editor: unknown) {
  registry.set(paneId, { paneId, uri: `crowbar://editor/ws-1/${path}`, filePath: path, editor })
}

beforeEach(() => {
  vi.clearAllMocks()
  resetWindowPaneStoreForTests()
  registry.clear(windowPaneStore.getState().activePaneId)
})

describe('revealInEditor', () => {
  it('opens a closed file and places the cursor only once the editor shows it', async () => {
    openBuffer('src/app.ts')
    readWorkspaceFile.mockResolvedValue('target text')
    const paneId = windowPaneStore.getState().activePaneId
    const editor = fakeEditor()

    const pending = revealInEditor({
      workspaceId: 'ws-1',
      path: 'src/lib/target.ts',
      position: { line: 1200, character: 4 },
    })
    await vi.waitFor(() => expect(readWorkspaceFile).toHaveBeenCalled())
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
    expect(readWorkspaceFile).toHaveBeenCalledWith('ws-1', 'src/lib/target.ts')
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

    const pending = revealInEditor({ workspaceId: 'ws-1', path: 'src/lib/target.ts' })
    showInPane(paneId, 'src/lib/target.ts', editor)

    expect(await pending).toBe(editor)
    expect(readWorkspaceFile).not.toHaveBeenCalled()
  })

  it('gives up (null) when the pane moves to another tab before the target shows', async () => {
    const other = openBuffer('src/other.ts')
    openBuffer('src/lib/target.ts')
    const paneId = windowPaneStore.getState().activePaneId

    const pending = revealInEditor({
      workspaceId: 'ws-1',
      path: 'src/lib/target.ts',
      position: { line: 3, character: 0 },
    })
    windowPaneStore.getState().paneActions.activateEditorTabInPane(paneId, other)

    expect(await pending).toBeNull()
  })

  it('restores an exact scroll offset for jump-list entries', async () => {
    openBuffer('src/lib/target.ts')
    const paneId = windowPaneStore.getState().activePaneId
    const editor = fakeEditor()
    showInPane(paneId, 'src/lib/target.ts', editor)

    await revealInEditor({
      workspaceId: 'ws-1',
      path: 'src/lib/target.ts',
      position: { line: 10, character: 2 },
      scroll: { top: 400, left: 0 },
    })

    expect(editor.setScrollPosition).toHaveBeenCalledWith({ scrollTop: 400, scrollLeft: 0 })
    expect(editor.revealPositionInCenterIfOutsideViewport).not.toHaveBeenCalled()
  })

  it('propagates a failed read so the caller can tell the user', async () => {
    readWorkspaceFile.mockRejectedValue(new Error('path escapes the workspace'))

    await expect(revealInEditor({ workspaceId: 'ws-1', path: 'src/missing.ts' })).rejects.toThrow(
      'path escapes the workspace',
    )
  })
})
