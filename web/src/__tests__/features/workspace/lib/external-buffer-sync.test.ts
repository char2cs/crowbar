import { describe, it, expect, vi, afterEach, beforeAll, beforeEach } from 'vitest'
import { isEditorContent, type EditorContent } from '@/features/panes/types/pane-content'
import type { DocumentRef } from '@/lib/api/documents'
import { MONACO_LOAD_MS, openTestDocument } from '@/__tests__/__fixtures__/documents'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'

const readDiskMock = vi.fn<(wsId: string, path: string) => Promise<string | null>>()
const readDocumentMock = vi.fn()
vi.mock('@/lib/api/documents', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api/documents')>()),
  readDocument: (ref: unknown) => readDocumentMock(ref),
  readDocumentFromDisk: (ref: { workspaceId: string; path: string }) =>
    readDiskMock(ref.workspaceId, ref.path),
}))

const toastWarning = vi.fn()
const toastError = vi.fn()
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: {
    warning: (...args: unknown[]) => toastWarning(...args),
    error: (...args: unknown[]) => toastError(...args),
    info: vi.fn(),
    success: vi.fn(),
  },
}))

const { ApiError } = await import('@/lib/api')
const { HOST_DOCUMENT_TRANSPORT_CODE, workspaceRef } = await import('@/lib/api/documents')
const { documentText, markDocumentSaved, onExternalText, replaceDocumentText, savedDocumentText } =
  await import('@/features/editor/documents/document-text')
const { reconcileWorkspaceDocuments, syncBufferWithDisk } =
  await import('@/features/workspace/lib/external-buffer-sync')
const { findDocumentBuffer, openDocument } =
  await import('@/features/editor/documents/open-document')
const { wsManager } = await import('@/lib/ws/manager')
const { resetConnectionState, useConnectionStore } = await import('@/lib/ws/connection-store')
const { useFileWatcherStore } =
  await import('@/features/file-system/controllers/file-watcher-store')
const { loadEditorHost } = await import('@/features/editor/host/editor-host')

const README = workspaceRef('ws-1', 'README.md')
const NOTES = { authority: 'host', path: '/tmp/s5/notes.md' } as const
const NOTES_WATCH = '/v0/documents/ws?path=%2Ftmp%2Fs5%2Fnotes.md'
const change = (type: string) => ({ type, authority: 'host', path: NOTES.path, wsId: '' })

/** The daemon as one host document meets it: its file, its watch socket, and the calls in order. */
function hostDaemon(disk = 'v1') {
  const d = { disk, calls: [] as string[], held: 0, reads: [] as Promise<string>[] }
  const daemon = Object.assign(d, {
    frame: (_: unknown) => {},
    drop: () => {},
    opened: (): Promise<void> => Promise.resolve(),
  })
  vi.spyOn(wsManager, 'subscribe').mockImplementation((endpoint, cb) => {
    d.calls.push(`subscribe ${endpoint}`)
    d.held++
    daemon.frame = cb
    return () => void (d.calls.push(`unsubscribe ${endpoint}`), d.held--)
  })
  vi.spyOn(wsManager, 'opened').mockImplementation(() => (d.calls.push('opened'), daemon.opened()))
  vi.spyOn(wsManager, 'onDrop').mockImplementation((_, drop) => ((daemon.drop = drop), () => {}))
  vi.spyOn(wsManager, 'subscribed').mockImplementation(() => d.held > 0)
  readDocumentMock.mockImplementation(
    async () => (d.calls.push('read'), { ref: NOTES, text: d.disk }),
  )
  readDiskMock.mockImplementation(() => {
    d.calls.push('read')
    return d.reads[d.reads.push(Promise.resolve(d.disk)) - 1]
  })
  return daemon
}

/** Every read the daemon was sent has landed. */
const landed = (d: ReturnType<typeof hostDaemon>) => Promise.all(d.reads)

let bufferId = ''

/** README.md open in a tab: `text` in the editor, `saved` on disk at the last load. */
async function seed(text = 'old content', saved = text, doc: DocumentRef = README) {
  resetWindowPaneStoreForTests()
  bufferId = await openTestDocument(doc, text, { saved })
}

/** A restored tab whose document has not opened: no text anywhere yet. */
function seedTab(doc: DocumentRef, flags: Partial<EditorContent> = {}) {
  resetWindowPaneStoreForTests()
  const { bufferActions } = windowPaneStore.getState()
  bufferId = bufferActions.openContent({ type: 'editor', doc, name: doc.path })
  windowPaneStore.setState((s) => ({
    buffers: s.buffers.map((b) =>
      b.id === bufferId && isEditorContent(b) ? { ...b, ...flags } : b,
    ),
  }))
}

