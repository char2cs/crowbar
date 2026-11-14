import type { ReactNode } from 'react'
import { describe, it, expect, beforeEach, afterEach, onTestFinished, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useWorkspaceFileTree } from '@/features/workspace/stores/hooks/use-workspace-file-tree'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { __resetWorkspaceScopesForTest, setWorkspaceScope } from '@/lib/workspace-scope'
import type { AppFile } from '@/features/file-system/types/app'
import { toastManager } from '@/lib/toast-manager'
import { heldScopes, reconnectStream, streamDaemon } from '@/__tests__/__fixtures__/stream-daemon'
import { holding } from '@/core/stream/stream'
import { ApiError } from '@/lib/errors/api-error'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const { fetchFileTree, reconcileBuffers } = vi.hoisted(() => ({
  fetchFileTree: vi.fn(),
  reconcileBuffers: vi.fn(async (_wsId: string) => {}),
}))

vi.mock('@/features/files/lib/file-tree-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/files/lib/file-tree-api')>()
  return { ...actual, fetchFileTree }
})
vi.mock('@/features/workspace/lib/external-buffer-sync', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/workspace/lib/external-buffer-sync')>()),
  reconcileWorkspaceDocuments: reconcileBuffers,
}))

const rootTree: AppFile[] = [
  { name: 'src', path: 'src', isDir: true, children: undefined },
  { name: 'README.md', path: 'README.md', isDir: false },
]

function setup(wsId = 'ws-test', ready = true) {
  const store = createWorkspaceStore(wsId)
  const wrapper = ({ children }: { children: ReactNode }) => (
    <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
  )
  const hook = renderHook(({ active }) => useWorkspaceFileTree(wsId, active, ready), {
    wrapper,
    initialProps: { active: true },
  })
  return { store, ...hook }
}

const held = () => heldScopes('files:')
const fileChange = (data: unknown) =>
  streamDaemon.deliver({ t: 'signal', scope: 'files:ws-test', kind: 'file_change', data })

beforeEach(() => {
  vi.clearAllMocks()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-test', owningChatId: 'chat-test' })
  fetchFileTree.mockResolvedValue(rootTree)
  useFileTreeStore.setState({ expandedPathsByWorkspace: {} })
})

