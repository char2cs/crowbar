import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
import { createElement } from 'react'
import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

const { slashCatalogFn, switchToTerminalFn, switchToNativeFn } = vi.hoisted(() => ({
  slashCatalogFn: vi.fn(),
  switchToTerminalFn: vi.fn(),
  switchToNativeFn: vi.fn(),
}))

vi.mock('@/utils/platform', () => ({ IS_MAC: true, IS_MOBILE: false, APPLE_KEYBOARD: true }))

import { chatPane, renderChatPane, stubPaneChrome } from '@/__tests__/__fixtures__/agent-pane'
stubPaneChrome()
import * as readsApi from '@/features/agent/api/agent-chat-reads'
vi.spyOn(readsApi, 'getSlashCatalog').mockImplementation(slashCatalogFn)
import * as commandsApi from '@/features/agent/api/agent-chat-commands'
vi.spyOn(commandsApi, 'switchProvider').mockImplementation(vi.fn())
vi.spyOn(commandsApi, 'resumeChat').mockImplementation(vi.fn())
vi.spyOn(commandsApi, 'submitAgentPrompt').mockImplementation(vi.fn())
vi.spyOn(commandsApi, 'switchToTerminal').mockImplementation(switchToTerminalFn)
vi.spyOn(commandsApi, 'switchToNative').mockImplementation(switchToNativeFn)

import * as lazyTerminalMod from '@/features/terminal/components/lazy-terminal'
vi.spyOn(lazyTerminalMod, 'LazyXtermTerminal').mockImplementation(
  ({ sessionId, isVisible }: { sessionId: string; isVisible?: boolean }) =>
    createElement('div', {
      'data-testid': 'xterm',
      'data-session-id': sessionId,
      'data-visible': String(isVisible),
    }),
)

import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { chatStore } from '@/features/agent/stores/chat-store'
import {
  chatSnapshot,
  provider,
  resetChatStore,
  seedChats,
  setChatTerminalWait,
  setProviders,
  writeChat,
} from '@/__tests__/__fixtures__/agent-chat'

const providers = [provider('claude', 'Claude')]

function liveChat(wait?: { kind: string }): AgentChat {
  return chatSnapshot({
    id: 'c1',
    title: 'Chat c1',
    liveRunnerId: 'r1',
    terminalSessionId: 'pty-1',
    activeProviderId: 'claude',
    phase: 'dormant',
    terminalWait: wait,
  })
}

function seedOne(list: AgentProvider[], chat: AgentChat) {
  setProviders(list)
  seedChats([chat])
  return createWorkspaceStore('w1')
}
const seed = (wait?: { kind: string }) => seedOne(providers, liveChat(wait))

const nonHotswapProviders = [provider('codex', 'Codex', { hotswap: false })]

const nonHotswapChat = (wait?: { kind: string }): AgentChat => ({
  ...liveChat(wait),
  terminalSessionId: '',
  activeProviderId: 'codex',
})

const seedNonHotswap = (wait?: { kind: string }) =>
  seedOne(nonHotswapProviders, nonHotswapChat(wait))

const renderPane = renderChatPane

async function renderLeft(store: ReturnType<typeof seed>) {
  const view = await renderPane(store)
  await act(async () => view.rerender(chatPane(store, { isVisible: false })))
}

async function setWait(wait: { kind: string } | null) {
  await act(async () => {
    setChatTerminalWait('c1', wait)
  })
}

const showing = () =>
  (screen.getByTestId('agent-chat-surface').className.includes('hidden') ? 'terminal' : 'chat') as
    'chat' | 'terminal'

const surfaceToggle = () => screen.getByRole('tab', { name: 'Terminal' })

beforeEach(() => {
  resetChatStore()
  useSettingsStore.setState({
    settings: { ...getDefaultSettingsSnapshot(), chatIsDefaultPresentation: true },
  })
  slashCatalogFn.mockReset()
  switchToTerminalFn.mockReset()
  switchToTerminalFn.mockImplementation(async () => {
    writeChat({ id: 'c1', surface: 'terminal', terminalSessionId: 'term-session-1' })
    return 'term-session-1'
  })
  switchToNativeFn.mockReset()
  switchToNativeFn.mockImplementation(async () => writeChat({ id: 'c1', surface: undefined }))
  slashCatalogFn.mockResolvedValue({
    providerId: 'claude',
    completeness: 'model_visible',
    items: [],
    warnings: [],
  })
  useTerminalStore.setState({ sessions: new Map() })
  localStorage.clear()
  setActiveWorkspaceId('w1')
})

