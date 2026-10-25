import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useSidebarTabKeyboard } from '@/features/keymaps/hooks/use-sidebar-tab-keyboard'

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

// Mirrors the registry defaults on purpose: the digit is the mode's position
// in the mode bar (Chats, Files, Git).
vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({
    'navigation.sidebarChats': 'mod+1',
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

const focused = vi.hoisted(() => ({
  ctx: { workspaceId: 'w1', repoId: 'r1' } as { workspaceId: string | null; repoId: string | null },
}))
vi.mock('@/features/window/stores/focused-workspace-context-store', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@/features/window/stores/focused-workspace-context-store')
  >()),
  getFocusedWorkspaceContext: () => focused.ctx,
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

  beforeEach(() => {
    focused.ctx = { workspaceId: 'w1', repoId: 'r1' }
  })

  it.each([
    ['1', 'chats'],
    ['2', 'files'],
    ['3', 'git'],
  ])('switches mode on Ctrl+%s to %s', (key, tab) => {
    renderHook(() => useSidebarTabKeyboard())
    const event = dispatchKeydown({ key, ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith(tab)
    expect(event.defaultPrevented).toBe(true)
  })

  it('has no fourth mode: Ctrl+4 does nothing', () => {
    renderHook(() => useSidebarTabKeyboard())
    dispatchKeydown({ key: '4', ctrlKey: true })
    expect(setActiveTab).not.toHaveBeenCalled()
  })

  it('ignores a mode the focused workspace does not offer', () => {
    focused.ctx = { workspaceId: null, repoId: null }
    renderHook(() => useSidebarTabKeyboard())
    const files = dispatchKeydown({ key: '2', ctrlKey: true })
    dispatchKeydown({ key: '3', ctrlKey: true })
    expect(setActiveTab).not.toHaveBeenCalled()
    expect(files.defaultPrevented).toBe(false)
    dispatchKeydown({ key: '1', ctrlKey: true })
    expect(setActiveTab).toHaveBeenCalledWith('chats')
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
