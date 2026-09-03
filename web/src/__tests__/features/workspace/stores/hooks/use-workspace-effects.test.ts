import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import {
  duplicateDestPath,
  useWorkspaceEffects,
} from '@/features/workspace/stores/hooks/use-workspace-effects'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { setWorkspaceScope } from '@/lib/workspace-scope'
import {
  __resetActivationFreshnessForTests,
  markWorkspaceDeactivated,
  WARM_FRESHNESS_WINDOW_MS,
} from '@/features/workspace/lib/activation-freshness'
import { resetWorkspaceScopedStores } from '@/features/workspace/lib/reset-workspace-scoped-stores'
import type { AppFile } from '@/features/file-system/types/app'

// Both live topics are addressed by the chat that owns 'ws-test''s worktree —
// the flat chat prefix — which is why the scope below carries an owningChatId.
// The home workspace further down is the deliberate exception: it has files but
// no worktree and so no chat, and keeps its own project-level base.
const FILES_BASE = '/v0/chats/chat-test/files'
const GIT_BASE = '/v0/chats/chat-test/git'

const mockBufferActions = {
  openContent: vi.fn(() => 'buf-id'),
  promotePreview: vi.fn(),
}

vi.mock('@/features/workspace/stores/hooks/use-buffer-store', () => ({
  useBufferActions: () => mockBufferActions,
}))

const { fetchFileTree, copyFileNode, subscribe, fetchAllGitData } = vi.hoisted(() => ({
  fetchFileTree: vi.fn(),
  copyFileNode: vi.fn(),
  subscribe: vi.fn(() => () => {}),
  fetchAllGitData: vi.fn(),
}))

vi.mock('@/features/files/lib/file-tree-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/files/lib/file-tree-api')>()
  return { ...actual, fetchFileTree, copyFileNode }
})

// Mock only fetchAllGitData (the four-request git seed) so the warm-fast-path
// tests can assert it was/wasn't fired; useGitStore stays the real store.
vi.mock('@/features/git/stores/git-store', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/git/stores/git-store')>()
  return { ...actual, fetchAllGitData }
})

vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe, send: vi.fn() } }))

const { revealItemInFinder, resolveWorkspaceRootPath, toastError } = vi.hoisted(() => ({
  revealItemInFinder: vi.fn().mockResolvedValue(undefined),
  resolveWorkspaceRootPath: vi.fn(() => '/disk/worktree'),
  toastError: vi.fn(),
}))

