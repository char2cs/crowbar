import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useSidebarTabKeyboard } from '@/features/keymaps/hooks/use-sidebar-tab-keyboard'

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({
    'navigation.sidebarWorkspaces': 'mod+1',
    'navigation.sidebarFiles': 'mod+2',
    'navigation.sidebarGit': 'mod+3',
  }),
}))

const setActiveTab = vi.fn()
vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: {
    getState: () => ({ setActiveTab }),
  },
}))

function dispatchKeydown(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { cancelable: true, bubbles: true, ...init })
  window.dispatchEvent(event)
  return event
}

describe('useSidebarTabKeyboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('switches to workspaces on Ctrl+1', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '1', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('workspaces')
    expect(event.defaultPrevented).toBe(true)
  })

  it('switches to files on Ctrl+2', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '2', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('files')
    expect(event.defaultPrevented).toBe(true)
  })

  it('switches to git on Ctrl+3', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '3', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('git')
    expect(event.defaultPrevented).toBe(true)
  })

  it('does not fire on plain number key', () => {
    renderHook(() => useSidebarTabKeyboard())
    dispatchKeydown({ key: '1' })
    expect(setActiveTab).not.toHaveBeenCalled()
  })

  it('does not fire on repeat keydown', () => {
    renderHook(() => useSidebarTabKeyboard())
    dispatchKeydown({ key: '1', ctrlKey: true, repeat: true })
    expect(setActiveTab).not.toHaveBeenCalled()
  })

  it('removes the listener on unmount', () => {
    const { unmount } = renderHook(() => useSidebarTabKeyboard())
    unmount()
    dispatchKeydown({ key: '1', ctrlKey: true })
    expect(setActiveTab).not.toHaveBeenCalled()
  })
})
