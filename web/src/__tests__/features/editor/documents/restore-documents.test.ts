import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

const { readDocument } = vi.hoisted(() => ({ readDocument: vi.fn() }))
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument,
  readDocumentFromDisk: async (ref: unknown) => (await readDocument(ref).catch(() => null))?.text,
}))
// Counts Monaco loads; the host itself is the real one.
const { loadEditorHost } = vi.hoisted(() => ({ loadEditorHost: vi.fn() }))
vi.mock('@/features/editor/host/editor-host', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/editor/host/editor-host')>()
  loadEditorHost.mockImplementation(actual.loadEditorHost)
  return { ...actual, loadEditorHost }
})

import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { ApiError } from '@/lib/api'
import { HOST_DOCUMENT_TRANSPORT_CODE, workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { documentUri } from '@/features/editor/documents/document-ref'
import { documentText, savedDocumentText } from '@/features/editor/documents/document-text'
import { clearDraft, loadDrafts, putDrafts } from '@/features/editor/documents/draft-store'
import {
  documentModelOpener,
  findDocumentBuffer,
  openDocument,
  retryLoad,
} from '@/features/editor/documents/open-document'
import { getEditorHost, releaseWithDocument } from '@/features/editor/host/editor-host'
import { restoreDocuments } from '@/features/editor/documents/restore-documents'
import { saveActiveBuffer } from '@/features/editor/lib/buffer-save'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import { toast } from '@/features/window/stores/toast-store'
import {
  followHostDocument,
  reconcileWorkspaceDocuments,
} from '@/features/workspace/lib/external-buffer-sync'
import { useSidebarStore } from '@/lib/store/sidebar'
import { wsManager } from '@/lib/ws/manager'

const A = workspaceRef('ws-restore', 'src/a.ts')
const B = workspaceRef('ws-restore', 'src/b.ts')
const UNTITLED = workspaceRef('ws-restore', 'untitled:Untitled')
const host = (path: string): DocumentRef => ({ authority: 'host', path })
const NOTES = { authority: 'host', path: '/tmp/s5/notes.md' } as const

/** Host watch sockets: the calls in order, how many are held, and what an open answers. */
function watches(opened: () => Promise<void> = () => Promise.resolve()) {
  const w = { calls: [] as string[], held: 0, opened }
  vi.spyOn(wsManager, 'subscribe').mockImplementation(() => {
    w.calls.push('subscribe')
    w.held++
    return () => void w.held--
  })
  vi.spyOn(wsManager, 'opened').mockImplementation(() => (w.calls.push('opened'), w.opened()))
  vi.spyOn(wsManager, 'subscribed').mockImplementation(() => w.held > 0)
  return w
}

/** Another opener's work: the document's model, holding a watch of its own. */
async function openElsewhere(ref: typeof NOTES, text: string) {
  const unwatch = await followHostDocument(ref)
  ;(await documentModelOpener())(ref, text)
  releaseWithDocument(documentUri(ref), unwatch)
}

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

    expect(bufferOf(id).unavailable).toEqual({ title: 'File not found', message: 'src/a.ts' })
    expect(documentText(A)).toBeNull()
    expect(error).not.toHaveBeenCalled()
  })

  it('a restored document whose read fails keeps its tab, says why, and opens on the next reconcile', async () => {
    const id = restoredTab(A)
    readDocument.mockRejectedValue(new ApiError('boom', 500))

    await restoreDocuments()

    expect(bufferOf(id).unavailable).toEqual({ title: 'Failed to open file', message: 'a.ts' })
    expect(error).toHaveBeenCalledTimes(1)
    expect(error).toHaveBeenCalledWith('Failed to open file', 'a.ts')
    expect(documentText(A)).toBeNull()

    readDocument.mockResolvedValue({ ref: A, text: 'disk' })
    await reconcileWorkspaceDocuments('ws-restore')

    expect(documentText(A)).toBe('disk')
    expect(bufferOf(id).unavailable).toBeUndefined()
  })

  it('one toast names every file a failure of the same kind hit', async () => {
    restoredTab(A)
    restoredTab(B)
    readDocument.mockRejectedValue(new ApiError('boom', 500))

    await restoreDocuments()

    expect(error).toHaveBeenCalledTimes(1)
    expect(error).toHaveBeenCalledWith('Failed to open file', 'a.ts, b.ts')
  })

  it('names files that share a name by enough of their path to tell them apart', async () => {
    restoredTab(workspaceRef('ws-restore', 'a/x.ts'))
    restoredTab(workspaceRef('ws-restore', 'b/x.ts'))
    readDocument.mockRejectedValue(new ApiError('boom', 500))

    await restoreDocuments()

    expect(error).toHaveBeenCalledWith('Failed to open file', 'a/x.ts, b/x.ts')
  })

  it('a tab whose restore fails after its read says why, and retries', async () => {
    const id = restoredTab(A)
    readDocument.mockResolvedValue({ ref: A, text: 'disk' })
    vi.spyOn(getEditorHost()!.documents, 'open').mockImplementationOnce(() => {
      throw new Error('no model')
    })

    await restoreDocuments()

    expect(bufferOf(id).unavailable).toEqual({ title: 'Failed to open file', message: 'a.ts' })
    expect(error).toHaveBeenCalledWith('Failed to open file', 'a.ts')

    expect(await retryLoad(bufferOf(id))).toBe(true)
    expect(documentText(A)).toBe('disk')
    expect(bufferOf(id).unavailable).toBeUndefined()
  })

  it('when the editor cannot load at all, every restored tab says why and retries', async () => {
    const [a, b] = [restoredTab(A), restoredTab(B)]
    const untitled = restoredTab(UNTITLED, { isVirtual: true })
    await putDrafts([draft(B, 'mine', 'disk'), draft(UNTITLED, 'scratch', '')])
    loadEditorHost.mockRejectedValueOnce(new Error('chunk failed'))

    await restoreDocuments()

    const failed = (message: string) => ({ title: 'Failed to open file', message })
    expect(bufferOf(a).unavailable).toEqual(failed('a.ts'))
    expect(bufferOf(b).unavailable).toEqual(failed('b.ts'))
    expect(bufferOf(untitled).unavailable).toEqual(failed('Untitled'))
    expect(error).toHaveBeenCalledWith('Failed to open file', 'a.ts, b.ts, Untitled')
    expect(await saveActiveBuffer()).toBe(false)
    expect(error).toHaveBeenLastCalledWith('Failed to open file', 'Untitled')

    readDocument.mockResolvedValue({ ref: B, text: 'disk' })
    expect(await openDocument(B)).toBe(b)
    expect(documentText(B)).toBe('mine')
    expect(bufferOf(b)).toMatchObject({ isDirty: true, unavailable: undefined })

    await reconcileWorkspaceDocuments('ws-restore')
    expect(documentText(A)).toBe('disk')
    expect(documentText(UNTITLED)).toBe('scratch')
    expect([bufferOf(a).unavailable, bufferOf(untitled).unavailable]).toEqual([
      undefined,
      undefined,
    ])
  })

  it('reopening a document whose restore failed loads it into its tab', async () => {
    const id = restoredTab(A)
    readDocument.mockRejectedValue(new ApiError('boom', 500))
    await restoreDocuments()

    readDocument.mockRejectedValueOnce(new ApiError('big', 413))
    expect(await openDocument(A)).toBeNull()
    expect(bufferOf(id).unavailable).toEqual({ title: 'File too large to open', message: 'a.ts' })
    expect(error).toHaveBeenLastCalledWith('File too large to open', 'a.ts')

    readDocument.mockResolvedValue({ ref: A, text: 'disk' })
    expect(await openDocument(A)).toBe(id)

    expect(windowPaneStore.getState().buffers).toHaveLength(1)
    expect(documentText(A)).toBe('disk')
    expect(bufferOf(id).unavailable).toBeUndefined()
  })

  it('saving a tab whose restore failed says why and writes nothing', async () => {
    restoredTab(A)
    readDocument.mockRejectedValue(new ApiError('boom', 500))
    await restoreDocuments()
    error.mockClear()

    expect(await saveActiveBuffer()).toBe(false)

    expect(error).toHaveBeenCalledWith('Failed to open file', 'a.ts')
  })

  it('keeps a draft when something else opens its document during the read', async () => {
    const id = restoredTab(A)
    await putDrafts([draft(A, 'mine', 'disk')])
    let finishRead!: (read: { ref: DocumentRef; text: string }) => void
    readDocument.mockReturnValue(new Promise((resolve) => (finishRead = resolve)))

    const restoring = restoreDocuments()
    await vi.waitFor(() => expect(readDocument).toHaveBeenCalled())
    ;(await documentModelOpener())(A, 'disk')
    finishRead({ ref: A, text: 'disk' })
    await restoring

    expect(documentText(A)).toBe('mine')
    expect(bufferOf(id).isDirty).toBe(true)
    expect((await loadDrafts()).get(documentUri(A))?.text).toBe('mine')
  })

  it('a draft whose edits reached disk opens clean and is dropped', async () => {
    const id = restoredTab(A)
    await putDrafts([draft(A, 'saved since', 'before')])
    readDocument.mockResolvedValue({ ref: A, text: 'saved since' })

    await restoreDocuments()

    expect(documentText(A)).toBe('saved since')
    expect(bufferOf(id).isDirty).toBe(false)
    expect(warning).not.toHaveBeenCalled()
    expect((await loadDrafts()).size).toBe(0)
  })

  it('restores an untitled buffer from its draft', async () => {
    const id = restoredTab(UNTITLED, { isVirtual: true })
    await putDrafts([draft(UNTITLED, 'scratch', '')])

    await restoreDocuments()

    expect(documentText(UNTITLED)).toBe('scratch')
    expect(bufferOf(id).isDirty).toBe(true)
    expect(readDocument).not.toHaveBeenCalled()
  })

  it('a layout without editor tabs drops its drafts and never loads the editor', async () => {
    windowPaneStore
      .getState()
      .bufferActions.openContent({ type: 'terminal', sessionId: 's', workspaceId: 'ws-restore' })
    await putDrafts([draft(A, 'orphan', 'disk')])
    loadEditorHost.mockClear()

    await restoreDocuments()

    expect(loadEditorHost).not.toHaveBeenCalled()
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
    expect(findDocumentBuffer(a)?.unavailable).toEqual({
      title: "Can't open this file here",
      message: 'Files outside your workspaces open only in the Crowbar desktop app.',
    })
    expect(documentText(a)).toBeNull()
    expect(documentText(b)).toBe('unsaved')
    expect(findDocumentBuffer(b)?.isDirty).toBe(true)
    expect(error).toHaveBeenCalledTimes(1)
    expect(error).toHaveBeenCalledWith(
      "Can't open this file here",
      'Files outside your workspaces open only in the Crowbar desktop app.',
    )
  })

  it('a failure after the document opened is logged, and said when it cost the draft', async () => {
    restoredTab(A)
    ;(await documentModelOpener())(A, 'disk')
    await putDrafts([draft(A, 'mine', 'disk')])
    vi.spyOn(getEditorHost()!.documents, 'release').mockImplementationOnce(() => {
      throw new Error('boom')
    })
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {})

    await restoreDocuments()

    expect(logged).toHaveBeenCalledWith('Restoring a document failed:', expect.any(Error))
    expect(error).toHaveBeenCalledWith('Unsaved changes not restored', 'a.ts')
  })

  it('the same file failing in two worktrees is named with each workspace', async () => {
    const workspaces = [
      { id: 'ws-x', branch: 'feature-x' },
      { id: 'ws-y', branch: 'feature-y' },
    ]
    useSidebarStore.setState({ repos: [{ id: 'r', name: 'r', workspaces }] as never })
    restoredTab(workspaceRef('ws-x', 'src/a.ts'))
    restoredTab(workspaceRef('ws-y', 'src/a.ts'))
    readDocument.mockRejectedValue(new ApiError('boom', 500))

    await restoreDocuments()

    expect(error).toHaveBeenCalledWith('Failed to open file', 'a.ts (feature-x), a.ts (feature-y)')
    useSidebarStore.setState({ repos: [] })
  })
})

