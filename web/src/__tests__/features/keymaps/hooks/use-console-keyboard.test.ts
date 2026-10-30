import { describe, it, expect, vi, beforeEach, onTestFinished } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import { useConsoleKeyboard } from '@/features/keymaps/hooks/use-console-keyboard'
import { toastManager } from '@/lib/toast-manager'

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

const { chordMap, setMenuChord, menuListeners, listenFailure } = vi.hoisted(() => ({
  chordMap: { current: { 'navigation.toggleConsole': 'mod+`' } as Record<string, string> },
  setMenuChord: vi.fn().mockResolvedValue(undefined),
  menuListeners: new Set<() => void>(),
  listenFailure: { current: null as Error | null },
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => chordMap.current,
}))

vi.mock('@/lib/crowbar-bridge', () => ({
  setConsoleMenuChord: (chord: string | null) => setMenuChord(chord),
  onConsoleMenuToggle: (handler: () => void) => {
    if (listenFailure.current) return Promise.reject(listenFailure.current)
    menuListeners.add(handler)
    return Promise.resolve(() => menuListeners.delete(handler))
  },
}))

beforeEach(() => {
  chordMap.current = { 'navigation.toggleConsole': 'mod+`' }
  setMenuChord.mockClear()
  menuListeners.clear()
  listenFailure.current = null
})

function dispatchKeydown(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { cancelable: true, bubbles: true, ...init })
  window.dispatchEvent(event)
  return event
}

describe('useConsoleKeyboard', () => {
  it('a failed menu shortcut setup reports why', async () => {
    const toastAdd = vi.spyOn(toastManager, 'add')
    onTestFinished(() => toastAdd.mockRestore())
    setMenuChord.mockRejectedValueOnce(new Error('menu missing'))
    listenFailure.current = new Error('webview gone')

    renderHook(() => useConsoleKeyboard(vi.fn()))

    await vi.waitFor(() => {
      const titles = toastAdd.mock.calls.map(([toast]) => toast.title)
      expect(titles).toEqual(
        expect.arrayContaining(['Set the console shortcut', 'Listen for the console menu']),
      )
    })
  })

  it('toggles and prevents default on the bound chord', () => {
    const onToggle = vi.fn()
    renderHook(() => useConsoleKeyboard(onToggle))

    const event = dispatchKeydown({ key: '`', ctrlKey: true })

    expect(onToggle).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)
  })

  it('ignores other chords and key repeat', () => {
    const onToggle = vi.fn()
    renderHook(() => useConsoleKeyboard(onToggle))

    dispatchKeydown({ key: '`' })
    dispatchKeydown({ key: '`', ctrlKey: true, repeat: true })

    expect(onToggle).not.toHaveBeenCalled()
  })

  it('stops listening on unmount', () => {
    const onToggle = vi.fn()
    const { unmount } = renderHook(() => useConsoleKeyboard(onToggle))
    unmount()

    dispatchKeydown({ key: '`', ctrlKey: true })

    expect(onToggle).not.toHaveBeenCalled()
  })

  it('mirrors the effective chord to the host and clears it when unbound', () => {
    const { rerender } = renderHook(() => useConsoleKeyboard(vi.fn()))
    expect(setMenuChord).toHaveBeenLastCalledWith('mod+`')

    chordMap.current = { 'navigation.toggleConsole': 'mod+j' }
    rerender()
    expect(setMenuChord).toHaveBeenLastCalledWith('mod+j')

    chordMap.current = { 'navigation.toggleConsole': '' }
    rerender()
    expect(setMenuChord).toHaveBeenLastCalledWith(null)
  })

  it('toggles once per host menu event and stops after unmount', async () => {
    const onToggle = vi.fn()
    const { unmount } = renderHook(() => useConsoleKeyboard(onToggle))
    await act(async () => Promise.resolve())

    for (const fire of menuListeners) fire()
    expect(onToggle).toHaveBeenCalledTimes(1)

    unmount()
    expect(menuListeners.size).toBe(0)
  })
})
