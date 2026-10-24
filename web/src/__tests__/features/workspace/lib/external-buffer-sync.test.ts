import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { EditorContent } from '@/features/panes/types/pane-content'
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
const { reconcileHostBuffersWithDisk, syncBufferWithDisk } =
  await import('@/features/workspace/lib/external-buffer-sync')
const { useFileWatcherStore } =
  await import('@/features/file-system/controllers/file-watcher-store')

function makeBuffer(overrides: Partial<EditorContent> = {}): EditorContent {
  return {
    id: 'buf-1',
    type: 'editor',
    authority: 'workspace',
    path: 'README.md',
    name: 'README.md',
    content: 'old content',
    savedContent: 'old content',
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    tokens: [],
    workspaceId: 'ws-1',
    ...overrides,
  }
}

// Buffers are window-level: syncBufferWithDisk takes the document's ref and
// reads/writes the one `windowPaneStore` singleton.
function seedBuffers(buffers: EditorContent[]): void {
  resetWindowPaneStoreForTests()
  windowPaneStore.setState((s) => {
    s.buffers = buffers
    return s
  })
}

function getBuffer(id = 'buf-1'): EditorContent {
  return windowPaneStore.getState().buffers.find((b) => b.id === id) as EditorContent
}

beforeEach(() => {
  readDiskMock.mockReset()
  readDocumentMock.mockReset()
  toastWarning.mockReset()
  toastError.mockReset()
  useFileWatcherStore.setState({ pendingSaves: new Set() })
})

describe('syncBufferWithDisk', () => {
  it('reloads a clean buffer from disk (content and savedContent)', async () => {
    seedBuffers([makeBuffer()])
    readDiskMock.mockResolvedValue('restored content')

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    const buf = getBuffer()
    // The read targets the buffer's own workspace, never the active one —
    // an active-workspace read would load a sibling worktree's file after a
    // mid-flight workspace switch.
    expect(readDiskMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(buf.content).toBe('restored content')
    expect(buf.savedContent).toBe('restored content')
    expect(buf.isDirty).toBe(false)
    expect(buf.hasExternalChange).toBe(false)
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('keeps a dirty buffer intact, flags the conflict and toasts once', async () => {
    seedBuffers([makeBuffer({ content: 'user edits', savedContent: 'old content', isDirty: true })])
    // A genuine external edit: disk diverges from the baseline we last wrote.
    readDiskMock.mockResolvedValue('someone else wrote this')

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    const buf = getBuffer()
    expect(readDiskMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(buf.content).toBe('user edits')
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
    seedBuffers([
      makeBuffer({
        content: 'newer unsaved keystrokes',
        savedContent: 'autosaved value',
        isDirty: true,
      }),
    ])
    readDiskMock.mockResolvedValue('autosaved value') // disk === savedContent

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    const buf = getBuffer()
    expect(readDiskMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(toastWarning).not.toHaveBeenCalled()
    expect(buf.hasExternalChange).toBeFalsy()
    // The in-flight unsaved edits are left untouched.
    expect(buf.content).toBe('newer unsaved keystrokes')
    expect(buf.isDirty).toBe(true)
  })

  it('ignores own-save echoes and consumes the pending-save marker', async () => {
    seedBuffers([makeBuffer()])
    useFileWatcherStore.getState().markPendingSave('README.md')

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    expect(readDiskMock).not.toHaveBeenCalled()
    expect(getBuffer().content).toBe('old content')
    expect(useFileWatcherStore.getState().pendingSaves.has('README.md')).toBe(false)

    // The next external event for the same path is no longer suppressed.
    readDiskMock.mockResolvedValue('external write')
    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))
    expect(getBuffer().content).toBe('external write')
  })

  it('does nothing when no open buffer matches the path', async () => {
    seedBuffers([makeBuffer({ path: 'other.ts', name: 'other.ts' })])

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    expect(readDiskMock).not.toHaveBeenCalled()
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('does not clobber edits made while the disk read was in flight', async () => {
    seedBuffers([makeBuffer()])
    let resolveRead: (v: string) => void = () => {}
    readDiskMock.mockReturnValue(
      new Promise<string>((r) => {
        resolveRead = r
      }),
    )

    const sync = syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))
    // User types while the read is pending.
    windowPaneStore.setState((s) => {
      s.buffers = [makeBuffer({ content: 'typed meanwhile', isDirty: true })]
      return s
    })
    resolveRead('disk content')
    await sync

    const buf = getBuffer()
    expect(buf.content).toBe('typed meanwhile')
    expect(buf.isDirty).toBe(true)
  })

  it('survives a failed disk read without touching the buffer', async () => {
    seedBuffers([makeBuffer()])
    readDiskMock.mockResolvedValue(null)

    await syncBufferWithDisk(workspaceRef('ws-1', 'README.md'))

    expect(getBuffer().content).toBe('old content')
  })
})

describe('reconcileHostBuffersWithDisk', () => {
  const host = (id: string, path: string) =>
    makeBuffer({ id, authority: 'host', workspaceId: '', path, name: path.split('/').pop() })

  it('reloads a restored host document from disk', async () => {
    seedBuffers([host('h-1', '/tmp/a.md'), makeBuffer()])
    readDocumentMock.mockResolvedValue({ text: 'on disk' })
    readDiskMock.mockResolvedValue('on disk')

    await reconcileHostBuffersWithDisk()

    expect(readDocumentMock).toHaveBeenCalledTimes(1)
    expect(readDocumentMock).toHaveBeenCalledWith({ authority: 'host', path: '/tmp/a.md' })
    expect(getBuffer('h-1').content).toBe('on disk')
  })

  it('a restored host document the daemon refuses keeps its tab and says why once', async () => {
    seedBuffers([host('h-1', '/tmp/a.md'), host('h-2', '/tmp/b.md')])
    readDocumentMock.mockRejectedValue(new ApiError('refused', 403, HOST_DOCUMENT_TRANSPORT_CODE))

    await reconcileHostBuffersWithDisk()

    expect(windowPaneStore.getState().buffers.map((b) => b.id)).toEqual(['h-1', 'h-2'])
    expect(getBuffer('h-1').content).toBe('old content')
    expect(getBuffer('h-1').fileMissing).toBeFalsy()
    expect(toastError).toHaveBeenCalledTimes(1)
    expect(toastError).toHaveBeenCalledWith(
      "Can't open this file here",
      'Files outside your workspaces open only in the Crowbar desktop app.',
    )
  })
})
