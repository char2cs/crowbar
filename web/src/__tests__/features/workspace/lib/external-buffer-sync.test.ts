import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest'
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
// The toast API moved to the window toast-store (cossui migration); mock the path
// external-buffer-sync actually imports, not the legacy @/components/ui/toast.
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
const { documentText, replaceDocumentText, savedDocumentText } =
  await import('@/features/editor/documents/document-text')
const { reconcileWorkspaceDocuments, syncBufferWithDisk } =
  await import('@/features/workspace/lib/external-buffer-sync')
const { useFileWatcherStore } =
  await import('@/features/file-system/controllers/file-watcher-store')
const { loadEditorHost } = await import('@/features/editor/host/editor-host')

const README = workspaceRef('ws-1', 'README.md')

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
    // The read targets the buffer's own workspace, never the active one —
    // an active-workspace read would load a sibling worktree's file after a
    // mid-flight workspace switch.
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

    // A second external change while already flagged must not re-toast (and must
    // not even hit the disk — the hasExternalChange guard returns first).
    readDiskMock.mockClear()
    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))
    expect(toastWarning).toHaveBeenCalledTimes(1)
    expect(readDiskMock).not.toHaveBeenCalled()
  })

  it('does not toast when a dirty buffer matches disk (own-write echo)', async () => {
    // Regression: an editor save can surface as several FS "modified" events;
    // the single-shot pendingSaves marker only cancels the first. A straggler
    // event on a still-dirty buffer must NOT produce a phantom "changed on disk"
    // toast when the on-disk bytes are exactly what we last wrote.
    await seed('newer unsaved keystrokes', 'autosaved value')
    readDiskMock.mockResolvedValue('autosaved value') // disk === saved text

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    const buf = getBuffer()
    expect(readDiskMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(toastWarning).not.toHaveBeenCalled()
    expect(buf.hasExternalChange).toBeFalsy()
    // The in-flight unsaved edits are left untouched.
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

  it('a refusal keeps the tab and says why once', async () => {
    await seed()
    readDocumentMock.mockRejectedValue(new ApiError('refused', 403, HOST_DOCUMENT_TRANSPORT_CODE))

    await reconcileWorkspaceDocuments('ws-1')

    expect(getBuffer().unavailable).toBeUndefined()
    expect(documentText(README)).toBe('old content')
    expect(toastError).toHaveBeenCalledTimes(1)
  })
})