function getBuffer(): EditorContent {
  return windowPaneStore.getState().buffers.find((b) => b.id === bufferId) as EditorContent
}

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

beforeEach(() => {
  readDiskMock.mockReset()
  readDocumentMock.mockReset()
  toastWarning.mockReset()
  toastError.mockReset()
  useFileWatcherStore.setState({ pendingSaves: new Set() })
})

describe('syncBufferWithDisk', () => {
  it('reloads a clean buffer from disk (text and saved text)', async () => {
    await seed()
    readDiskMock.mockResolvedValue('restored content')

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    const buf = getBuffer()
    // The buffer's own workspace, never the active one: a sibling worktree can hold the same path.
    expect(readDiskMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(documentText(README)).toBe('restored content')
    expect(savedDocumentText(README)).toBe('restored content')
    expect(buf.isDirty).toBe(false)
    expect(buf.hasExternalChange).toBeFalsy()
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('keeps a dirty buffer intact, flags the conflict and toasts once', async () => {
    await seed('user edits', 'old content')
    // A genuine external edit: disk diverges from the baseline we last wrote.
    readDiskMock.mockResolvedValue('someone else wrote this')

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    const buf = getBuffer()
    expect(readDiskMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(documentText(README)).toBe('user edits')
    expect(buf.isDirty).toBe(true)
    expect(buf.hasExternalChange).toBe(true)
    expect(toastWarning).toHaveBeenCalledTimes(1)
    expect(toastWarning.mock.calls[0][0]).toContain('README.md')

    // Already flagged: neither a second toast nor a read.
    readDiskMock.mockClear()
    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))
    expect(toastWarning).toHaveBeenCalledTimes(1)
    expect(readDiskMock).not.toHaveBeenCalled()
  })

  it('does not toast when a dirty buffer matches disk (own-write echo)', async () => {
    // A save can surface as several events; the single-shot marker cancels only the first.
    await seed('newer unsaved keystrokes', 'autosaved value')
    readDiskMock.mockResolvedValue('autosaved value') // disk === saved text

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    const buf = getBuffer()
    expect(readDiskMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(toastWarning).not.toHaveBeenCalled()
    expect(buf.hasExternalChange).toBeFalsy()
    expect(documentText(README)).toBe('newer unsaved keystrokes')
    expect(buf.isDirty).toBe(true)
  })

  it('ignores own-save echoes and consumes the pending-save marker', async () => {
    await seed()
    useFileWatcherStore.getState().markPendingSave('README.md')

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    expect(readDiskMock).not.toHaveBeenCalled()
    expect(documentText(README)).toBe('old content')
    expect(useFileWatcherStore.getState().pendingSaves.has('README.md')).toBe(false)

    // The next external event for the same path is no longer suppressed.
    readDiskMock.mockResolvedValue('external write')
    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))
    expect(documentText(README)).toBe('external write')
  })

  it('does nothing when no open buffer matches the path', async () => {
    await seed('old content', 'old content', workspaceRef('ws-1', 'other.ts'))

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    expect(readDiskMock).not.toHaveBeenCalled()
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('does not clobber edits made while the disk read was in flight', async () => {
    await seed()
    let resolveRead: (v: string) => void = () => {}
    readDiskMock.mockReturnValue(
      new Promise<string>((r) => {
        resolveRead = r
      }),
    )

    const sync = syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))
    // User types while the read is pending.
    replaceDocumentText(README, 'typed meanwhile')
    resolveRead('disk content')
    await sync

    expect(documentText(README)).toBe('typed meanwhile')
    expect(getBuffer().isDirty).toBe(true)
  })

  it('a read older than a save made while it was in flight never lands', async () => {
    await seed('v1')
    let finish: (text: string) => void = () => {}
    readDiskMock.mockReturnValue(new Promise<string>((resolve) => (finish = resolve)))

    const sync = syncBufferWithDisk(README)
    replaceDocumentText(README, 'v2')
    markDocumentSaved(README, 'v2')
    finish('v1')
    await sync

    expect(documentText(README)).toBe('v2')
    expect(getBuffer().isDirty).toBe(false)
  })

  it('does not flag a dirty buffer whose own text is what disk now holds', async () => {
    // A save still in flight: its write reached disk before its answer reached the buffer.
    await seed('mine', 'old')
    readDiskMock.mockResolvedValue('mine')

    await syncBufferWithDisk(README)

    expect(getBuffer().hasExternalChange).toBeFalsy()
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('survives a failed disk read without touching the buffer', async () => {
    await seed()
    readDiskMock.mockResolvedValue(null)

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    expect(documentText(README)).toBe('old content')
  })
})

describe('reconcileWorkspaceDocuments', () => {
  it('reloads an open document whose file changed while the workspace was hidden', async () => {
    await seed()
    readDocumentMock.mockResolvedValue({ ref: README, text: 'changed while hidden' })
    readDiskMock.mockResolvedValue('changed while hidden')

    await reconcileWorkspaceDocuments('ws-1')

    expect(documentText(README)).toBe('changed while hidden')
    expect(getBuffer().isDirty).toBe(false)
  })

  it('marks a clean document whose file is gone as missing', async () => {
    await seed()
    readDocumentMock.mockRejectedValue(new ApiError('gone', 404))

    await reconcileWorkspaceDocuments('ws-1')

    expect(getBuffer().unavailable).toEqual({ title: 'File not found', message: 'README.md' })
  })

  it('opens a missing file that came back', async () => {
    seedTab(README, { unavailable: { title: 'File not found', message: 'README.md' } })
    readDocumentMock.mockResolvedValue({ ref: README, text: 'back again' })

    await reconcileWorkspaceDocuments('ws-1')

    expect(getBuffer().unavailable).toBeUndefined()
    expect(documentText(README)).toBe('back again')
  })

  it('leaves a document the restore has not reached yet alone', async () => {
    seedTab(README)

    await reconcileWorkspaceDocuments('ws-1')

    expect(readDocumentMock).not.toHaveBeenCalled()
    expect(documentText(README)).toBeNull()
  })

  it('a host tab that failed to load reads again through its watch', async () => {
    seedTab(NOTES, { unavailable: { title: 'Failed to open file', message: 'notes.md' } })
    const d = hostDaemon('back')

    await reconcileWorkspaceDocuments('ws-1')

    expect(d.calls).toEqual([`subscribe ${NOTES_WATCH}`, 'opened', 'read'])
    expect(documentText(NOTES)).toBe('back')
    expect(getBuffer().unavailable).toBeUndefined()
  })

  it('a host document open without its watch follows again, and one holding it is left alone', async () => {
    await seed('v1', 'v1', NOTES)
    const d = hostDaemon('v2')

    await reconcileWorkspaceDocuments('ws-1')

    expect(d.calls).toEqual([`subscribe ${NOTES_WATCH}`, 'opened', 'read'])
    expect(documentText(NOTES)).toBe('v2')
    d.calls.length = 0
    await reconcileWorkspaceDocuments('ws-1')
    expect(d.calls).toEqual([])
    resetWindowPaneStoreForTests()
    expect(d.calls).toEqual([`unsubscribe ${NOTES_WATCH}`])
  })

  it('a refusal keeps the tab and says why once', async () => {
    await seed()
    readDocumentMock.mockRejectedValue(new ApiError('refused', 403, HOST_DOCUMENT_TRANSPORT_CODE))

    await reconcileWorkspaceDocuments('ws-1')

    expect(getBuffer().unavailable).toBeUndefined()
    expect(documentText(README)).toBe('old content')
    expect(toastError).toHaveBeenCalledTimes(1)
  })
})

describe('followHostDocument', () => {
  beforeEach(() => resetWindowPaneStoreForTests())
  afterEach(() => {
    vi.restoreAllMocks()
    resetConnectionState()
  })

  it(
    'reads a host document only after its watch is armed, and lets the watch go with it',
    async () => {
      const d = hostDaemon()
      let arm: () => void = () => {}
      d.opened = () => new Promise((resolve) => (arm = resolve))

      const opening = openDocument(NOTES)
      await vi.waitFor(() => expect(d.calls).toEqual([`subscribe ${NOTES_WATCH}`, 'opened']))
      arm()

      expect(await opening).not.toBeNull()
      expect(d.calls).toEqual([`subscribe ${NOTES_WATCH}`, 'opened', 'read'])
      expect(documentText(NOTES)).toBe('v1')
      resetWindowPaneStoreForTests()
      expect(d.calls.at(-1)).toBe(`unsubscribe ${NOTES_WATCH}`)
    },
    MONACO_LOAD_MS,
  )

  it('an absolute path is read to resolve it, then again once its watch is armed', async () => {
    const d = hostDaemon('first')
    d.opened = () => {
      d.disk = 'after the ack'
      return Promise.resolve()
    }

    expect(await openDocument({ absolutePath: NOTES.path })).not.toBeNull()

    expect(d.calls).toEqual(['read', `subscribe ${NOTES_WATCH}`, 'opened', 'read'])
    expect(documentText(NOTES)).toBe('after the ack')
  })

  it('a write between the watch ack and the read ends in the newest text', async () => {
    const d = hostDaemon('v1')
    d.opened = () => {
      d.disk = 'v2'
      return Promise.resolve()
    }
    await openDocument(NOTES)
    expect(documentText(NOTES)).toBe('v2')

    d.frame(change('modified'))
    await landed(d)
    expect(documentText(NOTES)).toBe('v2')
    expect(findDocumentBuffer(NOTES)?.isDirty).toBe(false)

    d.disk = 'v3'
    d.frame(change('modified'))
    await landed(d)
    expect(documentText(NOTES)).toBe('v3')
  })

  it.each(['modified', 'created'])(
    'reloads a clean host document whose file is %s',
    async (type) => {
      const d = hostDaemon('v1')
      await openDocument(NOTES)

      d.disk = 'v2'
      d.frame(change(type))
      await landed(d)

      expect(documentText(NOTES)).toBe('v2')
      expect(findDocumentBuffer(NOTES)?.isDirty).toBe(false)
      expect(toastWarning).not.toHaveBeenCalled()
    },
  )

  it('flags a dirty host document and toasts once', async () => {
    const d = hostDaemon('v1')
    await openDocument(NOTES)
    replaceDocumentText(NOTES, 'mine')

    d.disk = 'theirs'
    d.frame(change('modified'))
    await landed(d)
    d.frame(change('modified'))
    await landed(d)

    expect(findDocumentBuffer(NOTES)).toMatchObject({ isDirty: true, hasExternalChange: true })
    expect(documentText(NOTES)).toBe('mine')
    expect(toastWarning).toHaveBeenCalledTimes(1)
  })

  it('ignores the echo of its own save', async () => {
    const d = hostDaemon('v1')
    await openDocument(NOTES)
    replaceDocumentText(NOTES, 'v2')
    markDocumentSaved(NOTES, 'v2')
    const heard = vi.fn()
    const stop = onExternalText(NOTES, heard)

    d.disk = 'v2'
    d.frame(change('modified'))
    await landed(d)
    stop()

    expect(findDocumentBuffer(NOTES)?.hasExternalChange).toBeFalsy()
    expect(toastWarning).not.toHaveBeenCalled()
    expect(heard).not.toHaveBeenCalled()
  })

  it('leaves the buffer alone when its file is deleted or renamed away', async () => {
    const d = hostDaemon('v1')
    await openDocument(NOTES)
    replaceDocumentText(NOTES, 'mine')
    d.calls.length = 0

    d.frame(change('deleted'))
    d.frame(change('renamed'))

    expect(d.calls).toEqual([])
    expect(documentText(NOTES)).toBe('mine')
    expect(findDocumentBuffer(NOTES)?.isDirty).toBe(true)
  })

  it('opens no host document whose watch is refused, and says why', async () => {
    const d = hostDaemon('v1')
    d.opened = () => Promise.reject(new Error('closed before it opened'))
    readDocumentMock.mockRejectedValueOnce(
      new ApiError('refused', 403, HOST_DOCUMENT_TRANSPORT_CODE),
    )

    expect(await openDocument(NOTES)).toBeNull()

    expect(findDocumentBuffer(NOTES)).toBeNull()
    expect(toastError).toHaveBeenCalledWith("Can't open this file here", expect.any(String))
    expect(d.calls).toContain(`unsubscribe ${NOTES_WATCH}`)
    expect(await openDocument(NOTES)).toBeNull()
    expect(toastError).toHaveBeenLastCalledWith('Failed to open file', 'notes.md')
  })

  it('says once that it stopped following when the daemon keeps refusing its watch', async () => {
    const d = hostDaemon('v1')
    await openDocument(NOTES)
    useConnectionStore.setState({ status: 'connected' })
    let attempts = 0
    d.opened = () => (++attempts, Promise.reject(new Error('refused')))

    d.drop()

    await vi.waitFor(() => expect(toastWarning).toHaveBeenCalledTimes(1))
    expect(toastWarning).toHaveBeenCalledWith(
      'Not following changes to notes.md',
      expect.any(String),
    )
    expect(attempts).toBe(2)
  })

  it('a drop while the daemon is down says nothing, and the reopened socket reads once more', async () => {
    const d = hostDaemon('v1')
    await openDocument(NOTES)
    useConnectionStore.setState({ status: 'disconnected' })
    const outcomes = [false, false, true]
    d.opened = () => (outcomes.shift() ? Promise.resolve() : Promise.reject(new Error('down')))

    d.drop()
    await vi.waitFor(() => expect(outcomes).toEqual([]))
    d.calls.length = 0
    d.disk = 'while it was down'
    d.frame({ reconnected: true })
    await landed(d)

    expect(toastWarning).not.toHaveBeenCalled()
    expect(d.calls).toEqual(['read'])
    expect(documentText(NOTES)).toBe('while it was down')
  })
})
