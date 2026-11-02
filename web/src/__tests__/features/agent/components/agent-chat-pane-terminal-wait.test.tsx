import { createElement } from 'react'
import { act, fireEvent, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

const { slashCatalogFn, switchToTerminalFn } = vi.hoisted(() => ({
  slashCatalogFn: vi.fn(),
  switchToTerminalFn: vi.fn(),
}))

// The 44px header-clearance assertion below is a macOS value — force it
// rather than depend on whatever navigator.userAgent the test happens to run
// on (jsdom's UA bakes in the CI runner's own host platform, which is Linux
// on GitHub Actions).
vi.mock('@/utils/platform', () => ({ IS_MAC: true }))

import { chatPane, renderChatPane, stubPaneChrome } from '@/__tests__/__fixtures__/agent-pane'
stubPaneChrome()
import * as readsApi from '@/features/agent/api/agent-chat-reads'
vi.spyOn(readsApi, 'getSlashCatalog').mockImplementation(slashCatalogFn)
import * as commandsApi from '@/features/agent/api/agent-chat-commands'
vi.spyOn(commandsApi, 'switchProvider').mockImplementation(vi.fn())
vi.spyOn(commandsApi, 'resumeChat').mockImplementation(vi.fn())
vi.spyOn(commandsApi, 'submitAgentPrompt').mockImplementation(vi.fn())
vi.spyOn(commandsApi, 'switchToTerminal').mockImplementation(switchToTerminalFn)

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
import { resetChatPresentationMemoryForTests } from '@/features/agent/hooks/use-chat-presentation'
import {
  chatSnapshot,
  provider,
  resetChatStore,
  seedChats,
  setChatTerminalWait,
  setProviders,
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
  slashCatalogFn.mockReset()
  switchToTerminalFn.mockReset()
  switchToTerminalFn.mockResolvedValue('term-session-1')
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
  // The user's bug: the CLI is parked on a dialog no hook reports, the chat pane
  // shows nothing at all, and Crowbar looks broken. Both halves of the answer are
  // asserted here — the chat SAYS what is happening, and the user is taken to the
  // one place it can be dealt with.
  it('says what the agent is waiting on and takes the user to the terminal', async () => {
    const store = seed()
    await renderPane(store)
    expect(showing()).toBe('chat')

    await setWait({ kind: 'workspace_trust' })

    expect(screen.getByTestId('agent-terminal-wait')).toHaveTextContent(
      'Claude is asking whether you trust this folder',
    )
    expect(showing()).toBe('terminal')
  })

  it('reports an unidentified prompt without naming it', async () => {
    const store = seed()
    await renderPane(store)

    await setWait({ kind: '' })

    expect(screen.getByTestId('agent-terminal-wait')).toHaveTextContent(
      'Claude is waiting for input in its terminal',
    )
  })

  // ── Don't yank focus ───────────────────────────────────────────────

  // A hidden tab is still mounted for keep-alive, so nothing structural stops
  // this pane navigating — it has to decline. The banner is still raised, so the
  // user finds the explanation waiting when they come back.
  it('does not switch a chat the user is not looking at (hidden tab)', async () => {
    const store = seed()
    await renderPane(store, { isVisible: false })

    await setWait({ kind: 'workspace_trust' })

    expect(showing()).toBe('chat')
    expect(screen.getByTestId('agent-terminal-wait')).toBeInTheDocument()
  })

  it('does not switch a chat in a pane that does not have focus', async () => {
    const store = seed()
    await renderPane(store, { isActivePane: false })

    await setWait({ kind: 'workspace_trust' })

    expect(showing()).toBe('chat')
  })

  // The third axis, and the one that is invisible from inside the pane: a
  // RETAINED workspace stays mounted under display:none + inert, so isActivePane
  // and isVisible are both true for a pane nobody can see. Asking the registry is
  // the only way to know.
  it('does not switch a chat in a workspace that is not in view', async () => {
    const store = seed()
    await renderPane(store)
    setActiveWorkspaceId('w2')

    await setWait({ kind: 'workspace_trust' })

    expect(showing()).toBe('chat')
  })

  // Opening a chat that is ALREADY parked is the same rising edge as one that
  // becomes parked, and it is the commoner one: the CLI asks about trust before
  // the user has done anything.
  it('switches a chat that is already waiting when it is opened', async () => {
    const store = seed({ kind: 'workspace_trust' })

    await renderPane(store)

    expect(showing()).toBe('terminal')
  })

  // ── Don't fight the user ───────────────────────────────────────────

  it('returns the user to the chat once the prompt is cleared', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('terminal')

    await setWait(null)

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-terminal-wait')).not.toBeInTheDocument()
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  // The user chose to stay in the terminal. Their choice outranks ours from then
  // on, so the pane must not haul them out of a terminal they are using.
  it('leaves the user where they chose to be, and offers the way back instead', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })

    // Reasserting the surface they are already on IS a choice — it is the click
    // that says "I want to be here", and it is the only one available once the
    // pane has already moved them.
    fireEvent.click(surfaceToggle())
    await setWait(null)

    expect(showing()).toBe('terminal')
    expect(screen.getByTestId('agent-return-to-chat')).toBeInTheDocument()
  })

  // Never silently stranded: the pane sent them here, the reason has gone, and it
  // cannot take them back — so it says so.
  it('offers the way back when it cannot return the user itself', async () => {
    const store = seed()
    const { rerender } = await renderPane(store)
    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('terminal')

    // The user walks away — another pane takes focus — before the dialog clears.
    await act(async () => {
      rerender(chatPane(store, { isActivePane: false }))
    })
    await setWait(null)

    expect(showing()).toBe('terminal')
    expect(screen.getByTestId('agent-return-to-chat')).toBeInTheDocument()
  })

  it('takes the user back when they accept the offer', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })
    fireEvent.click(surfaceToggle())
    await setWait(null)

    fireEvent.click(screen.getByRole('button', { name: 'Return to Chat' }))

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  it('stops offering when the user dismisses it', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })
    fireEvent.click(surfaceToggle())
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
    await renderPane(store, { isVisible: false })
    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('chat')

    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    expect(showing()).toBe('terminal')
    await setWait(null)

    expect(showing()).toBe('terminal')
    expect(screen.getByTestId('agent-return-to-chat')).toBeInTheDocument()
  })

  // ── Doesn't overlap the composer ───────────────────────────────────

  // Task 1 measured a real, live overlap that got WORSE as the window
  // narrowed and the banner's own text wrapped to more lines: the banner sat
  // ABOVE the composer handle as its own free-floating row, so a resize could
  // separate the two. It now rides INSIDE AgentEmptyDocument's own
  // `.dochandle` — the SAME element the model/effort/attach/send row would
  // otherwise occupy, sharing that row's one `place()` transform — so there
  // is no second position left to drift out of sync with the first.
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
    expect(section.style.getPropertyValue('--agent-header-clearance')).toBe('52px')
  })

  it('keeps its original offset with no overlay header above (default)', async () => {
    const store = seed()
    await renderPane(store)
    await setWait({ kind: 'workspace_trust' })

    screen.getByTestId('agent-terminal-wait')
    const section = document.querySelector('.agent-chat.chat') as HTMLElement
    expect(section.style.getPropertyValue('--agent-header-clearance')).toBe('8px')
  })

  // The chat surface's className is built as `hidden`-when-not-
  // `presentation==='chat'`, full stop — there is no longer a second class to
  // reconcile it with (`flex flex-col` used to apply here too, only while
  // the wait banner was up, back when the banner was a sibling row this
  // container had to stack alongside AgentChatView). Reachable through the
  // banner's own primary CTA: `openTerminalFromBanner` flips `presentation`
  // to 'terminal' but does NOT clear `waiting` (that only clears once the CLI
  // actually answers) — so a user clicking "Open Terminal" while the prompt
  // is still pending is exactly `splitting=false, presentation='terminal',
  // waiting=true`.
  it('keeps the chat surface hidden after Open Terminal, even while still waiting', async () => {
    const store = seed()
    await renderPane(store, { isVisible: false })
    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('chat')

    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    expect(showing()).toBe('terminal')

    // Deliberately NOT calling setWait(null) here — the daemon has not
    // answered, `waiting` is still true, which is the exact case the bug
    // needs to reproduce.
    const chatSurface = screen.getByTestId('agent-chat-surface')
    expect(chatSurface.className).toMatch(/\bhidden\b/)
    expect(chatSurface.className).not.toMatch(/\bflex\b/)
  })

  // ── The quiet case ─────────────────────────────────────────────────

  // A chat nothing is blocking must be untouched by all of this: no banner, no
  // navigation, no offer. This is every chat of every provider that declares no
  // needles, forever.
  it('does nothing at all to a chat that is not waiting', async () => {
    const store = seed()

    await renderPane(store)

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-terminal-wait')).not.toBeInTheDocument()
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })
})

