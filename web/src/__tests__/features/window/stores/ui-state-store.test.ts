import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  useUIState,
  _resetTerminalFocusRegistryForTests,
} from '@/features/window/stores/ui-state-store'

describe('ui-state-store focus registry', () => {
  beforeEach(() => {
    // Reset module-level registry singletons between tests
    _resetTerminalFocusRegistryForTests()
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

  it('clearTerminalFocus falls back to the previously registered terminal', () => {
    const fn1 = vi.fn()
    const fn2 = vi.fn()
    useUIState.getState().registerTerminalFocus('term-1', fn1)
    useUIState.getState().registerTerminalFocus('term-2', fn2)
    useUIState.getState().clearTerminalFocus('term-2') // clear the latest
    useUIState.getState().requestTerminalFocus()
    expect(fn1).toHaveBeenCalledOnce() // should fall back to term-1
    expect(fn2).not.toHaveBeenCalled()
  })

  it('setBottomPaneHeight updates bottomPaneHeight', () => {
    useUIState.getState().setBottomPaneHeight(400)
    expect(useUIState.getState().bottomPaneHeight).toBe(400)
  })
})
