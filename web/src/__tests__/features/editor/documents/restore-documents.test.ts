import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const { readDocument } = vi.hoisted(() => ({ readDocument: vi.fn() }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument,
}))

import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { ApiError } from '@/lib/api'
import { HOST_DOCUMENT_TRANSPORT_CODE, workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { documentUri } from '@/features/editor/documents/document-ref'
import { documentText, savedDocumentText } from '@/features/editor/documents/document-text'
import { clearDraft, loadDrafts, putDrafts } from '@/features/editor/documents/draft-store'
import { findDocumentBuffer } from '@/features/editor/documents/open-document'
import { restoreDocuments } from '@/features/editor/documents/restore-documents'
import { loadEditorHost } from '@/features/editor/host/editor-host'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'

const A = workspaceRef('ws-restore', 'src/a.ts')
const UNTITLED = workspaceRef('ws-restore', 'untitled:Untitled')
const host = (path: string): DocumentRef => ({ authority: 'host', path })

/** A buffer the saved layout brought back: a tab, and no document yet. */
function restoredTab(doc: DocumentRef, opts: { isVirtual?: boolean } = {}): string {
  return windowPaneStore.getState().bufferActions.openContent({
    type: 'editor',
    doc,
    name: doc.path,
    isVirtual: opts.isVirtual,
  })
}

const draft = (doc: DocumentRef, text: string, savedText: string) => ({
  uri: documentUri(doc),
  text,
  savedText,
  updatedAt: 1,
})

function bufferOf(id: string): EditorContent {
  const found = windowPaneStore.getState().buffers.find((b) => b.id === id)
  if (!found || !isEditorContent(found)) throw new Error(`no editor buffer ${id}`)
  return found
}

let warning: ReturnType<typeof vi.spyOn>
let error: ReturnType<typeof vi.spyOn>

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

beforeEach(() => {
  resetWindowPaneStoreForTests()
  readDocument.mockReset()
  warning = vi.spyOn(toast, 'warning').mockImplementation(() => '')
  error = vi.spyOn(toast, 'error').mockImplementation(() => '')
})

afterEach(async () => {
  for (const uri of (await loadDrafts()).keys()) clearDraft(uri)
  await loadDrafts()
  vi.restoreAllMocks()
})

describe('restoreDocuments', () => {
  it('restores a dirty draft as a dirty document', async () => {
    const id = restoredTab(A)
    await putDrafts([draft(A, 'mine', 'disk')])
    readDocument.mockResolvedValue({ ref: A, text: 'disk' })

    await restoreDocuments()

    expect(documentText(A)).toBe('mine')
    expect(savedDocumentText(A)).toBe('disk')
    expect(bufferOf(id)).toMatchObject({ isDirty: true })
    expect(bufferOf(id).hasExternalChange).toBeFalsy()
    expect(warning).not.toHaveBeenCalled()
  })

  it('flags a dirty draft whose file changed on disk', async () => {
    const id = restoredTab(A)
    await putDrafts([draft(A, 'mine', 'disk')])
    readDocument.mockResolvedValue({ ref: A, text: 'other' })

    await restoreDocuments()

    expect(documentText(A)).toBe('mine')
    expect(bufferOf(id)).toMatchObject({ isDirty: true, hasExternalChange: true })
    expect(warning).toHaveBeenCalledTimes(1)
  })

  it('opens a clean buffer from disk', async () => {
    const id = restoredTab(A)
    readDocument.mockResolvedValue({ ref: A, text: 'disk' })

    await restoreDocuments()

    expect(documentText(A)).toBe('disk')
    expect(bufferOf(id).isDirty).toBe(false)
  })

  it('marks a clean buffer whose file is gone as missing', async () => {
    const id = restoredTab(A)
    readDocument.mockRejectedValue(new ApiError('gone', 404))

    await restoreDocuments()

    expect(bufferOf(id).fileMissing).toBe(true)
    expect(documentText(A)).toBeNull()
  })

  it('restores an untitled buffer from its draft', async () => {
    const id = restoredTab(UNTITLED, { isVirtual: true })
    await putDrafts([draft(UNTITLED, 'scratch', '')])

    await restoreDocuments()

    expect(documentText(UNTITLED)).toBe('scratch')
    expect(bufferOf(id).isDirty).toBe(true)
    expect(readDocument).not.toHaveBeenCalled()
  })

  it('drops a draft whose buffer is gone', async () => {
    await putDrafts([draft(A, 'orphan', 'disk')])

    await restoreDocuments()

    expect((await loadDrafts()).size).toBe(0)
  })

  it('a restored host document the daemon refuses keeps its tab and says why once', async () => {
    const [a, b] = [host('/tmp/a.md'), host('/tmp/b.md')]
    restoredTab(a)
    restoredTab(b)
    await putDrafts([draft(b, 'unsaved', 'saved')])
    readDocument.mockRejectedValue(new ApiError('refused', 403, HOST_DOCUMENT_TRANSPORT_CODE))

    await restoreDocuments()

    expect(windowPaneStore.getState().buffers).toHaveLength(2)
    expect(findDocumentBuffer(a)?.fileMissing).toBeFalsy()
    expect(documentText(a)).toBeNull()
    expect(documentText(b)).toBe('unsaved')
    expect(findDocumentBuffer(b)?.isDirty).toBe(true)
    expect(error).toHaveBeenCalledTimes(1)
    expect(error).toHaveBeenCalledWith(
      "Can't open this file here",
      'Files outside your workspaces open only in the Crowbar desktop app.',
    )
  })
})
