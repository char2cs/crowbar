import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useWorkspaceEffects } from '@/features/workspace/stores/hooks/use-workspace-effects'
import { useFileSystemStore } from '@/features/file-system/controllers/store'

const mockBufferActions = {
  openContent: vi.fn(() => 'buf-id'),
  promotePreview: vi.fn(),
}

vi.mock('@/features/workspace/stores/hooks/use-buffer-store', () => ({
  useBufferActions: () => mockBufferActions,
}))

const { fetchFileTree, subscribe } = vi.hoisted(() => ({
  fetchFileTree: vi.fn(),
  subscribe: vi.fn(() => () => {}),
}))

vi.mock('@/features/files/lib/file-tree-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/files/lib/file-tree-api')>()
  return { ...actual, fetchFileTree }
})

vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe, send: vi.fn() } }))

beforeEach(() => {
  vi.clearAllMocks()
  fetchFileTree.mockResolvedValue([
    { name: 'src', path: 'src', isDir: true, children: undefined },
    { name: 'README.md', path: 'README.md', isDir: false },
  ])
  useFileSystemStore.setState({ files: [], fileTree: [] })
})

describe('useWorkspaceEffects', () => {
  it('seeds the root tree from the workspace-scoped backend', async () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(fetchFileTree).toHaveBeenCalledWith('ws-test')
    await waitFor(() => {
      expect(useFileSystemStore.getState().files).toHaveLength(2)
    })
  })

  it('wires workspace-scoped file open/select handlers', async () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    await waitFor(() => {
      expect(useFileSystemStore.getState().handleFileOpen).toBeTypeOf('function')
      expect(useFileSystemStore.getState().handleFileSelect).toBeTypeOf('function')
    })
  })

  it('subscribes to the files WS topic for the workspace', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(subscribe).toHaveBeenCalledWith('/v0/ws/files?wsId=ws-test', expect.any(Function))
  })

  it('subscribes to the git WS topic for the workspace', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(subscribe).toHaveBeenCalledWith('/v0/ws/git?wsId=ws-test', expect.any(Function))
  })

  // Regression: an editor save dispatches "git-status-updated" but the git
  // store only refreshed on the backend watcher's WS event, so the Changes
  // panel went stale when that event was missed. The effect must also reload
  // status (debounced) on the local event.
  it('reloads git status when an editor save dispatches git-status-updated', async () => {
    vi.useFakeTimers()
    try {
      const { useGitStore } = await import('@/features/git/stores/git-store')
      const reloadStatusAndLog = vi.fn(() => Promise.resolve())
      const original = useGitStore.getState().actions
      useGitStore.setState({ actions: { ...original, reloadStatusAndLog } })

      renderHook(() => useWorkspaceEffects('ws-test'))
      window.dispatchEvent(new CustomEvent('git-status-updated', { detail: { filePath: 'a.ts' } }))
      await vi.advanceTimersByTimeAsync(500)

      expect(reloadStatusAndLog).toHaveBeenCalledWith('ws-test')
      useGitStore.setState({ actions: original })
    } finally {
      vi.useRealTimers()
    }
  })

  // Regression: the backend streams identical git frames more often than the
  // debounce window (~165ms apart, indefinitely). A resetting debounce starved
  // forever — neither the WS push nor the editor-save event ever produced a
  // refetch. The timer must coalesce (fire from the first trigger) and
  // identical consecutive frames must not retrigger it.
  it('reloads status despite a continuous stream of git frames', async () => {
    vi.useFakeTimers()
    try {
      const { useGitStore } = await import('@/features/git/stores/git-store')
      const reloadStatusAndLog = vi.fn(() => Promise.resolve())
      const original = useGitStore.getState().actions
      useGitStore.setState({ actions: { ...original, reloadStatusAndLog } })

      renderHook(() => useWorkspaceEffects('ws-test'))
      const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
      const gitCall = calls.find(([ep]) => ep.startsWith('/v0/ws/git'))
      expect(gitCall).toBeDefined()
      const onGitFrame = gitCall![1]

      // Stream identical frames every 150ms for 1.5s — faster than the 400ms window.
      for (let i = 0; i < 10; i++) {
        onGitFrame({ branch: 'main', files: [] })
        await vi.advanceTimersByTimeAsync(150)
      }
      // The first frame's coalesced timer must have fired exactly once;
      // identical repeats neither reset nor re-arm it.
      expect(reloadStatusAndLog).toHaveBeenCalledTimes(1)
      expect(reloadStatusAndLog).toHaveBeenCalledWith('ws-test')

      // A frame whose payload differs re-arms the reload.
      onGitFrame({ branch: 'main', files: [{ path: 'a.ts' }] })
      await vi.advanceTimersByTimeAsync(500)
      expect(reloadStatusAndLog).toHaveBeenCalledTimes(2)

      useGitStore.setState({ actions: original })
    } finally {
      vi.useRealTimers()
    }
  })

  // BUG-017: after the push-driven reload, open diff views (the "Uncommitted
  // Changes" tab, single-file diff tabs) must be told to refetch — they listen
  // on the window-level "git-status-changed" event.
  it('dispatches git-status-changed after the push-driven reload completes', async () => {
    vi.useFakeTimers()
    try {
      const { useGitStore } = await import('@/features/git/stores/git-store')
      const reloadStatusAndLog = vi.fn(() => Promise.resolve())
      const original = useGitStore.getState().actions
      useGitStore.setState({ actions: { ...original, reloadStatusAndLog } })

      const onStatusChanged = vi.fn()
      window.addEventListener('git-status-changed', onStatusChanged)

      renderHook(() => useWorkspaceEffects('ws-test'))
      const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
      const gitCall = calls.find(([ep]) => ep.startsWith('/v0/ws/git'))
      gitCall![1]({ branch: 'main', files: [] })
      await vi.advanceTimersByTimeAsync(500)

      expect(reloadStatusAndLog).toHaveBeenCalledWith('ws-test')
      expect(onStatusChanged).toHaveBeenCalledTimes(1)

      window.removeEventListener('git-status-changed', onStatusChanged)
      useGitStore.setState({ actions: original })
    } finally {
      vi.useRealTimers()
    }
  })

  it('editor-save event reloads status even while identical WS frames stream', async () => {
    vi.useFakeTimers()
    try {
      const { useGitStore } = await import('@/features/git/stores/git-store')
      const reloadStatusAndLog = vi.fn(() => Promise.resolve())
      const original = useGitStore.getState().actions
      useGitStore.setState({ actions: { ...original, reloadStatusAndLog } })

      renderHook(() => useWorkspaceEffects('ws-test'))
      const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
      const gitCall = calls.find(([ep]) => ep.startsWith('/v0/ws/git'))
      const onGitFrame = gitCall![1]

      // Settle the initial frame's reload first.
      onGitFrame({ branch: 'main', files: [] })
      await vi.advanceTimersByTimeAsync(500)
      reloadStatusAndLog.mockClear()

      // Save dispatches the event while the identical-frame spam continues.
      window.dispatchEvent(new CustomEvent('git-status-updated', { detail: { filePath: 'a.ts' } }))
      for (let i = 0; i < 4; i++) {
        onGitFrame({ branch: 'main', files: [] })
        await vi.advanceTimersByTimeAsync(150)
      }
      expect(reloadStatusAndLog).toHaveBeenCalledWith('ws-test')

      useGitStore.setState({ actions: original })
    } finally {
      vi.useRealTimers()
    }
  })
})
