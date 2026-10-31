import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, renderHook } from '@testing-library/react'
import type { ChromeButton, NativeChrome, NativeChromeIntent } from '@/lib/native-chrome'

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
import { useConnectionStore } from '@/lib/ws/connection-store'

const describeButtons = (list: ChromeButton[] | undefined) =>
  list?.map((b) => `${b.side}:${b.id}:${b.enabled}`)
const chatButtons = () => describeButtons(chrome.models.at(-1)?.headers?.chat)
const ideButtons = () => describeButtons(chrome.models.at(-1)?.headers?.ide)
const tap = (id: string) =>
  act(() => chrome.listeners.forEach((listener) => listener({ type: 'tap', id })))
const workspaces = { ide: true, shownHost: 'h1' }
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
    root.setProperty('--brand-light', '#606c38')
    root.setProperty('--brand-dark', '#98a86a')
    renderHook(() => useMobileNativeChrome(workspaces))
    expect(chrome.models.at(-1)?.palette).toEqual({
      surface: '#fcfcfa',
      raised: '#0000000a',
      text: '#1b1b19',
      secondaryText: '#484848',
      brandLight: '#606c38',
      brandDark: '#98a86a',
      sidebar: null,
    })
  })

  it('gives the native views behind the page its surface colour', () => {
    document.body.style.backgroundColor = 'rgb(1, 2, 3)'
    renderHook(() => useMobileNativeChrome(workspaces))
    const meta = document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')
    expect(meta?.content).toBe('rgb(1, 2, 3)')
  })

  it('puts the chat list on the left and the IDE on the right of the chat', () => {
    renderHook(() => useMobileNativeChrome({ ide: false, shownHost: 'h1' }))
    expect(chatButtons()).toEqual(['leading:chat-list:true', 'trailing:ide:false'])
  })

  // The IDE is a `</>` (the desktop's open-editor icon), never a terminal.
  it('draws the IDE button with the Code icon', () => {
    renderHook(() => useMobileNativeChrome(workspaces))
    const ide = chrome.models.at(-1)?.headers?.chat.find((b) => b.id === 'ide')
    expect(ide?.icon).toBe('Code')
  })

  it('drives the shell’s own navigation from the buttons', () => {
    renderHook(() => useMobileNativeChrome(workspaces))

    tap('ide')
    expect(useMobileScreenStore.getState().screen).toBe('ide')

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

  // The files drawer is the IDE's: the chrome slides it out over the IDE.
  it('reveals the files drawer over the IDE, from the shell’s own store', () => {
    useMobileScreenStore.setState({ screen: 'ide' })
    renderHook(() => useMobileNativeChrome(workspaces))
    act(() => useMobileScreenStore.getState().setFilesOpen(true))
    expect(reveal()).toBe('files')
  })

  // Each screen owns its header and every push carries both, so the chrome never has to keep
  // the one a screen left behind while it slides away.
  it('hands over the header of each screen, whichever is in front', () => {
    renderHook(() => useMobileNativeChrome(workspaces))
    const expected = {
      chat: ['leading:chat-list:true', 'trailing:ide:true'],
      ide: ['leading:back:true', 'trailing:files:true'],
    }
    for (const move of [
      () => useMobileScreenStore.getState().open('ide'),
      () => useMobileScreenStore.getState().setFilesOpen(true),
      () => useMobileScreenStore.getState().back(),
      () => useMobileScreenStore.getState().back(),
      () => useMobileScreenStore.getState().setChatListOpen(true),
    ]) {
      act(move)
      expect(chatButtons()).toEqual(expected.chat)
      expect(ideButtons()).toEqual(expected.ide)
    }
  })

  // Fast swipes queue intents; the chrome throws away any state older than its last word.
  it('acknowledges the last intent it applied, in the same push as the reveal it caused', async () => {
    renderHook(() => useMobileNativeChrome(workspaces))
    await send({ type: 'openIde', seq: 1 })
    await send({ type: 'closeIde', seq: 2 })
    await send({ type: 'openIde', seq: 3 })
    expect(chrome.models.at(-1)).toMatchObject({ reveal: 'ide', ack: 3 })
    expect(chrome.models.every((m) => m.ack === undefined || m.ack <= 3)).toBe(true)
  })

  it('acknowledges an intent that changes nothing', async () => {
    renderHook(() => useMobileNativeChrome(workspaces))
    await send({ type: 'closeChatList', seq: 7 })
    expect(chrome.models.at(-1)).toMatchObject({ reveal: 'none', ack: 7 })
  })

  it('takes the files drawer swipes as navigation too', async () => {
    useMobileScreenStore.setState({ screen: 'ide' })
    renderHook(() => useMobileNativeChrome(workspaces))
    await send({ type: 'openFiles', seq: 1 })
    expect(chrome.models.at(-1)).toMatchObject({ reveal: 'files', ack: 1 })
    await send({ type: 'closeFiles', seq: 2 })
    expect(chrome.models.at(-1)).toMatchObject({ reveal: 'ide', ack: 2 })
  })

  // The IDE's files button is the web's to handle: the chrome sends its intent as given.
  it('has the IDE’s files button ask for the files', () => {
    useMobileScreenStore.setState({ screen: 'ide' })
    renderHook(() => useMobileNativeChrome(workspaces))
    const files = chrome.models.at(-1)?.headers?.ide.find((button) => button.id === 'files')
    expect(files?.intent).toBe('openFiles')
  })

  // Settings brings its own header: no chat button floats over it.
  it('shows no chat buttons over Settings', () => {
    renderHook(() => useMobileNativeChrome(workspaces))
    act(() => useUIState.setState({ isSettingsOpen: true }))
    expect(chatButtons()).toEqual([])
  })

  // Settings is a native sheet whose pages are the web's own tabs; the store still owns open.
  it('opens the native Settings sheet from the store and closes it on its intent', async () => {
    renderHook(() => useMobileNativeChrome(workspaces))
    expect(chrome.models.at(-1)?.settings?.open).toBe(false)

    act(() => useUIState.setState({ isSettingsOpen: true }))
    const settings = chrome.models.at(-1)?.settings
    expect(settings?.open).toBe(true)

    await send({ type: 'closeSettings' })
    expect(useUIState.getState().isSettingsOpen).toBe(false)
  })

  // The native list's rows, appearance cards and theme list are the web's inventory, not Swift's.
  it('hands the Settings sheet its pages and the registry’s themes', () => {
    useSettingsStore.setState((state) => ({ settings: { ...state.settings, theme: 'crowbar' } }))
    renderHook(() => useMobileNativeChrome(workspaces))
    const settings = chrome.models.at(-1)?.settings
    expect(settings?.theme).toBe('crowbar')
    expect(settings?.themes).toEqual(
      expect.arrayContaining([{ id: 'crowbar', name: expect.any(String) }]),
    )
    expect(settings?.pages.map((page) => page.id)).toEqual([
      'hosts',
      'providers',
      'terminal',
      'git',
      'appearance',
    ])
  })

  it('applies the theme mode and theme the sheet picks', () => {
    renderHook(() => useMobileNativeChrome(workspaces))
    send({ type: 'setThemeMode', mode: 'dark' })
    expect(useSettingsStore.getState().settings.themeMode).toBe('dark')
    send({ type: 'setTheme', id: 'frosted' })
    expect(useSettingsStore.getState().settings.theme).toBe('frosted')
  })

  // The chat list's Hosts button shows where the shown host runs, as the desktop's host icons do.
  it('tells the chrome whether the shown host is this computer or a remote', () => {
    const { rerender } = renderHook(
      ({ shownHost }) => useMobileNativeChrome({ ide: true, shownHost }),
      { initialProps: { shownHost: 'h1' as string | null } },
    )
    expect(chrome.models.at(-1)?.hostPlace).toBe('remote')
    rerender({ shownHost: 'local' })
    expect(chrome.models.at(-1)?.hostPlace).toBe('local')
  })

  // The Hosts button's dot is the shown host's status, from the hook the desktop's icons read.
  it('tells the chrome the shown host\u2019s status', () => {
    useConnectionStore.setState({ status: 'connected' })
    const { rerender } = renderHook(
      ({ shownHost }) => useMobileNativeChrome({ ide: true, shownHost }),
      { initialProps: { shownHost: 'local' as string | null } },
    )
    expect(chrome.models.at(-1)?.hostStatus).toBe('up')
    act(() => useConnectionStore.setState({ status: 'disconnected' }))
    expect(chrome.models.at(-1)?.hostStatus).toBe('down')
    rerender({ shownHost: 'gone' })
    expect(chrome.models.at(-1)?.hostStatus).toBe('unknown')
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