describe('useWorkspaceFileTree', () => {
  it('loads the root tree into the workspace’s own store when first shown', async () => {
    const { store } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    expect(fetchFileTree).toHaveBeenCalledWith('ws-test')
    expect(store.getState().files).toEqual(rootTree)
    // Buffers restored while the workspace was never shown are checked against disk.
    expect(reconcileBuffers).toHaveBeenCalledWith('ws-test')
  })

  it('does nothing while the workspace has never been shown', () => {
    const store = createWorkspaceStore('ws-test')
    renderHook(() => useWorkspaceFileTree('ws-test', false, true), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
      ),
    })
    expect(fetchFileTree).not.toHaveBeenCalled()
    expect(store.getState().fileTreeStatus).toBe('idle')
  })

  it('retries on the next activation after a failed load', async () => {
    fetchFileTree.mockRejectedValueOnce(new ApiError('boom', 500))
    const { store, rerender } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('failed'))

    rerender({ active: false })
    rerender({ active: true })
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    expect(fetchFileTree).toHaveBeenCalledTimes(2)
  })

  it('a failed tree load reports why', async () => {
    const toastAdd = vi.spyOn(toastManager, 'add')
    onTestFinished(() => toastAdd.mockRestore())
    fetchFileTree.mockRejectedValueOnce(new ApiError('daemon unreachable', 500))
    const { store } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('failed'))
    expect(toastAdd).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Load the file tree', description: 'daemon unreachable' }),
    )
  })

  it('a failed check of open files against disk reports why', async () => {
    const toastAdd = vi.spyOn(toastManager, 'add')
    onTestFinished(() => toastAdd.mockRestore())
    reconcileBuffers.mockRejectedValueOnce(new ApiError('read failed', 500))
    setup()
    await waitFor(() =>
      expect(toastAdd).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Check open files against disk',
          description: 'read failed',
        }),
      ),
    )
  })

  it('re-lists after the first read when the snapshot lands during it', async () => {
    let attach: () => void = () => {}
    streamDaemon.answer = ({ op, scope }) => {
      if (op === 'sub') attach = () => streamDaemon.deliver({ t: 'synced', scope })
      return []
    }
    let read: (files: AppFile[]) => void = () => {}
    fetchFileTree.mockReturnValueOnce(new Promise<AppFile[]>((resolve) => (read = resolve)))
    const { store, unmount } = setup()
    await waitFor(() => expect(held()).toEqual(['files:ws-test']))
    expect(fetchFileTree).toHaveBeenCalledTimes(1)

    attach() // a change before the attach may be missing from the read
    fetchFileTree.mockResolvedValueOnce([rootTree[1]])
    read(rootTree)
    await waitFor(() => expect(store.getState().files).toEqual([rootTree[1]]))
    unmount()
    expect(held()).toEqual([])
  })

  it('loads the tree while the stream is down', async () => {
    streamDaemon.drop()
    onTestFinished(() => reconnectStream())
    const { store } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    expect(store.getState().files).toEqual(rootTree)
  })

  it('a refused files: scope fails the tree and says why', async () => {
    const toastAdd = vi.spyOn(toastManager, 'add')
    onTestFinished(() => toastAdd.mockRestore())
    fetchFileTree.mockReturnValueOnce(new Promise<AppFile[]>(() => {}))
    streamDaemon.answer = ({ op, scope }) =>
      op === 'sub'
        ? [
            {
              t: 'closed',
              scope,
              reason: 'refused',
              problem: { code: 'not_found', message: 'no workspace' },
            },
          ]
        : []
    const { store } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('failed'))
    expect(toastAdd).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Follow the file tree', description: 'no workspace' }),
    )
  })

  // A structural change is applied in place and a plain content edit never
  // re-lists a directory.
  it('re-lists the parent directory on a structural change only', async () => {
    const { store } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    fetchFileTree.mockClear()
    const onEvent = fileChange

    onEvent({ type: 'modified', path: 'src/a.ts' })
    expect(fetchFileTree).not.toHaveBeenCalled()

    fetchFileTree.mockResolvedValueOnce([
      ...rootTree,
      { name: 'new.ts', path: 'new.ts', isDir: false },
    ])
    onEvent({ type: 'created', path: 'new.ts' })
    await waitFor(() => expect(store.getState().files).toHaveLength(3))
  })

  // After a reconnect the tree and any open buffers may have missed writes: the
  // resubscribe's synced re-reads both from disk.
  it('re-lists the root and reconciles open buffers when the stream reconnects', async () => {
    const { store } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    fetchFileTree.mockClear()
    reconcileBuffers.mockClear()
    fetchFileTree.mockResolvedValueOnce([rootTree[1]])

    reconnectStream()

    await waitFor(() => expect(store.getState().files).toHaveLength(1))
    expect(reconcileBuffers).toHaveBeenCalledWith('ws-test')
  })

  it('fetches an expanded folder’s children once, and again if a re-list drops them', async () => {
    const srcChildren: AppFile[] = [{ name: 'index.ts', path: 'src/index.ts', isDir: false }]
    useFileTreeStore.setState({ expandedPathsByWorkspace: { 'ws-test': new Set(['src']) } })
    // The snapshot may land during the first read, which then re-lists the root.
    fetchFileTree.mockImplementation(async (_ws: string, path?: string) =>
      path === 'src' ? srcChildren : rootTree,
    )

    const { store } = setup()
    await waitFor(() => expect(fetchFileTree).toHaveBeenCalledWith('ws-test', 'src'))
    await waitFor(() =>
      expect(store.getState().files.find((f) => f.path === 'src')?.children).toEqual(srcChildren),
    )

    fetchFileTree.mockClear()
    store.getState().fileTreeActions.setFiles(rootTree)
    await waitFor(() =>
      expect(store.getState().files.find((f) => f.path === 'src')?.children).toEqual(srcChildren),
    )
    expect(fetchFileTree).toHaveBeenCalledWith('ws-test', 'src')
  })

  it('an expanded folder that no longer exists leaves the expansion quietly', async () => {
    const toastAdd = vi.spyOn(toastManager, 'add')
    onTestFinished(() => toastAdd.mockRestore())
    useFileTreeStore.setState({
      expandedPathsByWorkspace: { 'ws-test': new Set(['src', 'src/gone']) },
    })
    fetchFileTree.mockImplementation(async (_ws: string, path?: string) => {
      if (path === 'src') throw new ApiError('no such directory', 404)
      return rootTree
    })

    setup()
    await waitFor(() =>
      expect(useFileTreeStore.getState().isExpanded('ws-test', 'src')).toBe(false),
    )
    expect(useFileTreeStore.getState().isExpanded('ws-test', 'src/gone')).toBe(false)
    expect(toastAdd).not.toHaveBeenCalled()
  })

  it('waits for the chat scope before fetching an expanded folder’s children', async () => {
    useFileTreeStore.setState({ expandedPathsByWorkspace: { 'ws-test': new Set(['src']) } })
    const store = createWorkspaceStore('ws-test')
    store.getState().fileTreeActions.setFiles(rootTree)
    renderHook(() => useWorkspaceFileTree('ws-test', true, false), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
      ),
    })
    expect(fetchFileTree).not.toHaveBeenCalled()
  })
})

