import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useUIState } from '@/features/window/stores/ui-state-store'

describe('ui-state-store focus registry', () => {
  beforeEach(() => {
    // Clear any terminal registrations from previous tests
    useUIState.getState().clearTerminalFocus('term-1')
    // Reset bottomPaneHeight to default
    useUIState.setState({ bottomPaneHeight: 240 })
  })

  it('registerTerminalFocus + requestTerminalFocus calls the registered fn', () => {
    const focusCalled = vi.fn()
    useUIState.getState().registerTerminalFocus('term-1', focusCalled)
    useUIState.getState().requestTerminalFocus()
    expect(focusCalled).toHaveBeenCalledOnce()
  })

  it('clearTerminalFocus removes the fn so requestTerminalFocus is a no-op', () => {
    const focusCalled = vi.fn()
    useUIState.getState().registerTerminalFocus('term-1', focusCalled)
    useUIState.getState().clearTerminalFocus('term-1')
    useUIState.getState().requestTerminalFocus()
    expect(focusCalled).not.toHaveBeenCalled()
  })

  it('setBottomPaneHeight updates bottomPaneHeight', () => {
    useUIState.getState().setBottomPaneHeight(400)
    expect(useUIState.getState().bottomPaneHeight).toBe(400)
  })
})
