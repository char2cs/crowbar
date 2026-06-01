import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useWorkspaceEffects } from '@/features/workspace/stores/hooks/use-workspace-effects'
import { useFileSystemStore } from '@/features/file-system/controllers/store'

const mockBufferActions = {
  openContent: vi.fn(() => 'buf-id'),
  promotePreview: vi.fn(),
}
const mockWorkflowActions = {
  setFlowDefinition: vi.fn(),
  setCurrentStep: vi.fn(),
}

vi.mock('@/features/workspace/stores/hooks/use-buffer-store', () => ({
  useBufferActions: () => mockBufferActions,
}))
vi.mock('@/features/workspace/stores/hooks/use-pane-store', () => ({
  usePaneActions: () => ({ getActivePane: vi.fn(() => null) }),
}))
vi.mock('@/features/workspace/stores/hooks/use-workflow', () => ({
  useWorkflowActions: () => mockWorkflowActions,
}))
vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStore: () => ({ getState: () => ({ buffers: [] }) }),
}))
vi.mock('@/lib/mock/files', () => ({
  getMockFileTree: () => [],
  getMockFileContent: (_path: string) => '// file content',
}))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useWorkspaceEffects', () => {
  describe('handleFileSelect', () => {
    it('opens the file as a preview buffer (isPreview=true)', () => {
      renderHook(() => useWorkspaceEffects('ws-test'))

      const { handleFileSelect } = useFileSystemStore.getState()
      handleFileSelect?.('/src/foo.ts')

      expect(mockBufferActions.openContent).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'editor',
          path: '/src/foo.ts',
          name: 'foo.ts',
          isPreview: true,
        }),
      )
    })

    it('does nothing for directories', () => {
      renderHook(() => useWorkspaceEffects('ws-test'))
      const { handleFileSelect } = useFileSystemStore.getState()
      handleFileSelect?.('/src', true)
      expect(mockBufferActions.openContent).not.toHaveBeenCalledWith(
        expect.objectContaining({ path: '/src' }),
      )
    })
  })
})
