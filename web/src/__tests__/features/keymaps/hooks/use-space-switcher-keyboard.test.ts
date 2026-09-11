import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useSpaceSwitcherKeyboard } from '@/features/keymaps/hooks/use-space-switcher-keyboard'

function dispatchKeydown(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { cancelable: true, bubbles: true, ...init })
  window.dispatchEvent(event)
  return event
}

describe('useSpaceSwitcherKeyboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('Cmd+1 selects index 0', () => {
    const onSelect = vi.fn()
    renderHook(() => useSpaceSwitcherKeyboard(3, onSelect))

    const event = dispatchKeydown({ key: '1', metaKey: true })

    expect(onSelect).toHaveBeenCalledExactlyOnceWith(0)
    expect(event.defaultPrevented).toBe(true)
  })

  it('Ctrl+3 selects index 2', () => {
    const onSelect = vi.fn()
    renderHook(() => useSpaceSwitcherKeyboard(3, onSelect))

    dispatchKeydown({ key: '3', ctrlKey: true })

    expect(onSelect).toHaveBeenCalledExactlyOnceWith(2)
  })

  it('9 always jumps to the last space, even with fewer than 9 open', () => {
    const onSelect = vi.fn()
    renderHook(() => useSpaceSwitcherKeyboard(4, onSelect))

    dispatchKeydown({ key: '9', metaKey: true })

    expect(onSelect).toHaveBeenCalledExactlyOnceWith(3)
  })

  it('a position past the open count does nothing — no clamping to the last one', () => {
    const onSelect = vi.fn()
    renderHook(() => useSpaceSwitcherKeyboard(2, onSelect))

    const event = dispatchKeydown({ key: '5', metaKey: true })

    expect(onSelect).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })

  it('does nothing with no modifier key', () => {
    const onSelect = vi.fn()
    renderHook(() => useSpaceSwitcherKeyboard(3, onSelect))

    dispatchKeydown({ key: '1' })

    expect(onSelect).not.toHaveBeenCalled()
  })

  it('does nothing with Shift or Alt held alongside the modifier', () => {
    const onSelect = vi.fn()
    renderHook(() => useSpaceSwitcherKeyboard(3, onSelect))

    dispatchKeydown({ key: '1', metaKey: true, shiftKey: true })
    dispatchKeydown({ key: '1', metaKey: true, altKey: true })

    expect(onSelect).not.toHaveBeenCalled()
  })

  it('does nothing with zero spaces open', () => {
    const onSelect = vi.fn()
    renderHook(() => useSpaceSwitcherKeyboard(0, onSelect))

    dispatchKeydown({ key: '1', metaKey: true })

    expect(onSelect).not.toHaveBeenCalled()
  })

  it('removes the listener on unmount', () => {
    const onSelect = vi.fn()
    const { unmount } = renderHook(() => useSpaceSwitcherKeyboard(3, onSelect))
    unmount()

    dispatchKeydown({ key: '1', metaKey: true })

    expect(onSelect).not.toHaveBeenCalled()
  })
})
