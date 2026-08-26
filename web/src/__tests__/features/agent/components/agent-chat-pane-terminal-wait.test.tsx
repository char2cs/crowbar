import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChat, AgentProvider } from '@/features/agent/api/agent-api'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

const { getChatFn, listMessagesFn, slashCatalogFn } = vi.hoisted(() => ({
  getChatFn: vi.fn(),
  listMessagesFn: vi.fn(),
  slashCatalogFn: vi.fn(),
}))

vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({}),
}))

vi.mock('@/features/agent/api/agent-api', async (importOriginal) => {
  // Partial: the module also exports the TERMINAL_WAIT_* constants the banner
  // reads, and stubbing those away would make it silently take its fallback
  // wording on every render — a green test over a broken banner.
  const actual = await importOriginal<typeof import('@/features/agent/api/agent-api')>()
  return {
    ...actual,
    getChat: (...a: unknown[]) => getChatFn(...a),
    switchProvider: vi.fn(),
    resumeChat: vi.fn(),
    listChatMessages: (...a: unknown[]) => listMessagesFn(...a),
    submitAgentPrompt: vi.fn(),
    getSlashCatalog: (...a: unknown[]) => slashCatalogFn(...a),
  }
})

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({ saveReconnect: vi.fn() }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: vi.fn() } }))

// jsdom cannot run xterm/WebGL. The marker records the visibility props the pane
// threads down, which is how "the terminal surface is the selected one" is proven
// without reading a class name.
vi.mock('@/features/terminal/components/terminal', () => ({
  XtermTerminal: ({ sessionId, isVisible }: { sessionId: string; isVisible?: boolean }) =>
    createElement('div', {
      'data-testid': 'xterm',
      'data-session-id': sessionId,
      'data-visible': String(isVisible),
    }),
}))

vi.mock('@/features/agent/components/provider-switch-dropdown', () => ({
  ProviderSwitchDropdown: () => createElement('button', { 'data-testid': 'provider-switch' }),
}))

import { AgentChatPane } from '@/features/agent/components/agent-chat-pane'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'

const providers: AgentProvider[] = [
  {
    id: 'claude',
    displayName: 'Claude',
    icon: '<svg/>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    // These tests move the SAME provider between chat/terminal via chooseSurface,
    // which is now capability-gated: hotswap:true is what keeps that move
    // synchronous (claude's terminal is already live) rather than routing
    // through switchToTerminal/switchToNative, which a non-hotswap provider
    // needs instead.
    hotswap: true,
  },
]

function liveChat(wait?: { kind: string }): AgentChat {
  return {
    id: 'c1',
    workspaceId: 'w1',
    title: 'Chat c1',
    liveRunnerId: 'r1',
    terminalSessionId: 'pty-1',
    activeProviderId: 'claude',
    createdAt: '',
    order: 0,
    terminalWait: wait,
  }
}

function seed(wait?: { kind: string }) {
  const store = createWorkspaceStore('w1')
  store.getState().setAgentProviders(providers)
  store.getState().seedAgentChats([liveChat(wait)])
  return store
}

type Store = ReturnType<typeof seed>

/** Render the pane directly rather than through a buffer: these tests are about
 *  the surface the pane SELECTS, and the three visibility axes are exactly the
 *  props under test — so they have to be settable one at a time. */
async function renderPane(
  store: Store,
  opts: { isActivePane?: boolean; isVisible?: boolean } = {},
) {
  const result = await act(async () =>
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(AgentChatPane, {
          chatId: 'c1',
          runnerId: 'r1',
          wsId: 'w1',
          bufferId: 'b1',
          isActivePane: opts.isActivePane ?? true,
          isVisible: opts.isVisible ?? true,
        }),
      ),
    ),
  )
  return result
}

/** Raise or clear the daemon's verdict the way the WS frame does. */
async function setWait(store: Store, wait: { kind: string } | null) {
  await act(async () => {
    store.getState().setAgentChatTerminalWait('c1', wait)
  })
}

/** Which surface is showing. The pane keeps both mounted and hides one, so the
 *  honest read is the toggle's own aria-pressed rather than a class. */
const showing = () =>
  (surfaceToggle().getAttribute('aria-selected') === 'true' ? 'terminal' : 'chat') as
    | 'chat'
    | 'terminal'

/** The Terminal half of the surface switcher. Named EXACTLY, because the banner's
 *  own call to action is "Open Terminal" and a loose match finds both. */
const surfaceToggle = () => screen.getByRole('tab', { name: 'Terminal' })