// Regression: a non-hotswap api-transport provider (codex) has no terminal
// session live until switchToTerminal forks one — every path onto the
// terminal surface used to call setPresentation('terminal') directly, which
// left the pane showing a terminal with nothing behind it and, from the
// user's side, no way to ever reach the provider's real TUI. The suite above
// only ever exercises claude (hotswap:true), where that shortcut happens to
// be correct — see its own `providers` fixture comment.
describe('AgentChatPane — waiting in the terminal (non-hotswap provider)', () => {
  it('calls switchToTerminal before showing the terminal on the automatic escort', async () => {
    const store = seedNonHotswap()
    await renderPane(store)
    expect(showing()).toBe('chat')

    await setWait({ kind: 'workspace_trust' })

    expect(screen.getByTestId('agent-terminal-wait')).toHaveTextContent(
      'Codex is asking whether you trust this folder',
    )
    await waitFor(() => expect(showing()).toBe('terminal'))
    expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1')
  })

  it("calls switchToTerminal when the wait banner's Open Terminal button is used", async () => {
    const store = seedNonHotswap()
    await renderPane(store, { isVisible: false })
    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('chat')

    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))

    await waitFor(() => expect(showing()).toBe('terminal'))
    expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1')
  })

  // A refused switch (a turn in flight) must leave
  // the user on chat rather than stranding them on an empty terminal view —
  // the same "left where they were" contract chooseSurface's own click path
  // already had.
  it('leaves the user on chat when switchToTerminal is refused', async () => {
    switchToTerminalFn.mockRejectedValue(new Error('agent: provider cannot hand a live turn'))
    const store = seedNonHotswap()
    await renderPane(store)

    await setWait({ kind: 'workspace_trust' })
    await waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1'))

    expect(showing()).toBe('chat')
  })
})
