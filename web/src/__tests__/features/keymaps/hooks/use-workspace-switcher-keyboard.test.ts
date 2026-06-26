import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useWorkspaceSwitcherKeyboard } from '@/features/keymaps/hooks/use-workspace-switcher-keyboard'

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({
    'navigation.openWorkspaceSwitcher': 'mod+.',
  }),
}))

function dispatchKeydown(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { cancelable: true, bubbles: true, ...init })
  window.dispatchEvent(event)
  return event
}

describe('useWorkspaceSwitcherKeyboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls onOpen and prevents default on Ctrl+.', () => {
    const onOpen = vi.fn()
    renderHook(() => useWorkspaceSwitcherKeyboard(onOpen))

    const event = dispatchKeydown({ key: '.', ctrlKey: true })

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)
  })

  it('does not call onOpen on plain dot', () => {
    const onOpen = vi.fn()
    renderHook(() => useWorkspaceSwitcherKeyboard(onOpen))

    dispatchKeydown({ key: '.' })

    expect(onOpen).not.toHaveBeenCalled()
  })

  it('does not call onOpen on a different mod chord', () => {
    const onOpen = vi.fn()
    renderHook(() => useWorkspaceSwitcherKeyboard(onOpen))

    dispatchKeydown({ key: ',', ctrlKey: true })

    expect(onOpen).not.toHaveBeenCalled()
  })

  it('removes the listener on unmount', () => {
    const onOpen = vi.fn()
    const { unmount } = renderHook(() => useWorkspaceSwitcherKeyboard(onOpen))
    unmount()

    dispatchKeydown({ key: '.', ctrlKey: true })

    expect(onOpen).not.toHaveBeenCalled()
  })
})