beforeEach(() => {
  getChatFn.mockReset()
  listMessagesFn.mockReset()
  slashCatalogFn.mockReset()
  listMessagesFn.mockResolvedValue({ cursor: 0, oldestCursor: 0, hasMore: false, items: [] })
  slashCatalogFn.mockResolvedValue({
    providerId: 'claude',
    completeness: 'model_visible',
    items: [],
    warnings: [],
  })
  getChatFn.mockImplementation(() => Promise.resolve({ ...liveChat(), conversations: [] }))
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

    await setWait(store, { kind: 'workspace_trust' })

    expect(screen.getByTestId('agent-terminal-wait')).toHaveTextContent(
      'Claude is asking whether you trust this folder',
    )
    expect(showing()).toBe('terminal')
  })

  it('reports an unidentified prompt without naming it', async () => {
    const store = seed()
    await renderPane(store)

    await setWait(store, { kind: '' })

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

    await setWait(store, { kind: 'workspace_trust' })

    expect(showing()).toBe('chat')
    expect(screen.getByTestId('agent-terminal-wait')).toBeInTheDocument()
  })

  it('does not switch a chat in a pane that does not have focus', async () => {
    const store = seed()
    await renderPane(store, { isActivePane: false })

    await setWait(store, { kind: 'workspace_trust' })

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

    await setWait(store, { kind: 'workspace_trust' })

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
    await setWait(store, { kind: 'workspace_trust' })
    expect(showing()).toBe('terminal')

    await setWait(store, null)

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-terminal-wait')).not.toBeInTheDocument()
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  // The user chose to stay in the terminal. Their choice outranks ours from then
  // on, so the pane must not haul them out of a terminal they are using.
  it('leaves the user where they chose to be, and offers the way back instead', async () => {
    const store = seed()
    await renderPane(store)
    await setWait(store, { kind: 'workspace_trust' })

    // Reasserting the surface they are already on IS a choice — it is the click
    // that says "I want to be here", and it is the only one available once the
    // pane has already moved them.
    fireEvent.click(surfaceToggle())
    await setWait(store, null)

    expect(showing()).toBe('terminal')
    expect(screen.getByTestId('agent-return-to-chat')).toBeInTheDocument()
  })

  // Never silently stranded: the pane sent them here, the reason has gone, and it
  // cannot take them back — so it says so.
  it('offers the way back when it cannot return the user itself', async () => {
    const store = seed()
    const { rerender } = await renderPane(store)
    await setWait(store, { kind: 'workspace_trust' })
    expect(showing()).toBe('terminal')

    // The user walks away — another pane takes focus — before the dialog clears.
    await act(async () => {
      rerender(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(AgentChatPane, {
            chatId: 'c1',
            runnerId: 'r1',
            wsId: 'w1',
            bufferId: 'b1',
            isActivePane: false,
            isVisible: true,
          }),
        ),
      )
    })
    await setWait(store, null)

    expect(showing()).toBe('terminal')
    expect(screen.getByTestId('agent-return-to-chat')).toBeInTheDocument()
  })

  it('takes the user back when they accept the offer', async () => {
    const store = seed()
    await renderPane(store)
    await setWait(store, { kind: 'workspace_trust' })
    fireEvent.click(surfaceToggle())
    await setWait(store, null)

    fireEvent.click(screen.getByRole('button', { name: 'Return to Chat' }))

    expect(showing()).toBe('chat')
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  it('stops offering when the user dismisses it', async () => {
    const store = seed()
    await renderPane(store)
    await setWait(store, { kind: 'workspace_trust' })
    fireEvent.click(surfaceToggle())
    await setWait(store, null)

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

    await setWait(store, { kind: 'workspace_trust' })
    await setWait(store, null)

    expect(showing()).toBe('terminal')
    expect(screen.queryByTestId('agent-return-to-chat')).not.toBeInTheDocument()
  })

  // Following the banner's own button is not the pane moving anybody, so it must
  // not auto-return — but Crowbar did send them, so it owes them the offer.
  it('offers the way back to a user who followed the banner', async () => {
    const store = seed()
    await renderPane(store, { isVisible: false })
    await setWait(store, { kind: 'workspace_trust' })
    expect(showing()).toBe('chat')

    fireEvent.click(screen.getByRole('button', { name: 'Open Terminal' }))
    expect(showing()).toBe('terminal')
    await setWait(store, null)

    expect(showing()).toBe('terminal')
    expect(screen.getByTestId('agent-return-to-chat')).toBeInTheDocument()
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
