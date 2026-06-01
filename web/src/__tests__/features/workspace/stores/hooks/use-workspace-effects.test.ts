import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useWorkspaceEffects } from '@/features/workspace/stores/hooks/use-workspace-effects'

const mockBufferActions = {
  openContent: vi.fn(() => 'buf-id'),
  promotePreview: vi.fn(),
}

vi.mock('@/features/workspace/stores/hooks/use-buffer-store', () => ({
  useBufferActions: () => mockBufferActions,
}))
vi.mock('@/features/workspace/stores/workspace-context', () => ({
  useWorkspaceStore: () => ({ getState: () => ({ buffers: [] }) }),
}))

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useWorkspaceEffects', () => {
  it('opens the branchReview buffer on mount (sole default pane)', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(mockBufferActions.openContent).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'branchReview', wsId: 'ws-test' }),
    )
  })

  it('does not open a standalone crowbarChat buffer on mount', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(mockBufferActions.openContent).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: 'crowbarChat' }),
    )
  })
})
