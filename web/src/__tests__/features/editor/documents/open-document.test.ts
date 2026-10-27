import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

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

import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { ApiError } from '@/lib/api'
import {
  HOST_DOCUMENT_TRANSPORT_CODE,
  workspaceRef,
  type DocumentRef,
  type DocumentTarget,
} from '@/lib/api/documents'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import { documentText, replaceDocumentText } from '@/features/editor/documents/document-text'
import {
  documentModelOpener,
  openDocument,
  openDocumentFromDialog,
  openDocumentsAtPaths,
  openUntitledDocument,
  retryLoad,
  setDocumentUnavailable,
} from '@/features/editor/documents/open-document'
import { setDocumentDirty } from '@/features/editor/lib/buffer-save'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { toast } from '@/features/window/stores/toast-store'
import { wsManager } from '@/lib/ws/manager'

const host = (path: string): DocumentRef => ({ authority: 'host', path })
const NOTES = host('/tmp/s5/notes.md')

/** The daemon resolves an absolute path to itself as a host document. */
function readsAsHost(target: DocumentTarget) {
  const path = 'absolutePath' in target ? target.absolutePath : target.path
  return Promise.resolve({ ref: host(path), text: `text of ${path}` })
}

const editors = () => windowPaneStore.getState().buffers.filter(isEditorContent)
const bufferOf = (id: string) => editors().find((b) => b.id === id)!
const A = workspaceRef('ws-a', 'src/a.ts')
/** A tab whose document is not open yet. */
const openContent = (doc: DocumentRef) =>
  windowPaneStore.getState().bufferActions.openContent({ type: 'editor', doc, name: doc.path })
const pane = () => windowPaneStore.getState().panes[windowPaneStore.getState().activePaneId]

let toastError: ReturnType<typeof vi.spyOn>
let toastWarning: ReturnType<typeof vi.spyOn>

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

beforeEach(() => {
  resetWindowPaneStoreForTests()
  readDocument.mockReset()
  openNativeDialog.mockReset()
  desktop.value = true
  toastError = vi.spyOn(toast, 'error').mockImplementation(() => '')
  toastWarning = vi.spyOn(toast, 'warning').mockImplementation(() => '')
})

afterEach(() => {
  toastError.mockRestore()
  toastWarning.mockRestore()
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
      }),
    ])
    expect(documentText(host('/tmp/s5/notes.md'))).toBe('hi')
    expect(pane().activeEditorTabId).toBe(id)
  })

  it('each untitled tab is a document of its own', async () => {
    const paneId = windowPaneStore.getState().activePaneId
    await openUntitledDocument('ws-a', paneId)
    await openUntitledDocument('ws-a', paneId)
    const [first, second] = editors().map(documentRefOf)

    replaceDocumentText(first!, 'scratch')

    expect(documentText(first!)).toBe('scratch')
    expect(documentText(second!)).toBe('')
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

  it('a document held in another project’s view opens in the pane it is aimed at', async () => {
    readDocument.mockImplementation(readsAsHost)
    const { paneActions } = windowPaneStore.getState()
    const paneOf = (chatId: string) => chatPaneIndex(windowPaneStore.getState().panes).get(chatId)!
    paneActions.openChat('c1', { workspaceId: 'ws-a', projectId: 'p1' })
    const held = await openDocument({ absolutePath: '/x/a.md' }, { paneId: paneOf('c1') })
    paneActions.openChat('c2', { workspaceId: 'ws-b', projectId: 'p2' })
    paneActions.setActiveProject('p2')

    expect(await openDocument({ absolutePath: '/x/a.md' }, { paneId: paneOf('c2') })).toBe(held)

    expect(windowPaneStore.getState().panes[paneOf('c2')]?.editorTabIds).toContain(held)
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

  it('a host document holds one watch however many opens race for it, and only while open', async () => {
    let held = 0
    vi.spyOn(wsManager, 'subscribe').mockImplementation(() => (held++, () => void held--))
    vi.spyOn(wsManager, 'subscribed').mockImplementation(() => held > 0)
    let finish: () => void = () => {}
    readDocument
      .mockReturnValueOnce(new Promise((resolve) => (finish = () => resolve(readsAsHost(NOTES)))))
      .mockImplementation(readsAsHost)

    const first = openDocument(NOTES)
    await vi.waitFor(() => expect(readDocument).toHaveBeenCalledTimes(1))
    const second = await openDocument(NOTES)
    finish()

    expect(await first).toBe(second)
    expect([editors().length, held]).toEqual([1, 1])
    readDocument.mockRejectedValueOnce(new ApiError('gone', 404))
    expect(await openDocument(host('/tmp/s5/gone.md'))).toBeNull()
    expect(held).toBe(1)
    resetWindowPaneStoreForTests()
    expect(held).toBe(0)
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

describe('retryLoad', () => {
  it('a later read failure replaces the reason a clean open document shows; a file back unchanged clears it', async () => {
    const id = openContent(A)
    ;(await documentModelOpener())(A, 'disk')
    setDocumentUnavailable(A, { title: 'File not found', message: 'src/a.ts' })
    readDocument.mockRejectedValueOnce(new ApiError('boom', 500))

    expect(await retryLoad(bufferOf(id))).toBe(false)
    expect(bufferOf(id).unavailable).toEqual({ title: 'Failed to open file', message: 'a.ts' })
    expect(toastError).toHaveBeenLastCalledWith('Failed to open file', 'a.ts')

    readDocument.mockResolvedValue({ ref: A, text: 'disk' })
    expect(await retryLoad(bufferOf(id))).toBe(true)
    expect(bufferOf(id).unavailable).toBeUndefined()
  })

  it('a dirty open document meeting changed disk keeps its text, is flagged, and warns once', async () => {
    const id = openContent(A)
    ;(await documentModelOpener())(A, 'mine', 'disk')
    setDocumentDirty(documentUri(A), true)
    readDocument.mockResolvedValue({ ref: A, text: 'theirs' })

    await retryLoad(bufferOf(id))

    expect(documentText(A)).toBe('mine')
    expect(bufferOf(id)).toMatchObject({ isDirty: true, hasExternalChange: true })
    expect(toastWarning).toHaveBeenCalledTimes(1)
  })
})
