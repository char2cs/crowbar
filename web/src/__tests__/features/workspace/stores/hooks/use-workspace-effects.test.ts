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

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useWorkspaceEffects', () => {
  it('opens a crowbarChat buffer on mount', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(mockBufferActions.openContent).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'crowbarChat', wsId: 'ws-test' }),
    )
  })

  it('opens a branchReview buffer on mount', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(mockBufferActions.openContent).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'branchReview', wsId: 'ws-test' }),
    )
  })
})
