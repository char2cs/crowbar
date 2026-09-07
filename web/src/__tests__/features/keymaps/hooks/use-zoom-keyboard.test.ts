import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useZoomKeyboard } from '@/features/keymaps/hooks/use-zoom-keyboard'

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({
    'agent.zoomIn': 'mod+=',
    'agent.zoomOut': 'mod+-',
    'agent.zoomReset': 'mod+0',
  }),
}))

const zoomIn = vi.fn()
const zoomOut = vi.fn()
const resetZoom = vi.fn()
vi.mock('@/features/window/stores/zoom-store', () => ({
  useZoomStore: {
    getState: () => ({ actions: { zoomIn, zoomOut, resetZoom } }),
  },
}))

function dispatchKeydown(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { cancelable: true, bubbles: true, ...init })
  window.dispatchEvent(event)
  return event
}

describe('useZoomKeyboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('zooms in on Ctrl+=', () => {
    renderHook(() => useZoomKeyboard())
    const event = dispatchKeydown({ key: '=', ctrlKey: true })
    expect(zoomIn).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)
  })

  it('zooms out on Ctrl+-', () => {
    renderHook(() => useZoomKeyboard())
    const event = dispatchKeydown({ key: '-', ctrlKey: true })
    expect(zoomOut).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)
  })

  it('resets zoom on Ctrl+0', () => {
    renderHook(() => useZoomKeyboard())
    const event = dispatchKeydown({ key: '0', ctrlKey: true })
    expect(resetZoom).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)
  })

  it('does not fire without the modifier', () => {
    renderHook(() => useZoomKeyboard())
    dispatchKeydown({ key: '=' })
    dispatchKeydown({ key: '-' })
    dispatchKeydown({ key: '0' })
    expect(zoomIn).not.toHaveBeenCalled()
    expect(zoomOut).not.toHaveBeenCalled()
    expect(resetZoom).not.toHaveBeenCalled()
  })

  it('removes the listener on unmount', () => {
    const { unmount } = renderHook(() => useZoomKeyboard())
    unmount()
    dispatchKeydown({ key: '=', ctrlKey: true })
    expect(zoomIn).not.toHaveBeenCalled()
  })
})
