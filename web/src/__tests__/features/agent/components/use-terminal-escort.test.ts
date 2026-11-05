// A hot-swap chat's one process serves both views, so the daemon routes its asks and gates
// compact by the surface the pane last recorded: every move the user sees must be recorded.
import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import * as commandsApi from '@/features/agent/api/agent-chat-commands'
import { useTerminalEscort } from '@/features/agent/components/use-terminal-escort'
import {
  resetChatPresentationMemoryForTests,
  useChatPresentation,
} from '@/features/agent/hooks/use-chat-presentation'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import type { LandingChatPresentation } from '@/features/settings/lib/chat-presentation'
import { useSettingsStore } from '@/features/settings/store'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { provider } from '@/__tests__/__fixtures__/agent-chat'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

const toTerminal = vi.spyOn(commandsApi, 'switchToTerminal')
const toNative = vi.spyOn(commandsApi, 'switchToNative')
const records: string[] = []

interface Props {
  recorded: LandingChatPresentation | null
  waiting?: boolean
}

function renderPane(initial: Props) {
  return renderHook(
    ({ recorded, waiting = false }: Props) => {
      const view = useChatPresentation('c1', { current: null }, null, recorded)
      const escort = useTerminalEscort({
        wsId: 'w1',
        chatId: 'c1',
        providers: [provider('claude', 'Claude')],
        activeProviderId: 'claude',
        onTerminalSurface: false,
        recordedSurface: recorded,
        waiting,
        isActivePane: true,
        isVisible: true,
        presentation: view.presentation,
        setPresentation: view.setPresentation,
        splitting: view.splitting,
        setSplitFocus: view.setSplitFocus,
        setReturnOffered: view.setReturnOffered,
        terminalApiRef: { current: null },
      })
      return { ...view, ...escort }
    },
    { initialProps: initial },
  )
}

const preferring = (landing: LandingChatPresentation) =>
  useSettingsStore.setState({
    settings: {
      ...getDefaultSettingsSnapshot(),
      chatIsDefaultPresentation: landing === 'chat',
      chatSplitPresentationEnabled: true,
    },
  })

beforeEach(() => {
  records.length = 0
  toTerminal.mockReset().mockImplementation(() => {
    records.push('terminal')
    return Promise.resolve('pty-1')
  })
  toNative.mockReset().mockImplementation(() => {
    records.push('chat')
    return Promise.resolve()
  })
  setActiveWorkspaceId('w1')
  preferring('chat')
})

afterEach(() => {
  platform.mobile = false
  resetChatPresentationMemoryForTests()
})

describe('useTerminalEscort — a hot-swap chat records every surface it is shown on', () => {
  it('records each tab switch, terminal to chat to terminal, and split as the chat', () => {
    const { result } = renderPane({ recorded: 'chat' })

    act(() => result.current.chooseSurface('terminal'))
    act(() => result.current.chooseSurface('chat'))
    act(() => result.current.chooseSurface('terminal'))
    act(() => result.current.chooseSurface('split'))

    expect(result.current.presentation).toBe('split')
    // Split shows the chat, so its asks and compact belong there.
    expect(records).toEqual(['terminal', 'chat', 'terminal', 'chat'])
  })

  it('records the terminal the wait escorts the user to, and the chat it brings them back to', () => {
    const { result, rerender } = renderPane({ recorded: 'chat' })

    rerender({ recorded: 'chat', waiting: true })
    expect(result.current.presentation).toBe('terminal')
    rerender({ recorded: 'terminal', waiting: false })

    expect(result.current.presentation).toBe('chat')
    expect(records).toEqual(['terminal', 'chat'])
  })

  it.each(['chat', 'terminal'] as const)(
    'lands a reload on the recorded %s over the preference, recording nothing',
    (recorded) => {
      preferring(recorded === 'chat' ? 'terminal' : 'chat')
      const { result, rerender } = renderPane({ recorded: null })

      rerender({ recorded })

      expect(result.current.presentation).toBe(recorded)
      expect(records).toEqual([])
    },
  )
})

describe('useTerminalEscort — on the phone, which shows only the chat', () => {
  beforeEach(() => {
    platform.mobile = true
  })

  it('brings a chat recorded on its TUI back to the chat once its record lands', () => {
    const { rerender } = renderPane({ recorded: null })
    expect(records).toEqual([])

    rerender({ recorded: 'terminal' })

    expect(records).toEqual(['chat'])
  })

  it('records nothing for a chat already on the chat, nor when another device moves it later', () => {
    const { rerender } = renderPane({ recorded: 'chat' })

    rerender({ recorded: 'terminal' })

    expect(records).toEqual([])
  })
})
