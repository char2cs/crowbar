import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useSidebarTabKeyboard } from '@/features/keymaps/hooks/use-sidebar-tab-keyboard'

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

// The digit follows the tab's POSITION in the sidebar strip, so Chats — which
// renders second — is mod+2. Mirrors the registry defaults on purpose.
vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({
    'navigation.sidebarWorkspaces': 'mod+1',
    'navigation.sidebarChats': 'mod+2',
    'navigation.sidebarFiles': 'mod+3',
    'navigation.sidebarGit': 'mod+4',
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

  // The Chats tab is gone (Task 8: the unified SidebarTree covers it under
  // 'workspaces' now) but the chord itself is untouched, so Ctrl+2 still
  // lands somewhere real rather than going dead.
  it('switches to workspaces on Ctrl+2 — the old Chats chord, now folded in', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '2', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('workspaces')
    expect(event.defaultPrevented).toBe(true)
  })

  it('switches to files on Ctrl+3', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '3', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('files')
    expect(event.defaultPrevented).toBe(true)
  })

  it('switches to git on Ctrl+4', () => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key: '4', ctrlKey: true })
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
