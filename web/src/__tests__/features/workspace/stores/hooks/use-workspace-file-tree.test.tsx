import type { ReactNode } from 'react'
import { describe, it, expect, beforeEach, onTestFinished, vi } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { useWorkspaceFileTree } from '@/features/workspace/stores/hooks/use-workspace-file-tree'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { setWorkspaceScope } from '@/lib/workspace-scope'
import type { AppFile } from '@/features/file-system/types/app'
import { toastManager } from '@/lib/toast-manager'
import { heldScopes, reconnectStream, streamDaemon } from '@/__tests__/__fixtures__/stream-daemon'
import { ApiError } from '@/lib/errors/api-error'

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
    onTestFinished(reconnectStream)
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
