import { describe, it, expect, vi, beforeEach } from 'vitest'
import { createStore } from 'zustand'
import type { EditorContent } from '@/features/panes/types/pane-content'
import type { WorkspaceStore } from '@/features/workspace/stores/workspace-store'

const readWorkspaceFileMock = vi.fn<(wsId: string, path: string) => Promise<string>>()
vi.mock('@/features/file-system/controllers/platform', () => ({
  readWorkspaceFile: (wsId: string, path: string) => readWorkspaceFileMock(wsId, path),
}))

const toastWarning = vi.fn()
// The toast API moved to the window toast-store (cossui migration); mock the path
// external-buffer-sync actually imports, not the legacy @/components/ui/toast.
vi.mock('@/features/window/stores/toast-store', () => ({
  toast: {
    warning: (...args: unknown[]) => toastWarning(...args),
    error: vi.fn(),
    info: vi.fn(),
    success: vi.fn(),
  },
}))

const { syncBufferWithDisk } = await import('@/features/workspace/lib/external-buffer-sync')
const { useFileWatcherStore } =
  await import('@/features/file-system/controllers/file-watcher-store')

function makeBuffer(overrides: Partial<EditorContent> = {}): EditorContent {
  return {
    id: 'buf-1',
    type: 'editor',
    path: 'README.md',
    name: 'README.md',
    content: 'old content',
    savedContent: 'old content',
    isDirty: false,
    isVirtual: false,
    isPinned: false,
    isPreview: false,
    isActive: true,
    tokens: [],
    ...overrides,
  }
}

function makeWorkspaceStore(buffers: EditorContent[]): WorkspaceStore {
  return createStore<{ workspaceId: string; buffers: EditorContent[] }>(() => ({
    workspaceId: 'ws-1',
    buffers,
  })) as unknown as WorkspaceStore
}

beforeEach(() => {
  readWorkspaceFileMock.mockReset()
  toastWarning.mockReset()
  useFileWatcherStore.setState({ pendingSaves: new Set() })
})

describe('syncBufferWithDisk', () => {
  it('reloads a clean buffer from disk (content and savedContent)', async () => {
    const store = makeWorkspaceStore([makeBuffer()])
    readWorkspaceFileMock.mockResolvedValue('restored content')

    await syncBufferWithDisk(store, 'README.md')

    const buf = store.getState().buffers[0] as EditorContent
    // The read targets the buffer's own workspace, never the active one —
    // an active-workspace read would load a sibling worktree's file after a
    // mid-flight workspace switch.
    expect(readWorkspaceFileMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(buf.content).toBe('restored content')
    expect(buf.savedContent).toBe('restored content')
    expect(buf.isDirty).toBe(false)
    expect(buf.hasExternalChange).toBe(false)
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('keeps a dirty buffer intact, flags the conflict and toasts once', async () => {
    const store = makeWorkspaceStore([
      makeBuffer({ content: 'user edits', savedContent: 'old content', isDirty: true }),
    ])
    // A genuine external edit: disk diverges from the baseline we last wrote.
    readWorkspaceFileMock.mockResolvedValue('someone else wrote this')

    await syncBufferWithDisk(store, 'README.md')

    const buf = store.getState().buffers[0] as EditorContent
    expect(readWorkspaceFileMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(buf.content).toBe('user edits')
    expect(buf.isDirty).toBe(true)
    expect(buf.hasExternalChange).toBe(true)
    expect(toastWarning).toHaveBeenCalledTimes(1)
    expect(toastWarning.mock.calls[0][0]).toContain('README.md')

    // A second external change while already flagged must not re-toast (and must
    // not even hit the disk — the hasExternalChange guard returns first).
    readWorkspaceFileMock.mockClear()
    await syncBufferWithDisk(store, 'README.md')
    expect(toastWarning).toHaveBeenCalledTimes(1)
    expect(readWorkspaceFileMock).not.toHaveBeenCalled()
  })

  it('does not toast when a dirty buffer matches disk (own-write echo)', async () => {
    // Regression: an editor save can surface as several FS "modified" events;
    // the single-shot pendingSaves marker only cancels the first. A straggler
    // event on a still-dirty buffer must NOT produce a phantom "changed on disk"
    // toast when the on-disk bytes are exactly what we last wrote.
    const store = makeWorkspaceStore([
      makeBuffer({
        content: 'newer unsaved keystrokes',
        savedContent: 'autosaved value',
        isDirty: true,
      }),
    ])
    readWorkspaceFileMock.mockResolvedValue('autosaved value') // disk === savedContent

    await syncBufferWithDisk(store, 'README.md')

    const buf = store.getState().buffers[0] as EditorContent
    expect(readWorkspaceFileMock).toHaveBeenCalledWith('ws-1', 'README.md')
    expect(toastWarning).not.toHaveBeenCalled()
    expect(buf.hasExternalChange).toBeFalsy()
    // The in-flight unsaved edits are left untouched.
    expect(buf.content).toBe('newer unsaved keystrokes')
    expect(buf.isDirty).toBe(true)
  })

  it('ignores own-save echoes and consumes the pending-save marker', async () => {
    const store = makeWorkspaceStore([makeBuffer()])
    useFileWatcherStore.getState().markPendingSave('README.md')

    await syncBufferWithDisk(store, 'README.md')

    expect(readWorkspaceFileMock).not.toHaveBeenCalled()
    expect((store.getState().buffers[0] as EditorContent).content).toBe('old content')
    expect(useFileWatcherStore.getState().pendingSaves.has('README.md')).toBe(false)

    // The next external event for the same path is no longer suppressed.
    readWorkspaceFileMock.mockResolvedValue('external write')
    await syncBufferWithDisk(store, 'README.md')
    expect((store.getState().buffers[0] as EditorContent).content).toBe('external write')
  })

  it('does nothing when no open buffer matches the path', async () => {
    const store = makeWorkspaceStore([makeBuffer({ path: 'other.ts', name: 'other.ts' })])

    await syncBufferWithDisk(store, 'README.md')

    expect(readWorkspaceFileMock).not.toHaveBeenCalled()
    expect(toastWarning).not.toHaveBeenCalled()
  })

  it('does not clobber edits made while the disk read was in flight', async () => {
    const store = makeWorkspaceStore([makeBuffer()])
    let resolveRead: (v: string) => void = () => {}
    readWorkspaceFileMock.mockReturnValue(
      new Promise<string>((r) => {
        resolveRead = r
      }),
    )

    const sync = syncBufferWithDisk(store, 'README.md')
    // User types while the read is pending.
    store.setState({ buffers: [makeBuffer({ content: 'typed meanwhile', isDirty: true })] })
    resolveRead('disk content')
    await sync

    const buf = store.getState().buffers[0] as EditorContent
    expect(buf.content).toBe('typed meanwhile')
    expect(buf.isDirty).toBe(true)
  })

  it('survives a failed disk read without touching the buffer', async () => {
    const store = makeWorkspaceStore([makeBuffer()])
    readWorkspaceFileMock.mockRejectedValue(new Error('boom'))

    await syncBufferWithDisk(store, 'README.md')

    expect((store.getState().buffers[0] as EditorContent).content).toBe('old content')
  })
})
