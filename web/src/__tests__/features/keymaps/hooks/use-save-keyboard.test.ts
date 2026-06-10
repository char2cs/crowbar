import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useSaveKeyboard } from '@/features/keymaps/hooks/use-save-keyboard'

const { handleSave, handleSaveAll } = vi.hoisted(() => ({
  handleSave: vi.fn().mockResolvedValue(undefined),
  handleSaveAll: vi.fn().mockResolvedValue(0),
}))

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

vi.mock('@/features/editor/stores/editor-app-store', () => ({
  useEditorAppStore: {
    getState: () => ({ actions: { handleSave, handleSaveAll } }),
  },
}))

function dispatchKeydown(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { cancelable: true, bubbles: true, ...init })
  window.dispatchEvent(event)
  return event
}

describe('useSaveKeyboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls handleSave exactly once and prevents default on mod+s', () => {
    renderHook(() => useSaveKeyboard())

    const event = dispatchKeydown({ key: 's', ctrlKey: true })

    expect(handleSave).toHaveBeenCalledTimes(1)
    expect(handleSaveAll).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(true)
  })

  it('calls handleSaveAll and prevents default on mod+shift+s', () => {
    renderHook(() => useSaveKeyboard())

    const event = dispatchKeydown({ key: 'S', ctrlKey: true, shiftKey: true })

    expect(handleSaveAll).toHaveBeenCalledTimes(1)
    expect(handleSave).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(true)
  })

  it("does nothing on plain 's'", () => {
    renderHook(() => useSaveKeyboard())

    const event = dispatchKeydown({ key: 's' })

    expect(handleSave).not.toHaveBeenCalled()
    expect(handleSaveAll).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })

  it('ignores repeated key events', () => {
    renderHook(() => useSaveKeyboard())

    dispatchKeydown({ key: 's', ctrlKey: true, repeat: true })

    expect(handleSave).not.toHaveBeenCalled()
    expect(handleSaveAll).not.toHaveBeenCalled()
  })

  it('does not swallow other mod shortcuts', () => {
    renderHook(() => useSaveKeyboard())

    const event = dispatchKeydown({ key: 'p', ctrlKey: true })

    expect(handleSave).not.toHaveBeenCalled()
    expect(handleSaveAll).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })

  it('removes the listener on unmount', () => {
    const { unmount } = renderHook(() => useSaveKeyboard())
    unmount()

    const event = dispatchKeydown({ key: 's', ctrlKey: true })

    expect(handleSave).not.toHaveBeenCalled()
    expect(event.defaultPrevented).toBe(false)
  })
})
