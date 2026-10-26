import { useEffect } from 'react'
import {
  onNativeChromeIntent,
  setNativeChrome,
  type ChromeButton,
  type ChromeHeaders,
  type ChromeReveal,
  type NativeChromeIntent,
} from '@/lib/native-chrome'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { useSurfaceThemeColor } from '@/features/window/hooks/use-surface-theme-color'
import { useThemeVersion } from '@/features/editor/theme/use-theme-version'
import { useSettingsStore } from '@/features/settings/store'
import { readNativePalette } from '@/lib/native-palette'

interface Workspaces {
  /** A chat is on screen, with a workspace whose IDE can open. */
  ide: boolean
}

const CHAT_LIST: ChromeButton = {
  id: 'chat-list',
  symbol: 'sidebar.leading',
  side: 'leading',
  label: 'Open chat list',
  enabled: true,
}
const IDE: ChromeButton = {
  id: 'ide',
  symbol: 'chevron.left.forwardslash.chevron.right',
  side: 'trailing',
  label: 'Open IDE',
  enabled: true,
}
const FILES: ChromeButton = {
  id: 'files',
  symbol: 'folder',
  side: 'trailing',
  label: 'Files',
  enabled: true,
  intent: 'openFiles',
}
const BACK: ChromeButton = {
  id: 'back',
  symbol: 'chevron.left',
  side: 'leading',
  label: 'Back',
  enabled: true,
}

function handleTap(id: string) {
  const screens = useMobileScreenStore.getState()
  if (id === 'chat-list') screens.setChatListOpen(true)
  if (id === 'back') screens.back()
  if (id === 'ide') screens.open('ide')
}

// Navigation intents say where to be, not what to toggle, so a repeated one changes nothing.
function handleIntent(intent: NativeChromeIntent) {
  const screens = useMobileScreenStore.getState()
  if (intent.type === 'tap') handleTap(intent.id)
  if (intent.type === 'openChatList') screens.setChatListOpen(true)
  if (intent.type === 'closeChatList') screens.setChatListOpen(false)
  if (intent.type === 'openIde') screens.open('ide')
  if (intent.type === 'closeIde') {
    screens.setFilesOpen(false)
    screens.back()
  }
  if (intent.type === 'openFiles') screens.setFilesOpen(true)
  if (intent.type === 'closeFiles') screens.setFilesOpen(false)
  if (intent.type === 'closeSettings') useUIState.getState().setIsSettingsDialogVisible(false)
  if ('seq' in intent && intent.seq !== undefined) screens.acknowledge(intent.seq)
}

/** The phone shell's native chrome: glass corner buttons and the edge-swipe reveals of the chat
 *  list and the IDE, drawn from the shell's navigation store and driving that same store. */
export function useMobileNativeChrome({ ide }: Workspaces) {
  const screen = useMobileScreenStore((s) => s.screen)
  const listOpen = useMobileScreenStore((s) => s.chatListOpen)
  const filesOpen = useMobileScreenStore((s) => s.filesOpen)
  const ack = useMobileScreenStore((s) => s.ack)
  const settingsOpen = useUIState((s) => s.isSettingsOpen)
  const appearance = useSettingsStore((s) => s.settings.themeMode)
  const theme = useSettingsStore((s) => s.settings.theme)
  // Colours are read from CSS, so a theme change must hand them over again.
  const themeVersion = useThemeVersion()
  // What the keyboard or a shrunk webview uncovers is painted from this, not left black.
  useSurfaceThemeColor()

  useEffect(() => {
    let reveal: ChromeReveal = 'none'
    if (screen === 'ide') reveal = filesOpen ? 'files' : 'ide'
    else if (listOpen) reveal = 'chatList'
    const headers: ChromeHeaders = {
      chat: settingsOpen ? [] : [CHAT_LIST, { ...IDE, enabled: ide }],
      ide: [BACK, FILES],
    }
    const brandGround = getComputedStyle(document.documentElement)
      .getPropertyValue('--brand-ground')
      .trim()
    setNativeChrome({
      headers,
      reveal,
      ack,
      settings: { open: settingsOpen },
      appearance,
      brandGround,
      palette: readNativePalette(),
    })
  }, [screen, listOpen, filesOpen, ack, settingsOpen, ide, appearance, themeVersion, theme])

  useEffect(() => {
    const stop = onNativeChromeIntent(handleIntent)
    return () => {
      stop()
      setNativeChrome({})
    }
  }, [])
}