describe('a restored host document and its watch', () => {
  it('is read only once its watch is armed; a workspace document opens none', async () => {
    let arm: () => void = () => {}
    const w = watches(() => new Promise((resolve) => (arm = resolve)))
    restoredTab(NOTES)
    restoredTab(A)
    readDocument.mockImplementation(async (ref: DocumentRef) => {
      w.calls.push(`read ${ref.path}`)
      return { ref, text: 'disk' }
    })

    const restoring = restoreDocuments()
    await vi.waitFor(() => expect(w.calls).toContain('read src/a.ts'))
    expect(w.calls).not.toContain(`read ${NOTES.path}`)
    arm()
    await restoring

    expect(w.calls.filter((c) => c !== 'read src/a.ts')).toEqual([
      'subscribe',
      'opened',
      `read ${NOTES.path}`,
    ])
    expect([documentText(NOTES), w.held]).toEqual(['disk', 1])
    resetWindowPaneStoreForTests()
    expect(w.held).toBe(0)
  })

  it.each([
    ['its file is gone', new ApiError('gone', 404), undefined, 0],
    ['its file is gone under an unsaved draft', new ApiError('gone', 404), 'mine', 1],
  ])('holds its watch only with its model when %s', async (_, failure, unsavedText, held) => {
    const w = watches()
    restoredTab(NOTES)
    if (unsavedText) await putDrafts([draft(NOTES, unsavedText, 'disk')])
    readDocument.mockRejectedValue(failure)

    await restoreDocuments()

    expect([documentText(NOTES), w.held]).toEqual([unsavedText ?? null, held])
  })

  it('lets its watch go when its tab closes during the read', async () => {
    const w = watches()
    restoredTab(NOTES)
    let finish: () => void = () => {}
    readDocument.mockReturnValue(
      new Promise((resolve) => (finish = () => resolve({ ref: NOTES, text: 'disk' }))),
    )

    const restoring = restoreDocuments()
    await vi.waitFor(() => expect(readDocument).toHaveBeenCalled())
    resetWindowPaneStoreForTests()
    finish()
    await restoring

    expect([documentText(NOTES), w.held]).toEqual([null, 0])
  })

  it('opened elsewhere before or during its read keeps that watch alone, under its draft', async () => {
    const w = watches()
    restoredTab(NOTES)
    await putDrafts([draft(NOTES, 'mine', 'disk')])
    let finish: () => void = () => {}
    readDocument.mockReturnValueOnce(
      new Promise((resolve) => (finish = () => resolve({ ref: NOTES, text: 'disk' }))),
    )

    const restoring = restoreDocuments()
    await vi.waitFor(() => expect(readDocument).toHaveBeenCalled())
    await openElsewhere(NOTES, 'disk')
    finish()
    await restoring

    expect([documentText(NOTES), w.held]).toEqual(['mine', 1])
    await restoreDocuments()
    expect(w.calls.filter((c) => c === 'subscribe')).toHaveLength(2)
    resetWindowPaneStoreForTests()
    expect(w.held).toBe(0)
  })

  it('whose watch is refused opens nowhere and says why', async () => {
    watches(() => Promise.reject(new Error('refused')))
    const [id, drifted] = [restoredTab(NOTES), restoredTab(host('/tmp/S5/b.md'))]
    readDocument.mockImplementation(async (ref: DocumentRef) => ({
      ref: ref.path === NOTES.path ? ref : host('/tmp/s5/b.md'),
      text: 'disk',
    }))

    await restoreDocuments()

    expect(bufferOf(id).unavailable).toEqual({ title: 'Failed to open file', message: 'notes.md' })
    expect(bufferOf(drifted).unavailable).toEqual({
      title: 'Failed to open file',
      message: '/tmp/S5/b.md',
    })
    expect(documentText(NOTES)).toBeNull()
  })

  it('under an unsaved draft whose watch is refused opens unwatched, says so once, and follows again when reopened', async () => {
    const w = watches(() => Promise.reject(new Error('refused')))
    const id = restoredTab(NOTES)
    await putDrafts([draft(NOTES, 'mine', 'disk')])
    readDocument.mockResolvedValue({ ref: NOTES, text: 'disk' })

    await restoreDocuments()

    expect([documentText(NOTES), w.held]).toEqual(['mine', 0])
    expect(error).toHaveBeenCalledTimes(1)
    expect(error).toHaveBeenCalledWith('Not following changes on disk', 'notes.md')
    w.opened = () => Promise.resolve()
    expect(await openDocument(NOTES)).toBe(id)
    expect(w.held).toBe(1)
  })
})