vi.mock('@/lib/crowbar-bridge', () => ({ revealItemInFinder }))
vi.mock('@/lib/workspace/resolve-root-path', () => ({ resolveWorkspaceRootPath }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

beforeEach(() => {
  vi.clearAllMocks()
  __resetActivationFreshnessForTests()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-test', owningChatId: 'chat-test' })
  fetchFileTree.mockResolvedValue([
    { name: 'src', path: 'src', isDir: true, children: undefined },
    { name: 'README.md', path: 'README.md', isDir: false },
  ])
  copyFileNode.mockResolvedValue(undefined)
  fetchAllGitData.mockResolvedValue(null)
  // Default: no prior workspace data in the global stores (cold mount seeds).
  useFileSystemStore.setState({ rootFolderPath: null, files: [], fileTree: [] })
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

  // Reveal in Finder: the tab menu passes workspace-relative buffer paths, the
  // explorer passes absolute ones, and virtual buffers must never reach the OS.
  it('wires handleRevealInFolder to reveal absolute + root-joined relative paths', async () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    await waitFor(() => {
      expect(useFileSystemStore.getState().handleRevealInFolder).toBeTypeOf('function')
    })
    const reveal = useFileSystemStore.getState().handleRevealInFolder!

    reveal('/abs/path/file.ts')
    expect(revealItemInFinder).toHaveBeenCalledWith('/abs/path/file.ts')

    reveal('api/main.go')
    expect(revealItemInFinder).toHaveBeenCalledWith('/disk/worktree/api/main.go')

    revealItemInFinder.mockClear()
    reveal('remote://x/y.ts')
    expect(revealItemInFinder).not.toHaveBeenCalled()

    // No resolvable root → no OS call rather than a bogus relative reveal.
    resolveWorkspaceRootPath.mockReturnValueOnce(undefined as unknown as string)
    reveal('api/main.go')
    expect(revealItemInFinder).not.toHaveBeenCalled()
  })

  it('surfaces a reveal failure as an error toast', async () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    await waitFor(() => {
      expect(useFileSystemStore.getState().handleRevealInFolder).toBeTypeOf('function')
    })

    revealItemInFinder.mockRejectedValueOnce(new Error('finder exploded'))
    useFileSystemStore.getState().handleRevealInFolder!('/abs/file.ts')

    await waitFor(() => {
      expect(toastError).toHaveBeenCalledWith('Reveal in Finder failed', 'finder exploded')
    })
  })

  // Duplicate must be ONE server-side copy call (byte-faithful for binaries)
  // with the collision-free "<name> copy" destination derived client-side.
  it('wires handleDuplicatePath to the copy verb with a collision-free name', async () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    await waitFor(() => {
      expect(useFileSystemStore.getState().handleDuplicatePath).toBeTypeOf('function')
    })
    useFileSystemStore.setState({
      files: [
        { name: 'img.png', path: 'img.png', isDir: false },
        { name: 'img copy.png', path: 'img copy.png', isDir: false },
      ],
    })

    await useFileSystemStore.getState().handleDuplicatePath!('img.png')

    expect(copyFileNode).toHaveBeenCalledTimes(1)
    expect(copyFileNode).toHaveBeenCalledWith('ws-test', 'img.png', 'img copy 2.png')
  })

  it('surfaces a duplicate failure as an error toast', async () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    await waitFor(() => {
      expect(useFileSystemStore.getState().handleDuplicatePath).toBeTypeOf('function')
    })

    copyFileNode.mockRejectedValueOnce(new Error('workspace locked'))
    await useFileSystemStore.getState().handleDuplicatePath!('a.txt')

    expect(toastError).toHaveBeenCalledWith('Duplicate failed', 'workspace locked')
  })

  it('subscribes to the files WS topic for the workspace', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(subscribe).toHaveBeenCalledWith(`${FILES_BASE}/ws`, expect.any(Function))
  })

  it('subscribes to the git WS topic for the workspace', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(subscribe).toHaveBeenCalledWith(`${GIT_BASE}/status`, expect.any(Function))
  })

  // The home (project-level) workspace has no git surface: the project root is
  // not a git worktree. Its scope deliberately records NO owning chat, so the
  // git base could not even be built for it — the effect must skip the git
  // stream before reaching for one, while keeping files (the tree watcher).
  //
  // Files is the reason this stays a real assertion rather than a formality
  // now that files is chat-scoped too: home has no chat to be named by, so its
  // stream must still resolve to the project's own /home/files/ws leaf. A
  // filesBaseForWorkspace that reached for an owning chat unconditionally
  // would throw here and take the home file tree down with it.
  it('skips the git stream for a home workspace but keeps the files stream', () => {
    setWorkspaceScope({ projectId: 'p1', repoId: '', wsId: 'home-ws' })
    renderHook(() => useWorkspaceEffects('home-ws'))

    const endpoints = (subscribe.mock.calls as unknown as [string][]).map(([ep]) => ep)
    expect(endpoints).toContain('/v0/projects/p1/home/files/ws')
    expect(endpoints.some((ep) => ep.includes('/git/'))).toBe(false)
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
      const gitCall = calls.find(([ep]) => ep.startsWith(GIT_BASE))
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
      const gitCall = calls.find(([ep]) => ep.startsWith(GIT_BASE))
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
      const gitCall = calls.find(([ep]) => ep.startsWith(GIT_BASE))
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

  // ── Warm reactivation fast path (Task 33 Target A) ────────────────────────
  // A workspace hidden only briefly, whose own data still occupies the global
  // stores, must NOT re-seed on return — that fan-out (tree fetch + 4-request
  // git load) plus the synchronous `isFileTreeLoading:true` flash was the bulk
  // of the warm-switch cost. Correctness-first: any doubt re-seeds.
  describe('warm reactivation fast path', () => {
    const keptTree: AppFile[] = [{ name: 'kept.ts', path: 'kept.ts', isDir: false }]

    // Simulate a warm return: this workspace's own tree still occupies the
    // global store, and it was hidden `agoMs` ago.
    function seedWarm(agoMs: number) {
      useFileSystemStore.setState({
        rootFolderPath: 'ws-test',
        files: keptTree,
        fileTree: keptTree,
        isFileTreeLoading: false,
        handleFileOpen: async () => {},
      })
      markWorkspaceDeactivated('ws-test', Date.now() - agoMs)
    }

    it('keeps the loaded tree and skips the refetch when hidden briefly', () => {
      seedWarm(100)
      renderHook(() => useWorkspaceEffects('ws-test'))

      expect(fetchFileTree).not.toHaveBeenCalled()
      const fs = useFileSystemStore.getState()
      // Same reference preserved — no wipe, no "Loading…" flash on the switch frame.
      expect(fs.files).toBe(keptTree)
      expect(fs.isFileTreeLoading).toBe(false)
    })

    it('re-seeds when the workspace was hidden longer than the freshness window', () => {
      seedWarm(WARM_FRESHNESS_WINDOW_MS + 1)
      renderHook(() => useWorkspaceEffects('ws-test'))
      expect(fetchFileTree).toHaveBeenCalledWith('ws-test')
    })

    it('re-seeds when another workspace clobbered the global tree while away', () => {
      // Recent hide, but the global store now holds a DIFFERENT workspace.
      useFileSystemStore.setState({
        rootFolderPath: 'other-ws',
        files: [{ name: 'x', path: 'x', isDir: false }],
        fileTree: [{ name: 'x', path: 'x', isDir: false }],
        isFileTreeLoading: false,
      })
      markWorkspaceDeactivated('ws-test', Date.now())
      renderHook(() => useWorkspaceEffects('ws-test'))
      expect(fetchFileTree).toHaveBeenCalledWith('ws-test')
    })

    it('re-seeds on a cold mount that never went hidden', () => {
      // No deactivation stamp — even with matching store data, treat as cold.
      useFileSystemStore.setState({
        rootFolderPath: 'ws-test',
        files: keptTree,
        fileTree: keptTree,
        isFileTreeLoading: false,
      })
      renderHook(() => useWorkspaceEffects('ws-test'))
      expect(fetchFileTree).toHaveBeenCalledWith('ws-test')
    })

    // Pins the `!isFileTreeLoading` clause of the fast-path guard. On the REAL
    // A→B→A path resetWorkspaceScopedStores normalises rootFolderPath back to
    // the returning wsId with an EMPTY tree and isFileTreeLoading:true — so the
    // identity check alone matches and freshness alone passes. Without this
    // clause the fast path would "keep" that empty loading tree and the
    // explorer would render blank on every real A→B→A within the window.
    it('re-seeds when the store holds this wsId but is mid-load (empty reset tree)', () => {
      useFileSystemStore.setState({
        rootFolderPath: 'ws-test',
        files: [],
        fileTree: [],
        isFileTreeLoading: true,
      })
      markWorkspaceDeactivated('ws-test', Date.now())
      renderHook(() => useWorkspaceEffects('ws-test'))
      expect(fetchFileTree).toHaveBeenCalledWith('ws-test')
    })

    // Integration-style: the full A→B→A sequence, driving the REAL activation
    // pieces together in production order — resetWorkspaceScopedStores (the
    // WorkspaceView layout effect) before the seed effects mount — so the test
    // exercises the actual interplay rather than a hand-crafted store shape.
    it('A→B→A within the window refetches A (reset wiped the tree; must not stay empty)', async () => {
      const treeA: AppFile[] = [{ name: 'a.ts', path: 'a.ts', isDir: false }]
      const treeB: AppFile[] = [{ name: 'b.ts', path: 'b.ts', isDir: false }]

      // A activates cold and seeds its tree.
      fetchFileTree.mockResolvedValueOnce(treeA)
      resetWorkspaceScopedStores('ws-test')
      const a = renderHook(() => useWorkspaceEffects('ws-test'))
      await waitFor(() => expect(useFileSystemStore.getState().files).toEqual(treeA))
      a.unmount()
      markWorkspaceDeactivated('ws-test')

      // B activates: reset points the global store at B, B seeds its own tree.
      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-B', owningChatId: 'chat-B' })
      fetchFileTree.mockResolvedValueOnce(treeB)
      resetWorkspaceScopedStores('ws-B')
      const b = renderHook(() => useWorkspaceEffects('ws-B'))
      await waitFor(() => expect(useFileSystemStore.getState().files).toEqual(treeB))
      b.unmount()
      markWorkspaceDeactivated('ws-B')

      // A returns WITHIN the window. Reset normalises the store back to A with
      // an empty loading tree — the fast path must refuse it and refetch.
      setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-test', owningChatId: 'chat-test' })
      fetchFileTree.mockClear()
      fetchFileTree.mockResolvedValueOnce(treeA)
      resetWorkspaceScopedStores('ws-test')
      renderHook(() => useWorkspaceEffects('ws-test'))

      expect(fetchFileTree).toHaveBeenCalledWith('ws-test')
      await waitFor(() => {
        const fs = useFileSystemStore.getState()
        expect(fs.rootFolderPath).toBe('ws-test')
        expect(fs.files).toEqual(treeA)
        expect(fs.isFileTreeLoading).toBe(false)
      })
    })

    it('git fast path skips the 4-request seed but self-heals via the preserved frame', async () => {
      vi.useFakeTimers()
      try {
        const { useGitStore } = await import('@/features/git/stores/git-store')
        const reloadStatusAndLog = vi.fn(() => Promise.resolve())
        const original = useGitStore.getState().actions
        useGitStore.setState({ actions: { ...original, reloadStatusAndLog } })

        const gitHandler = () => {
          const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
          return calls.filter(([ep]) => ep.startsWith(GIT_BASE)).pop()![1]
        }

        // Cold mount: the stream pushes frame F1 → one reload; unmount preserves F1.
        const first = renderHook(() => useWorkspaceEffects('ws-test'))
        expect(fetchAllGitData).toHaveBeenCalledTimes(1)
        gitHandler()({ branch: 'main', files: [] })
        await vi.advanceTimersByTimeAsync(500)
        expect(reloadStatusAndLog).toHaveBeenCalledTimes(1)
        first.unmount()

        // Warm return within the window with git data intact.
        fetchAllGitData.mockClear()
        reloadStatusAndLog.mockClear()
        subscribe.mockClear()
        useGitStore.setState({ currentWorkspaceRepoPath: 'ws-test' })
        markWorkspaceDeactivated('ws-test', Date.now())
        renderHook(() => useWorkspaceEffects('ws-test'))

        // Skipped the four-request git seed on the fast path.
        expect(fetchAllGitData).not.toHaveBeenCalled()

        // The re-subscribed stream re-pushes the SAME frame → no reload (dedup
        // via the preserved frame — no needless git-status-changed diff refetch).
        gitHandler()({ branch: 'main', files: [] })
        await vi.advanceTimersByTimeAsync(500)
        expect(reloadStatusAndLog).not.toHaveBeenCalled()

        // A frame that changed while hidden → reload (self-heal).
        gitHandler()({ branch: 'main', files: [{ path: 'new.ts' }] })
        await vi.advanceTimersByTimeAsync(500)
        expect(reloadStatusAndLog).toHaveBeenCalledTimes(1)

        useGitStore.setState({ actions: original })
      } finally {
        vi.useRealTimers()
      }
    })
  })
})

describe('duplicateDestPath — collision-free copy naming', () => {
  const file = (path: string, isDir = false): AppFile => ({
    path,
    name: path.split('/').pop() ?? path,
    isDir,
  })

  it('inserts " copy" before the extension', () => {
    expect(duplicateDestPath('api/main.go', [])).toBe('api/main copy.go')
  })

  it('appends " copy" to an extension-less name (e.g. a directory)', () => {
    expect(duplicateDestPath('src/utils', [])).toBe('src/utils copy')
  })

  it('treats a leading dot as part of the stem, not an extension', () => {
    expect(duplicateDestPath('.env', [])).toBe('.env copy')
  })

  it('bumps to " copy 2" when the first copy already exists in the tree', () => {
    const tree = [file('api/main.go'), file('api/main copy.go')]
    expect(duplicateDestPath('api/main.go', tree)).toBe('api/main copy 2.go')
  })

  it('keeps bumping past multiple existing copies', () => {
    const tree = [file('main.go'), file('main copy.go'), file('main copy 2.go')]
    expect(duplicateDestPath('main.go', tree)).toBe('main copy 3.go')
  })
})
