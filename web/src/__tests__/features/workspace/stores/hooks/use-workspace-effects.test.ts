import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useWorkspaceEffects } from '@/features/workspace/stores/hooks/use-workspace-effects'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useUIState, _resetTerminalFocusRegistryForTests } from '@/features/window/stores/ui-state-store'

const mockBufferActions = {
  openContent: vi.fn(() => 'buf-id'),
  promotePreview: vi.fn(),
}
const mockWorkflowActions = {
  setFlowDefinition: vi.fn(),
  setCurrentStep: vi.fn(),
}

const mockGetPane = vi.fn()
const mockPaneActions = {
  getPane: mockGetPane,
  addBufferToPane: vi.fn(),
}
const mockBuffers: unknown[] = []

const mockStoreState = {
  bufferActions: mockBufferActions,
  paneActions: mockPaneActions,
  buffers: mockBuffers,
}

const mockStore = {
  getState: vi.fn(() => mockStoreState),
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
vi.mock('@/lib/mock/files', () => ({
  getMockFileTree: () => [],
  getMockFileContent: (_path: string) => '// file content',
}))
vi.mock('@/features/workspace/stores/workspace-store-ref', () => ({
  getActiveWorkspaceStoreRef: vi.fn(() => mockStore),
}))

beforeEach(() => {
  vi.clearAllMocks()
  _resetTerminalFocusRegistryForTests()
  // Reset isBottomPaneVisible to false before each test
  useUIState.setState({ isBottomPaneVisible: false })
  // Default: bottom pane has no terminal buffers
  mockGetPane.mockReturnValue({ id: 'bottom-pane', bufferIds: [], activeBufferId: null })
})

afterEach(() => {
  vi.useRealTimers()
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

  describe('terminal-ensure-session event', () => {
    it('sets isBottomPaneVisible to true when terminal-ensure-session is dispatched', () => {
      renderHook(() => useWorkspaceEffects('ws-test'))

      act(() => {
        window.dispatchEvent(new CustomEvent('terminal-ensure-session'))
      })

      expect(useUIState.getState().isBottomPaneVisible).toBe(true)
    })

    it('opens a terminal buffer in the bottom pane when none exists', () => {
      mockGetPane.mockReturnValue({ id: 'bottom-pane', bufferIds: [], activeBufferId: null })
      mockBufferActions.openContent.mockReturnValue('new-terminal-buf')

      renderHook(() => useWorkspaceEffects('ws-test'))

      act(() => {
        window.dispatchEvent(new CustomEvent('terminal-ensure-session'))
      })

      expect(mockBufferActions.openContent).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'terminal' }),
      )
      expect(mockPaneActions.addBufferToPane).toHaveBeenCalledWith(
        'bottom-pane',
        'new-terminal-buf',
        true,
      )
    })

    it('does not open a new terminal if one already exists in the bottom pane', () => {
      const existingBufId = 'existing-terminal-buf'
      mockBuffers.length = 0
      mockBuffers.push({ id: existingBufId, type: 'terminal' })
      mockGetPane.mockReturnValue({
        id: 'bottom-pane',
        bufferIds: [existingBufId],
        activeBufferId: existingBufId,
      })

      renderHook(() => useWorkspaceEffects('ws-test'))

      act(() => {
        window.dispatchEvent(new CustomEvent('terminal-ensure-session'))
      })

      // openContent should NOT have been called for a terminal
      expect(mockBufferActions.openContent).not.toHaveBeenCalledWith(
        expect.objectContaining({ type: 'terminal' }),
      )
    })

    it('removes the event listener on unmount', () => {
      const { unmount } = renderHook(() => useWorkspaceEffects('ws-test'))
      unmount()

      act(() => {
        window.dispatchEvent(new CustomEvent('terminal-ensure-session'))
      })

      // After unmount, isBottomPaneVisible should remain false
      expect(useUIState.getState().isBottomPaneVisible).toBe(false)
    })
  })
})
