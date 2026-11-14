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

// The header clearance below is a macOS value; jsdom's UA is the runner's own platform.
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

// jsdom cannot run xterm/WebGL. The marker records the visibility props the pane
// threads down, which is how "the terminal surface is the selected one" is proven
// without reading a class name.
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
import { resetChatPresentationMemoryForTests } from '@/features/agent/hooks/use-chat-presentation'
import {
  chatSnapshot,
  provider,
  resetChatStore,
  seedChats,
  setChatTerminalWait,
  setProviders,
  writeChat,
} from '@/__tests__/__fixtures__/agent-chat'

// Hotswap keeps a chat/terminal move synchronous; without it the move goes
// through switchToTerminal first.
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

// A non-hotswap provider has no live terminal until switchToTerminal succeeds.
const nonHotswapProviders = [provider('codex', 'Codex', { hotswap: false })]

const nonHotswapChat = (wait?: { kind: string }): AgentChat => ({
  ...liveChat(wait),
  terminalSessionId: '',
  activeProviderId: 'codex',
})

const seedNonHotswap = (wait?: { kind: string }) =>
  seedOne(nonHotswapProviders, nonHotswapChat(wait))

const renderPane = renderChatPane

/** A tab the user looked at, then left: its chat is open, its pane hidden. */
async function renderLeft(store: ReturnType<typeof seed>) {
  const view = await renderPane(store)
  await act(async () => view.rerender(chatPane(store, { isVisible: false })))
}

/** Raise or clear the daemon's verdict the way the WS frame does. */
async function setWait(wait: { kind: string } | null) {
  await act(async () => {
    setChatTerminalWait('c1', wait)
  })
}

/** Which surface is showing: the pane hides one via `hidden` on the chat surface;
 *  the blank chat's switcher is not on screen while a signpost holds its slot. */
const showing = () =>
  (screen.getByTestId('agent-chat-surface').className.includes('hidden') ? 'terminal' : 'chat') as
    'chat' | 'terminal'

/** The Terminal half of the surface switcher. Named EXACTLY, because the banner's
 *  own call to action is "Open Terminal" and a loose match finds both. */
const surfaceToggle = () => screen.getByRole('tab', { name: 'Terminal' })

beforeEach(() => {
  resetChatPresentationMemoryForTests()
  resetChatStore()
  // The wait banner is about a chat that is on the Chat surface.
  useSettingsStore.setState({
    settings: { ...getDefaultSettingsSnapshot(), chatIsDefaultPresentation: true },
  })
  slashCatalogFn.mockReset()
  switchToTerminalFn.mockReset()
  // Each switch's answer is the daemon's frame moving the chat's surface.
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
  // ── THE HEADLINE ───────────────────────────────────────────────────
  // The CLI is parked on a dialog no hook reports: the chat SAYS what is happening
  // and offers the way to the terminal, but the surface is the person's to change.
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

  // ── Don't yank the surface ─────────────────────────────────────────

  // A new chat's CLI often opens on a dialog (trust, a startup notice) before it
  // has done anything: opening on it, or it clearing, moves nobody.
  it('keeps a chat that is already waiting on the chat, and there once the wait clears', async () => {
    const store = seed({ kind: 'workspace_trust' })
    await renderPane(store)
    expect(showing()).toBe('chat')

    await setWait(null)

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  // A restored tab nobody looks at would replay its feed at open, behind the one in view.
  it('opens a hidden tab’s chat only once it is shown, and keeps it after', async () => {
    const store = seed()
    const view = await renderPane(store, { isVisible: false })
    expect(chatStore.getState().feeds.c1).toBeUndefined()
    await act(async () => view.rerender(chatPane(store)))
    expect(chatStore.getState().feeds.c1).toBeDefined()
    await act(async () => view.rerender(chatPane(store, { isVisible: false })))
    expect(chatStore.getState().feeds.c1).toBeDefined()
  })

  // ── Don't fight the user ───────────────────────────────────────────

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

  // A user who walked to the terminal on their own is not lost, and nothing that
  // happens to the agent afterwards entitles the pane to move them.
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

  // Following the banner's own button is not the pane moving anybody, so it must
  // not auto-return — but Crowbar did send them, so it owes them the offer.
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

  // ── Doesn't overlap the composer ───────────────────────────────────

  // The banner rides INSIDE AgentEmptyDocument's `.dochandle`, sharing the composer row's one
  // `place()` transform, so a resize has no second position to drift out of sync with.
  it('renders the wait banner inside AgentEmptyDocument, not as a free-floating row above it', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })

    const banner = screen.getByTestId('agent-terminal-wait')
    const handle = banner.closest('.dochandle')
    expect(handle).not.toBeNull()
    expect(banner.closest('.banner')).not.toBeNull()
    // The document's own writing surface, not a sibling above it.
    expect(handle?.closest('.docwrap')).not.toBeNull()
  })

  // ── Doesn't render under the chat's own overlay header ──────────────
  // `--agent-header-clearance` is the one mechanism that keeps pinned-near-top
  // content clear of an overlay header; `belowOverlayHeader` says one floats here.
  it('clears the overlay header row when one floats above this pane', async () => {
    const store = seed()
    await renderPane(store, { belowOverlayHeader: true })
    await setWait({ kind: 'workspace_trust' })

    screen.getByTestId('agent-terminal-wait')
    // 44px header (IS_MAC forced true above) plus the pane's own original
    // 8px breathing room.
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

  // Open Terminal moves the surface but leaves `waiting` up until the CLI answers.
  it('keeps the chat surface hidden after Open Terminal, even while still waiting', async () => {
    const store = seed()
    await renderLeft(store)
    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('chat')

    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    expect(showing()).toBe('terminal')

    // The prompt is still pending: `waiting` stays true.
    const chatSurface = screen.getByTestId('agent-chat-surface')
    expect(chatSurface.className).toMatch(/\bhidden\b/)
    expect(chatSurface.className).not.toMatch(/\bflex\b/)
  })

  // ── The quiet case ─────────────────────────────────────────────────

  // A chat nothing is blocking gets no banner, no navigation and no offer.
  it('does nothing at all to a chat that is not waiting', async () => {
    const store = seed()

    await renderPane(store)

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-terminal-wait')).not.toBeInTheDocument()
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })
})

// A non-hotswap provider (codex) has no terminal view until switchToTerminal forks one.
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

  // The banner forked the terminal view, so the way back it offers drops it, as any return would.
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

  // A refused switch (a turn in flight) leaves the user on the chat, not an empty terminal.
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

// A reload keeps no surface of its own: a codex chat's view is the one its daemon holds.
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
