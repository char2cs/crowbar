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
})