describe('on an install that has paired a remote host', () => {
  beforeEach(() => {
    resetHostWorld()
    pairRemoteHost('host-1')
    __resetWorkspaceScopesForTest()
    setWorkspaceScope({
      projectId: 'p-remote',
      repoId: 'r1',
      wsId: 'ws-remote',
      owningChatId: 'chat-remote',
    })
    setWorkspaceScope({
      projectId: 'p-local',
      repoId: 'r1',
      wsId: 'ws-local',
      owningChatId: 'chat-local',
    })
  })

  afterEach(() => resetHostWorld())

  it('lists nothing and stays idle until the project list says whose workspace it is', async () => {
    const remote = setup('ws-remote')
    const local = setup('ws-local')
    expect(fetchFileTree).not.toHaveBeenCalled()
    expect(held()).toEqual([])
    expect(remote.store.getState().fileTreeStatus).toBe('idle')
    expect(local.store.getState().fileTreeStatus).toBe('idle')

    act(() => loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local')))

    await waitFor(() => expect(remote.store.getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(local.store.getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(held()).toEqual(['files:ws-local']))
    expect(heldScopes('files:', 'host-1')).toEqual(['files:ws-remote'])
  })

  it('does not fetch an expanded folder before the host is known', () => {
    useFileTreeStore.setState({ expandedPathsByWorkspace: { 'ws-remote': new Set(['src']) } })
    const store = createWorkspaceStore('ws-remote')
    store.getState().fileTreeActions.setFiles(rootTree)
    renderHook(() => useWorkspaceFileTree('ws-remote', true, true), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <WorkspaceStoreContext.Provider value={store}>{children}</WorkspaceStoreContext.Provider>
      ),
    })
    expect(fetchFileTree).not.toHaveBeenCalled()
  })

  it('switching local to remote releases the old scope and no signal on it reaches the new tree', async () => {
    loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
    const stores = {
      'ws-local': createWorkspaceStore('ws-local'),
      'ws-remote': createWorkspaceStore('ws-remote'),
    }
    let current: keyof typeof stores = 'ws-local'
    const { rerender } = renderHook(() => useWorkspaceFileTree(current, true, true), {
      wrapper: ({ children }: { children: ReactNode }) => (
        <WorkspaceStoreContext.Provider value={stores[current]}>
          {children}
        </WorkspaceStoreContext.Provider>
      ),
    })
    await waitFor(() => expect(stores['ws-local'].getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(held()).toEqual(['files:ws-local']))

    current = 'ws-remote'
    rerender()
    await waitFor(() => expect(stores['ws-remote'].getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(heldScopes('files:', 'host-1')).toEqual(['files:ws-remote']))
    expect(held()).toEqual([])

    fetchFileTree.mockClear()
    streamDaemon.deliver({
      t: 'signal',
      scope: 'files:ws-local',
      kind: 'file_change',
      data: { type: 'created', path: 'new.ts' },
    })
    expect(fetchFileTree).not.toHaveBeenCalled()
  })

  it('revoking the host releases its scope and lists nothing more', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    const { store } = setup('ws-remote')
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(heldScopes('files:', 'host-1')).toEqual(['files:ws-remote']))
    fetchFileTree.mockClear()

    act(() => useHostConfigStore.getState().removeHost('host-1'))

    expect(holding('host-1', 'files:ws-remote')).toBe(false)
    expect(fetchFileTree).not.toHaveBeenCalled()
  })

  // The scope's synced on re-hold re-reads a tree that may have missed changes in the gap.
  it('re-lists the tree when its host returns, picking up what changed in the gap', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    const { store } = setup('ws-remote')
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    act(() => useHostConfigStore.getState().removeHost('host-1'))
    fetchFileTree.mockClear()
    fetchFileTree.mockResolvedValueOnce([
      ...rootTree,
      { name: 'new.ts', path: 'new.ts', isDir: false },
    ])

    // A re-paired host streams its projects again.
    act(() => {
      pairRemoteHost('host-1')
      loadProjects(projectOn('p-remote', 'host-1'))
    })

    await waitFor(() => expect(store.getState().files).toHaveLength(3))
    expect(fetchFileTree.mock.calls.map(([wsId]) => wsId)).toEqual(['ws-remote'])
  })
})
