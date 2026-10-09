import type { ReactNode } from 'react'
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act, renderHook, waitFor } from '@testing-library/react'
import { useWorkspaceFileTree } from '@/features/workspace/stores/hooks/use-workspace-file-tree'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { __resetWorkspaceScopesForTest, setWorkspaceScope } from '@/lib/workspace-scope'
import type { AppFile } from '@/features/file-system/types/app'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const { fetchFileTree, subscribe, reconcileBuffers } = vi.hoisted(() => ({
  fetchFileTree: vi.fn(),
  subscribe: vi.fn(() => () => {}),
  reconcileBuffers: vi.fn(async (_wsId: string) => {}),
}))

vi.mock('@/features/files/lib/file-tree-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/files/lib/file-tree-api')>()
  return { ...actual, fetchFileTree }
})
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe, send: vi.fn() } }))
vi.mock('@/lib/persistence/hydrate', () => ({
  reconcileWorkspaceBuffersWithDisk: reconcileBuffers,
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

const filesHandler = () => {
  const calls = subscribe.mock.calls as unknown as [string, (evt: unknown) => void][]
  return calls.find(([ep]) => ep.endsWith('/files/ws'))![1]
}

beforeEach(() => {
  vi.clearAllMocks()
  setWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-test', owningChatId: 'chat-test' })
  fetchFileTree.mockResolvedValue(rootTree)
  useFileTreeStore.setState({ expandedPathsByWorkspace: {} })
})

describe('useWorkspaceFileTree', () => {
  it('loads the root tree into the workspace’s own store when first shown', async () => {
    const { store } = setup()
    expect(store.getState().fileTreeStatus).toBe('loading')
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
    fetchFileTree.mockRejectedValueOnce(new Error('boom'))
    const { store, rerender } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('failed'))

    rerender({ active: false })
    rerender({ active: true })
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    expect(fetchFileTree).toHaveBeenCalledTimes(2)
  })

  // The files stream carries no snapshot on subscribe: a structural change is
  // applied in place and a plain content edit never re-lists a directory.
  it('re-lists the parent directory on a structural change only', async () => {
    const { store } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(subscribe).toHaveBeenCalled())
    fetchFileTree.mockClear()
    const onEvent = filesHandler()

    onEvent({ type: 'modified', path: 'src/a.ts' })
    expect(fetchFileTree).not.toHaveBeenCalled()

    fetchFileTree.mockResolvedValueOnce([
      ...rootTree,
      { name: 'new.ts', path: 'new.ts', isDir: false },
    ])
    onEvent({ type: 'created', path: 'new.ts' })
    await waitFor(() => expect(store.getState().files).toHaveLength(3))
  })

  // The stream has no snapshot on subscribe, so after a reconnect the tree and
  // any open buffers may have missed writes: both are re-read from disk.
  it('re-lists the root and reconciles open buffers when the stream reconnects', async () => {
    const { store } = setup()
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(subscribe).toHaveBeenCalled())
    fetchFileTree.mockClear()
    fetchFileTree.mockResolvedValueOnce([rootTree[1]])

    filesHandler()({ reconnected: true })

    await waitFor(() => expect(store.getState().files).toHaveLength(1))
    expect(reconcileBuffers).toHaveBeenCalledWith('ws-test')
  })

  it('fetches an expanded folder’s children once, and again if a re-list drops them', async () => {
    const srcChildren: AppFile[] = [{ name: 'index.ts', path: 'src/index.ts', isDir: false }]
    useFileTreeStore.setState({ expandedPathsByWorkspace: { 'ws-test': new Set(['src']) } })
    fetchFileTree.mockResolvedValueOnce(rootTree).mockResolvedValueOnce(srcChildren)

    const { store } = setup()
    await waitFor(() => expect(fetchFileTree).toHaveBeenCalledWith('ws-test', 'src'))
    await waitFor(() =>
      expect(store.getState().files.find((f) => f.path === 'src')?.children).toEqual(srcChildren),
    )

    fetchFileTree.mockResolvedValueOnce(srcChildren)
    store.getState().fileTreeActions.setFiles(rootTree)
    await waitFor(() =>
      expect(store.getState().files.find((f) => f.path === 'src')?.children).toEqual(srcChildren),
    )
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
  const REMOTE_FILES = 'crowbar://host-1/v0/chats/chat-remote/files/ws'
  const LOCAL_FILES = '/v0/chats/chat-local/files/ws'
  const filesEndpoints = () =>
    (subscribe.mock.calls as unknown as [string][]).map(([endpoint]) => endpoint)

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
    expect(remote.store.getState().fileTreeStatus).toBe('idle')
    expect(local.store.getState().fileTreeStatus).toBe('idle')

    act(() => loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local')))

    await waitFor(() => expect(remote.store.getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(local.store.getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(filesEndpoints().sort()).toEqual([LOCAL_FILES, REMOTE_FILES].sort()))
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

  it('switching local to remote closes the old channel and no event from it reaches the new tree', async () => {
    loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
    const unsubLocal = vi.fn()
    subscribe.mockReturnValueOnce(unsubLocal)
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
    await waitFor(() => expect(subscribe).toHaveBeenCalledTimes(1))
    const calls = () => subscribe.mock.calls as unknown as [string, (evt: unknown) => void][]
    const fromLocalChannel = calls()[0][1]

    current = 'ws-remote'
    rerender()
    await waitFor(() => expect(stores['ws-remote'].getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(subscribe).toHaveBeenCalledTimes(2))

    expect(unsubLocal).toHaveBeenCalledTimes(1)
    expect(filesEndpoints()).toEqual([LOCAL_FILES, REMOTE_FILES])
    fetchFileTree.mockClear()
    reconcileBuffers.mockClear()
    fromLocalChannel({ reconnected: true })
    expect(reconcileBuffers).not.toHaveBeenCalledWith('ws-remote')
  })

  it('revoking the host closes its channel and never reopens it against the local daemon', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    const unsubscribe = vi.fn()
    subscribe.mockReturnValueOnce(unsubscribe)
    const { store } = setup('ws-remote')
    await waitFor(() => expect(store.getState().fileTreeStatus).toBe('ready'))
    await waitFor(() => expect(subscribe).toHaveBeenCalledTimes(1))
    fetchFileTree.mockClear()

    act(() => useHostConfigStore.getState().removeHost('host-1'))

    expect(unsubscribe).toHaveBeenCalledTimes(1)
    expect(subscribe).toHaveBeenCalledTimes(1)
    expect(fetchFileTree).not.toHaveBeenCalled()
  })

  it('does not let a refused channel escape the effect', async () => {
    loadProjects(projectOn('p-remote', 'host-1'))
    subscribe.mockImplementationOnce(() => {
      throw new Error('remote hosts need the desktop app')
    })

    expect(() => setup('ws-remote')).not.toThrow()
    await waitFor(() => expect(fetchFileTree).toHaveBeenCalled())
  })
})
