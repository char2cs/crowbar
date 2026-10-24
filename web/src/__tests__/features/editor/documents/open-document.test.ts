import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { readDocument, openNativeDialog, desktop } = vi.hoisted(() => ({
  readDocument: vi.fn(),
  openNativeDialog: vi.fn(),
  desktop: { value: true },
}))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument,
}))
vi.mock('@/lib/native-dialog', () => ({ openNativeDialog }))
vi.mock('@/lib/crowbar-bridge', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/crowbar-bridge')>()),
  isTauri: () => desktop.value,
}))

import { ApiError } from '@/lib/api'
import {
  HOST_DOCUMENT_TRANSPORT_CODE,
  workspaceRef,
  type DocumentRef,
  type DocumentTarget,
} from '@/lib/api/documents'
import {
  openDocument,
  openDocumentFromDialog,
  openDocumentsAtPaths,
} from '@/features/editor/documents/open-document'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'

const host = (path: string): DocumentRef => ({ authority: 'host', path })

/** The daemon resolves an absolute path to itself as a host document. */
function readsAsHost(target: DocumentTarget) {
  const path = 'absolutePath' in target ? target.absolutePath : target.path
  return Promise.resolve({ ref: host(path), text: `text of ${path}` })
}

const editors = () => windowPaneStore.getState().buffers.filter(isEditorContent)
const pane = () => windowPaneStore.getState().panes[windowPaneStore.getState().activePaneId]

let toastError: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  resetWindowPaneStoreForTests()
  readDocument.mockReset()
  openNativeDialog.mockReset()
  desktop.value = true
  toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
})

afterEach(() => {
  toastError.mockRestore()
})

describe('openDocument', () => {
  it('opens a file outside every workspace as a host document', async () => {
    readDocument.mockResolvedValue({ ref: host('/tmp/s5/notes.md'), text: 'hi' })

    const id = await openDocument({ absolutePath: '/tmp/s5/notes.md' })

    expect(editors()).toEqual([
      expect.objectContaining({
        id,
        authority: 'host',
        path: '/tmp/s5/notes.md',
        workspaceId: '',
        name: 'notes.md',
        content: 'hi',
      }),
    ])
    expect(pane().activeEditorTabId).toBe(id)
  })

  it('the same file through an absolute path reuses the workspace buffer', async () => {
    const ref = workspaceRef('w1', 'src/a.ts')
    readDocument.mockResolvedValue({ ref, text: 'a' })

    const first = await openDocument(ref)
    const second = await openDocument({ absolutePath: '/wt/src/a.ts' })

    expect(second).toBe(first)
    expect(editors()).toHaveLength(1)
  })

  it('shows an open document through the pane that holds it, without reading it again', async () => {
    const ref = workspaceRef('w1', 'src/a.ts')
    readDocument.mockResolvedValue({ ref, text: 'a' })
    const id = await openDocument(ref)
    readDocument.mockClear()

    expect(await openDocument(ref)).toBe(id)
    expect(readDocument).not.toHaveBeenCalled()
  })

  it('an open aimed at another pane shows the document in the pane holding it', async () => {
    const ref = workspaceRef('w1', 'src/a.ts')
    readDocument.mockResolvedValue({ ref, text: 'a' })
    const holder = windowPaneStore.getState().activePaneId
    const id = await openDocument(ref)
    const other = windowPaneStore.getState().paneActions.splitPane(holder, 'horizontal')!

    await openDocument(ref, { paneId: other })

    const { panes, activePaneId } = windowPaneStore.getState()
    expect(activePaneId).toBe(holder)
    expect(panes[holder]?.activeEditorTabId).toBe(id)
    expect(panes[other]?.editorTabIds).not.toContain(id)
  })

  it('a refused host read says why and opens nothing', async () => {
    readDocument.mockRejectedValue(new ApiError('refused', 403, HOST_DOCUMENT_TRANSPORT_CODE))

    expect(await openDocument({ absolutePath: '/tmp/s5/notes.md' })).toBeNull()

    expect(editors()).toEqual([])
    expect(toastError).toHaveBeenCalledWith(
      "Can't open this file here",
      'Files outside your workspaces open only in the Crowbar desktop app.',
    )
  })

  it.each([
    [new ApiError('gone', 404), 'File not found', '/tmp/s5/notes.md'],
    [new ApiError('big', 413), 'File too large to open', 'notes.md'],
    [new Error('boom'), 'Failed to open file', 'notes.md'],
  ])('reports %s by its own title', async (error, title, message) => {
    readDocument.mockRejectedValue(error)

    expect(await openDocument({ absolutePath: '/tmp/s5/notes.md' })).toBeNull()

    expect(toastError).toHaveBeenCalledWith(title, message)
    expect(editors()).toEqual([])
  })

  it('a preview open lands as the pane’s preview tab', async () => {
    readDocument.mockImplementation(readsAsHost)

    const id = await openDocument({ absolutePath: '/x/a.ts' }, { preview: true })

    expect(editors()).toEqual([expect.objectContaining({ id, isPreview: true })])
  })
})

describe('openDocumentsAtPaths', () => {
  it('opens every dropped path, in drop order', async () => {
    let releaseFirst: () => void = () => {}
    readDocument
      .mockImplementationOnce(
        (target: DocumentTarget) =>
          new Promise((resolve) => {
            releaseFirst = () => resolve(readsAsHost(target))
          }),
      )
      .mockImplementation(readsAsHost)
    const paneId = windowPaneStore.getState().activePaneId

    const done = openDocumentsAtPaths(['/a/1.md', '/a/2.md'], paneId)
    await vi.waitFor(() => expect(readDocument).toHaveBeenCalledTimes(1))
    releaseFirst()
    await done

    const ids = windowPaneStore.getState().panes[paneId].editorTabIds
    expect(ids.map((id) => editors().find((b) => b.id === id)?.path)).toEqual([
      '/a/1.md',
      '/a/2.md',
    ])
  })
})

describe('openDocumentFromDialog', () => {
  it('Open File opens each path the dialog returns', async () => {
    readDocument.mockImplementation(readsAsHost)
    openNativeDialog.mockResolvedValue(['/x/a.ts', '/x/b.ts'])
    const paneId = windowPaneStore.getState().activePaneId

    await openDocumentFromDialog(paneId)

    expect(openNativeDialog).toHaveBeenCalledWith({ multiple: true, directory: false })
    expect(editors().map((b) => b.path)).toEqual(['/x/a.ts', '/x/b.ts'])
  })

  it('a dismissed dialog opens nothing', async () => {
    openNativeDialog.mockResolvedValue(null)

    await openDocumentFromDialog(windowPaneStore.getState().activePaneId)

    expect(readDocument).not.toHaveBeenCalled()
    expect(editors()).toEqual([])
  })

  it('does nothing in the browser, where there is no native dialog', async () => {
    desktop.value = false

    await openDocumentFromDialog(windowPaneStore.getState().activePaneId)

    expect(openNativeDialog).not.toHaveBeenCalled()
  })
})
