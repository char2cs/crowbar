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
  it('does not open a branchReview buffer on mount (feature removed)', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(mockBufferActions.openContent).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: 'branchReview' }),
    )
  })

  it('does not open a standalone crowbarChat buffer on mount', () => {
    renderHook(() => useWorkspaceEffects('ws-test'))
    expect(mockBufferActions.openContent).not.toHaveBeenCalledWith(
      expect.objectContaining({ type: 'crowbarChat' }),
    )
  })
})