describe('AgentChatPane — waiting in the terminal', () => {
  it('says what the agent is waiting on and leaves the surface to the user', async () => {
    const store = seed()
    await renderPane(store)
    expect(showing()).toBe('chat')

    await setWait({ kind: 'workspace_trust' })

    expect(screen.getByTestId('agent-terminal-wait')).toHaveTextContent(
      'Claude is asking whether you trust this folder',
    )
    expect(screen.getByRole('button', { name: 'Open Terminal' })).toBeInTheDocument()
    expect(showing()).toBe('chat')
  })

  it('reports an unidentified prompt without naming it', async () => {
    const store = seed()
    await renderPane(store)

    await setWait({ kind: '' })

    expect(screen.getByTestId('agent-terminal-wait')).toHaveTextContent(
      'Claude is waiting for input in its terminal',
    )
  })

  it('keeps a chat that is already waiting on the chat, and there once the wait clears', async () => {
    const store = seed({ kind: 'workspace_trust' })
    await renderPane(store)
    expect(showing()).toBe('chat')

    await setWait(null)

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  it('opens a hidden tab’s chat only once it is shown, and keeps it after', async () => {
    const store = seed()
    const view = await renderPane(store, { isVisible: false })
    expect(chatStore.getState().feeds.c1).toBeUndefined()
    await act(async () => view.rerender(chatPane(store)))
    expect(chatStore.getState().feeds.c1).toBeDefined()
    await act(async () => view.rerender(chatPane(store, { isVisible: false })))
    expect(chatStore.getState().feeds.c1).toBeDefined()
  })

  it('takes the user back when they accept the offer', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })
    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    await setWait(null)

    fireEvent.click(screen.getByRole('button', { name: 'Return to Chat' }))

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  it('stops offering when the user dismisses it', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })
    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    await setWait(null)

    fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }))

    expect(showing()).toBe('terminal')
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  it('never moves a user it did not move itself', async () => {
    const store = seed()
    await renderPane(store)
    fireEvent.click(surfaceToggle())
    expect(showing()).toBe('terminal')

    await setWait({ kind: 'workspace_trust' })
    await setWait(null)

    expect(showing()).toBe('terminal')
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  it('offers the way back to a user who followed the banner', async () => {
    const store = seed()
    await renderLeft(store)
    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('chat')

    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    expect(showing()).toBe('terminal')
    await setWait(null)

    expect(showing()).toBe('terminal')
    expect(screen.getByTestId('agent-return-to-chat')).toBeInTheDocument()
  })

  it('renders the wait banner inside AgentEmptyDocument, not as a free-floating row above it', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })

    const banner = screen.getByTestId('agent-terminal-wait')
    const handle = banner.closest('.dochandle')
    expect(handle).not.toBeNull()
    expect(banner.closest('.banner')).not.toBeNull()
    expect(handle?.closest('.docwrap')).not.toBeNull()
  })

  it('clears the overlay header row when one floats above this pane', async () => {
    const store = seed()
    await renderPane(store, { belowOverlayHeader: true })
    await setWait({ kind: 'workspace_trust' })

    screen.getByTestId('agent-terminal-wait')
    const section = document.querySelector('.agent-chat.chat') as HTMLElement
    expect(section.style.getPropertyValue('--agent-header-clearance')).toBe('50px')
  })

  it('keeps its original offset with no overlay header above (default)', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })

    screen.getByTestId('agent-terminal-wait')
    const section = document.querySelector('.agent-chat.chat') as HTMLElement
    expect(section.style.getPropertyValue('--agent-header-clearance')).toBe('8px')
  })

  it('keeps the chat surface hidden after Open Terminal, even while still waiting', async () => {
    const store = seed()
    await renderLeft(store)
    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('chat')

    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    expect(showing()).toBe('terminal')

    const chatSurface = screen.getByTestId('agent-chat-surface')
    expect(chatSurface.className).toMatch(/\bhidden\b/)
    expect(chatSurface.className).not.toMatch(/\bflex\b/)
  })

  it('does nothing at all to a chat that is not waiting', async () => {
    const store = seed()

    await renderPane(store)

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-terminal-wait')).not.toBeInTheDocument()
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })
})

describe('AgentChatPane — waiting in the terminal (non-hotswap provider)', () => {
  it("calls switchToTerminal when the wait banner's Open Terminal button is used", async () => {
    const store = seedNonHotswap()
    await renderLeft(store)
    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('chat')

    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))

    await waitFor(() => expect(showing()).toBe('terminal'))
    expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1')
  })

  it('hands the chat back to its api channel when the user returns after the prompt', async () => {
    const store = seedNonHotswap()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })
    expect(switchToTerminalFn).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    await waitFor(() => expect(showing()).toBe('terminal'))

    await setWait(null)
    fireEvent.click(screen.getByRole('button', { name: 'Return to Chat' }))

    await waitFor(() => expect(showing()).toBe('chat'))
    expect(switchToNativeFn).toHaveBeenCalledWith('w1', 'c1')
  })

  it('leaves the user on chat when switchToTerminal is refused', async () => {
    switchToTerminalFn.mockRejectedValue(new Error('agent: provider cannot hand a live turn'))
    const store = seedNonHotswap()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })

    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    await waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1'))

    expect(showing()).toBe('chat')
  })
})

describe('AgentChatPane — a non-hotswap chat after a reload', () => {
  const landOn = (presentation: 'chat' | 'terminal') =>
    useSettingsStore.setState({
      settings: {
        ...getDefaultSettingsSnapshot(),
        chatIsDefaultPresentation: presentation === 'chat',
      },
    })

  it('opens on the chat its daemon holds, even where new chats land on the terminal', async () => {
    landOn('terminal')
    const store = seedNonHotswap()
    await renderPane(store)

    expect(showing()).toBe('chat')
    expect(switchToTerminalFn).not.toHaveBeenCalled()
    expect(switchToNativeFn).not.toHaveBeenCalled()
  })

  it('opens on the terminal view its daemon holds, even where new chats land on the chat', async () => {
    landOn('chat')
    const store = seedOne(nonHotswapProviders, {
      ...nonHotswapChat(),
      terminalSessionId: 'pty-1',
      surface: 'terminal',
    })
    await renderPane(store)

    expect(showing()).toBe('terminal')
    expect(switchToTerminalFn).not.toHaveBeenCalled()
    expect(switchToNativeFn).not.toHaveBeenCalled()
  })
})
