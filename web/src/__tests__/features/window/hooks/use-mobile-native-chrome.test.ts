import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import type { NativeChrome, NativeChromeIntent } from '@/lib/native-chrome'

const chrome = vi.hoisted(() => ({
  models: [] as Partial<NativeChrome>[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
}))
vi.mock('@/lib/native-chrome', () => ({
  setNativeChrome: (model: Partial<NativeChrome>) => chrome.models.push(model),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))

import { useMobileNativeChrome } from '@/features/window/hooks/use-mobile-native-chrome'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useSettingsStore } from '@/features/settings/store'

const buttons = () => chrome.models.at(-1)?.buttons?.map((b) => `${b.side}:${b.id}:${b.enabled}`)
const tap = (id: string) =>
  act(() => chrome.listeners.forEach((listener) => listener({ type: 'tap', id })))
const workspaces = { ide: true }
const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))
const reveal = () => chrome.models.at(-1)?.reveal
const setThemeMode = (themeMode: 'light' | 'dark' | 'system') =>
  useSettingsStore.setState((state) => ({ settings: { ...state.settings, themeMode } }))

beforeEach(() => {
  chrome.models.length = 0
  chrome.listeners.clear()
  useMobileScreenStore.setState({ screen: 'chat', chatListOpen: false, filesOpen: false })
  useUIState.setState({ isSettingsOpen: false })
})

describe('useMobileNativeChrome', () => {
  it('hands the list’s New thread pill the theme’s brand ground', () => {
    document.documentElement.style.setProperty('--brand-ground', '#98a86a')
    renderHook(() => useMobileNativeChrome(workspaces))
    expect(chrome.models.at(-1)?.brandGround).toBe('#98a86a')
  })

  // The native sheets are painted from the same tokens as the Settings page and its cards.
  it('hands the native sheets the theme’s palette', () => {
    const root = document.documentElement.style
    root.setProperty('--background', '#fcfcfa')
    root.setProperty('--muted', 'oklch(0 0 0 / 4%)')
    root.setProperty('--foreground', '#1b1b19')
    root.setProperty('--muted-foreground', 'oklch(0.4 0 0)')
    renderHook(() => useMobileNativeChrome(workspaces))
    expect(chrome.models.at(-1)?.palette).toEqual({
      surface: '#fcfcfa',
      raised: 'oklch(0 0 0 / 4%)',
      text: '#1b1b19',
      secondaryText: 'oklch(0.4 0 0)',
    })
  })

  it('gives the native views behind the page its surface colour', () => {
    document.body.style.backgroundColor = 'rgb(1, 2, 3)'
    renderHook(() => useMobileNativeChrome(workspaces))
    const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
    expect(meta?.content).toBe('rgb(1, 2, 3)')
  })

  it('puts the chat list on the left and the IDE on the right of the chat', () => {
    renderHook(() => useMobileNativeChrome({ ide: false }))
    expect(buttons()).toEqual(['leading:chat-list:true', 'trailing:ide:false'])
  })

  it('drives the shell’s own navigation from the buttons', () => {
    renderHook(() => useMobileNativeChrome(workspaces))

    tap('ide')
    expect(useMobileScreenStore.getState().screen).toBe('ide')
    expect(buttons()).toEqual(['leading:back:true', 'trailing:files:true'])

    tap('back')
    expect(useMobileScreenStore.getState().screen).toBe('chat')

    tap('chat-list')
    expect(useMobileScreenStore.getState().chatListOpen).toBe(true)
  })

  it('tells the chrome which layer to reveal, from the shell’s own store', () => {
    renderHook(() => useMobileNativeChrome(workspaces))
    expect(reveal()).toBe('none')
    act(() => useMobileScreenStore.getState().setChatListOpen(true))
    expect(reveal()).toBe('chatList')
    act(() => useMobileScreenStore.setState({ chatListOpen: false, screen: 'ide' }))
    expect(reveal()).toBe('ide')
  })

  // An edge swipe the user finished is the chrome's report; the store decides what is current.
  it('takes the reveal swipes as navigation', async () => {
    renderHook(() => useMobileNativeChrome(workspaces))

    await send({ type: 'openChatList' })
    expect(useMobileScreenStore.getState().chatListOpen).toBe(true)
    await send({ type: 'closeChatList' })
    expect(useMobileScreenStore.getState().chatListOpen).toBe(false)

    await send({ type: 'openIde' })
    expect(useMobileScreenStore.getState().screen).toBe('ide')
    await send({ type: 'closeIde' })
    expect(useMobileScreenStore.getState().screen).toBe('chat')
  })

  // The files drawer is the IDE's: the chrome slides it out and no buttons float over it.
  it('reveals the files drawer over the IDE, from the shell’s own store', () => {
    useMobileScreenStore.setState({ screen: 'ide' })
    renderHook(() => useMobileNativeChrome(workspaces))
    act(() => useMobileScreenStore.getState().setFilesOpen(true))
    expect(reveal()).toBe('files')
    expect(buttons()).toEqual([])
  })

  // The IDE's files button is the web's to handle: the chrome sends its intent as given.
  it('has the IDE’s files button ask for the files', () => {
    useMobileScreenStore.setState({ screen: 'ide' })
    renderHook(() => useMobileNativeChrome(workspaces))
    const files = chrome.models.at(-1)?.buttons?.find((button) => button.id === 'files')
    expect(files?.intent).toBe('openFiles')
  })

  // The chat list and Settings bring their own headers: no button floats over them.
  it.each([
    ['the chat list', () => useMobileScreenStore.setState({ chatListOpen: true })],
    ['Settings', () => useUIState.setState({ isSettingsOpen: true })],
  ])('shows no buttons over %s', (_over, show) => {
    renderHook(() => useMobileNativeChrome(workspaces))
    act(show)
    expect(buttons()).toEqual([])
  })

  // Settings is a native sheet whose pages are the web's own tabs; the store still owns open.
  it('opens the native Settings sheet from the store and closes it on its intent', async () => {
    renderHook(() => useMobileNativeChrome(workspaces))
    expect(chrome.models.at(-1)?.settings?.open).toBe(false)

    act(() => useUIState.setState({ isSettingsOpen: true }))
    const settings = chrome.models.at(-1)?.settings
    expect(settings).toEqual({ open: true })

    await send({ type: 'closeSettings' })
    expect(useUIState.getState().isSettingsOpen).toBe(false)
  })

  // Native views follow the app's own theme mode, or the system's while the app does.
  it('hands the chrome the app’s theme mode', () => {
    setThemeMode('light')
    renderHook(() => useMobileNativeChrome(workspaces))
    expect(chrome.models.at(-1)?.appearance).toBe('light')
    act(() => setThemeMode('system'))
    expect(chrome.models.at(-1)?.appearance).toBe('system')
  })

  it('takes its buttons down with the shell', () => {
    const { unmount } = renderHook(() => useMobileNativeChrome(workspaces))
    unmount()
    expect(chrome.models.at(-1)).toEqual({})
    expect(chrome.listeners.size).toBe(0)
  })
})
