import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from 'zustand'
import type { AgentChat, AgentChatDetail, AgentProvider } from '@/features/agent/api/agent-api'
import { ApiError } from '@/lib/api'
import type { AgentChatContent } from '@/features/panes/types/pane-content'
import {
  WorkspaceStoreContext,
  useWorkspaceStore,
} from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

// Hoisted fakes — declared before the vi.mock calls that reference them.
const {
  getChatFn,
  switchProviderFn,
  resumeChatFn,
  listMessagesFn,
  submitPromptFn,
  slashCatalogFn,
  saveReconnectFn,
  toastErrorFn,
  switchToTerminalFn,
  switchToNativeFn,
} = vi.hoisted(() => ({
  getChatFn: vi.fn(),
  switchProviderFn: vi.fn(),
  resumeChatFn: vi.fn(),
  listMessagesFn: vi.fn(),
  submitPromptFn: vi.fn(),
  slashCatalogFn: vi.fn(),
  saveReconnectFn: vi.fn(),
  toastErrorFn: vi.fn(),
  switchToTerminalFn: vi.fn(),
  switchToNativeFn: vi.fn(),
}))

// The pane resolves its toggle-view chord through the keymap (so it stays
// rebindable); pin it here rather than standing up the settings store.
// 'agent.cycleProvider' is deliberately absent — it now ships unbound by
// default, and an absent key resolves the same way (falsy) as its real ''.
vi.mock('@/features/keymaps/hooks/use-effective-keymap', () => ({
  useEffectiveChordMap: () => ({ 'agent.toggleViewMode': 'mod+/' }),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  getChat: (...a: unknown[]) => getChatFn(...a),
  switchProvider: (...a: unknown[]) => switchProviderFn(...a),
  resumeChat: (...a: unknown[]) => resumeChatFn(...a),
  listChatMessages: (...a: unknown[]) => listMessagesFn(...a),
  submitAgentPrompt: (...a: unknown[]) => submitPromptFn(...a),
  getSlashCatalog: (...a: unknown[]) => slashCatalogFn(...a),
  switchToTerminal: (...a: unknown[]) => switchToTerminalFn(...a),
  switchToNative: (...a: unknown[]) => switchToNativeFn(...a),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: (...a: unknown[]) => saveReconnectFn(...a),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: (...a: unknown[]) => toastErrorFn(...a) },
}))

// jsdom can't run xterm/WebGL — stub the terminal renderer to a passive marker
// that records the sessionId it was mounted with (that's what the attach seam is
// proven by) plus the isActive/isVisible/attachOnly props threaded from the pane.
// The marker is clickable so a test can fire the terminal's onSessionGone — the
// pane's half of the "the PTY died under a mounted pane" contract. Click reports
// the session it is MOUNTED with (what real xterm does); double-click reports a
// stale id, standing in for a displaced PTY whose death lands late.
vi.mock('@/features/terminal/components/terminal', () => ({
  XtermTerminal: ({
    sessionId,
    isActive,
    isVisible,
    attachOnly,
    flush,
    onSessionGone,
  }: {
    sessionId: string
    isActive: boolean
    isVisible?: boolean
    attachOnly?: boolean
    flush?: boolean
    onSessionGone?: (goneSessionId: string) => void
  }) =>
    createElement('div', {
      'data-testid': 'xterm',
      'data-session-id': sessionId,
      'data-active': String(isActive),
      'data-visible': String(isVisible),
      'data-attach-only': String(Boolean(attachOnly)),
      'data-flush': String(Boolean(flush)),
      onClick: () => onSessionGone?.(sessionId),
      onDoubleClick: () => onSessionGone?.('pty-displaced'),
    }),
}))

// The prompt box is a Plate editor, and **jsdom never delivers a keydown to a
// Slate editable** — measured: window- and document-capture see the event, the
// editable's own listeners never fire. Neither `PlateContent onKeyDown` nor a
// plugin handler runs. So a test that typed into the real editor here would not
// be testing the queue, it would be testing nothing and passing.
//
// These suites are about the QUEUE, the catalog and the ledger. The editor gets
// a stand-in with the same contract — text in, markdown out, keys through — and
// the editor's own behaviour is verified live and in its own suite.
vi.mock('@/features/agent/composer/plate/chat-markdown-editor', () => ({
  ChatMarkdownEditor: ({
    initialValue,
    placeholder,
    ariaLabel,
    onChange,
    onKeyDown,
    expanded,
    controls,
  }: {
    initialValue: string
    placeholder: string
    ariaLabel: string
    onChange: (value: string) => void
    onKeyDown: (
      event: unknown,
      readMarkdown: () => string,
      caret: { atStart: boolean; atEnd: boolean },
    ) => void
    expanded?: boolean
    controls?: string
  }) =>
    createElement('textarea', {
      'aria-label': ariaLabel,
      'aria-expanded': expanded,
      'aria-controls': controls,
      placeholder,
      defaultValue: initialValue,
      onChange: (event: { target: { value: string } }) => onChange(event.target.value),
      // Second argument included deliberately: the real editor hands the key
      // handler the BOX's text, and a mock that omitted it would let a submit
      // path that reads stale state keep passing. Third argument is a stand-in
      // for the real editor's own caret-edge probe — see the identical note in
      // agent-chat-view.test.tsx's own mock of this module.
      onKeyDown: (event: { currentTarget: { value: string } }) =>
        onKeyDown(event, () => event.currentTarget.value, { atStart: true, atEnd: true }),
    }),
}))

// Stub the dropdown to expose its props and a one-click switch, so the footer
// wiring is asserted without the shared Dropdown's framer-motion machinery.
vi.mock('@/features/agent/components/provider-switch-dropdown', () => ({
  ProviderSwitchDropdown: ({
    providers,
    currentProviderId,
    onSwitch,
    disabled,
  }: {
    providers: AgentProvider[]
    currentProviderId: string
    onSwitch: (id: string) => void
    disabled?: boolean
  }) =>
    createElement(
      'button',
      {
        'data-testid': 'provider-switch',
        'data-current': currentProviderId,
        'data-count': String(providers.length),
        disabled,
        onClick: () => onSwitch('codex'),
      },
      'switch',
    ),
}))

import { AgentChatPane } from '@/features/agent/components/agent-chat-pane'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useSettingsStore } from '@/features/settings/store'

/**
 * Land this pane on the TERMINAL surface.
 *
 * The chat's status strip — its title and the provider switcher — is the
 * provider's-own-view chrome and is drawn only there: Chat states everything it
 * is running as in its own underbar, under the composer. So a test about the
 * switcher has to be on the surface that has one.
 */
function landOnTerminal() {
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: false },
  }))
}

// Both real descriptors declare hotswap:true and keep a real terminal — see
// TestShippedDescriptors_DeclareHotswapTrue on the backend — so these fixtures
// match that rather than exercising the (currently provider-less) false branch,
// which has its own dedicated coverage in agent-chat-view.test.tsx.
const providers: AgentProvider[] = [
  {
    id: 'claude',
    displayName: 'Claude',
    icon: '<svg/>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    hasTerminal: true,
    hotswap: true,
  },
  {
    id: 'codex',
    displayName: 'Codex',
    icon: '<svg/>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    hasTerminal: true,
    hotswap: true,
  },
]

// ── Wire fixtures ────────────────────────────────────────────────────
// A chat is LIVE exactly while a runner is placed on it. liveRunnerId is the whole
// liveness contract — no status flag exists that could disagree with it — and it
// carries that runner's PTY, which is what the pane attaches to.

function liveChat(o: {
  id: string
  runnerId: string
  pty: string
  title?: string
  provider?: string
}): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: o.title ?? `Chat ${o.id}`,
    liveRunnerId: o.runnerId,
    terminalSessionId: o.pty,
    activeProviderId: o.provider ?? 'codex',
    createdAt: '',
    order: 0,
  }
}

/** A dormant chat: no runner points at it, so there is nothing to attach. It keeps
 *  the provider of its last conversation — who Resume brings back. */
function dormantChat(o: { id: string; title?: string; provider?: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: o.title ?? `Chat ${o.id}`,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: o.provider ?? 'codex',
    createdAt: '',
    order: 0,
  }
}

/** A LIVE runner with nothing attached — the correct idle shape for a non-hotswap
 *  api-transport provider (codex) that has never been switched to its native view.
 *  This is NOT dormancy: liveRunnerId is the only liveness signal, and it is set. */
function liveChatNoTerminal(o: { id: string; runnerId: string; title?: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: o.title ?? `Chat ${o.id}`,
    liveRunnerId: o.runnerId,
    terminalSessionId: '',
    activeProviderId: 'codex',
    createdAt: '',
    order: 0,
  }
}

function detail(chat: AgentChat): AgentChatDetail {
  return { ...chat, conversations: [] }
}

/** A promise this test resolves by hand. The pane is asserted MID-FLIGHT (the spinner is
 *  a real state, not a frame of one), and nothing here waits on a clock — the test drives
 *  the request's completion itself. */
function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

// ── Harness ──────────────────────────────────────────────────────────
// The REAL workspace store, so the pane's repoint writes land on a real buffer.
// PaneHost is exactly what pane-container does: read the buffer, feed its
// chatId/runnerId back in as props. That closes the loop the feature IS — the
// buffer is the pane's moving target, and the pane is what moves it.

function seedWorkspace(chats: AgentChat[], wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  store.getState().setAgentProviders(providers)
  store.getState().seedAgentChats(chats)
  return store
}

type Store = ReturnType<typeof seedWorkspace>

function openBuffer(store: Store, chatId: string, runnerId: string, name = 'Chat', wsId = 'w1') {
  return store.getState().bufferActions.openContent({
    type: 'agentChat',
    chatId,
    wsId,
    name,
    runnerId,
  })
}

function PaneHost({ bufferId, isVisible = true }: { bufferId: string; isVisible?: boolean }) {
  const store = useWorkspaceStore()
  const buf = useStore(store, (s) => s.buffers.find((b) => b.id === bufferId)) as
    AgentChatContent | undefined
  if (!buf) return null
  return createElement(AgentChatPane, {
    chatId: buf.chatId,
    runnerId: buf.runnerId,
    wsId: buf.wsId,
    bufferId: buf.id,
    isActivePane: true,
    // Default true: the vast majority of these tests are the ACTIVE, visible tab.
    // The keep-alive suite drives this false to prove a hidden chat doesn't revive.
    isVisible,
  })
}

async function renderPane(store: Store, bufferId: string) {
  await act(async () => {
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(PaneHost, { bufferId }),
      ),
    )
  })
}

const buffer = (store: Store, id: string) =>
  store.getState().buffers.find((b) => b.id === id) as AgentChatContent | undefined

// The default backend is a HEALTHY one: a resume brings the chat's CLI back, and reading
// the chat back afterwards shows the runner now on it. Tests that are about failure say
// so explicitly by overriding these — nothing else has to opt in to "it worked".
beforeEach(() => {
  getChatFn.mockReset()
  switchProviderFn.mockReset()
  resumeChatFn.mockReset()
  listMessagesFn.mockReset()
  submitPromptFn.mockReset()
  slashCatalogFn.mockReset()
  saveReconnectFn.mockReset()
  toastErrorFn.mockReset()
  switchToTerminalFn.mockReset()
  switchToNativeFn.mockReset()
  switchProviderFn.mockResolvedValue('r-new')
  resumeChatFn.mockResolvedValue('r-revived')
  // A chat that has been SPOKEN IN. A chat with no messages is the blank
  // DOCUMENT surface — writing size, no pill under it — so a pane test about the
  // composer, the queue or the switcher has to start from a conversation for its
  // subject to be on screen at all.
  listMessagesFn.mockResolvedValue({
    cursor: 1,
    oldestCursor: 1,
    hasMore: false,
    items: [
      {
        sequence: 1,
        turnId: 'turn-1',
        role: 'assistant',
        providerId: 'claude',
        text: 'earlier turn',
        at: '2026-08-16T00:00:01Z',
      },
    ],
  })
  submitPromptFn.mockResolvedValue({ runnerId: 'r-prompt', terminalSessionId: 'pty-prompt' })
  slashCatalogFn.mockResolvedValue({
    providerId: 'codex',
    completeness: 'model_visible',
    items: [],
    warnings: [],
  })
  getChatFn.mockImplementation((_wsId: unknown, id: unknown) =>
    Promise.resolve(
      detail(liveChat({ id: String(id), runnerId: 'r-revived', pty: 'pty-revived' })),
    ),
  )
  useTerminalStore.setState({ sessions: new Map() })
  // The settings store is a GLOBAL singleton, so a test that lands the pane on
  // the terminal leaks that choice into every test after it. Reset to the
  // shipped default — Chat — before each one.
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
  localStorage.clear()
  // Every route that mounts a workspace publishes it as THE active one
  // (WorkspaceView). The pane's window-level chord listener is gated on that,
  // because a RETAINED workspace stays mounted (display:none + inert) with its
  // listener still registered — see the hidden-workspace test below.
  setActiveWorkspaceId('w1')
})

describe('AgentChatPane', () => {
  // ── THE HEADLINE ───────────────────────────────────────────────────
  // The user's bug: they type /clear inside the CLI, the CLI switches conversation,
  // and Crowbar moves the running process to a DIFFERENT chat. The pane used to be
  // pinned to a chatId for life, so it went "This agent has exited" — with a Resume
  // button that would spawn a SECOND CLI — while the first was alive and well in a
  // chat the user had to go find. The tab is a VIEWPORT on a moving target: it
  // follows the runner, and because the terminal is keyed by the PTY (which a move
  // does not change), the conversation changes WITHOUT changing the terminal.
  it('follows its runner to a new chat without remounting the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    const term = await screen.findByTestId('xterm')
    expect(term).toHaveAttribute('data-session-id', 'pty1')

    // The runner /clears into a brand-new chat — carrying the SAME pty.
    await act(async () => {
      store
        .getState()
        .seedAgentChats([
          dormantChat({ id: 'c1' }),
          liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1', title: 'Fresh' }),
        ])
    })

    // The tab re-points at the chat the runner is in NOW...
    expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c2', runnerId: 'r1' })
    // ...and relabels to that chat's title...
    expect(buffer(store, bufferId)?.name).toBe('Fresh')
    // ...while the terminal is the SAME DOM NODE. Not a remount: the very same
    // xterm instance, still attached to the same live PTY.
    expect(await screen.findByTestId('xterm')).toBe(term)
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
  })

  // The case above moves the runner into a chat that HAS a title, and that is exactly
  // why it never caught this: a real /clear lands the runner on a chat nobody has named
  // yet. The rename effect used to bail on an empty title, so the tab kept wearing the
  // PREVIOUS chat's name — pointing at a conversation it was no longer showing. Found by
  // running it: the tab still read "reply with exactly: ORION" after the /clear.
  it('relabels to the untitled placeholder when the runner /clears into a fresh chat', async () => {
    const store = seedWorkspace([
      liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', title: 'Respond With Orion' }),
    ])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)
    expect(buffer(store, bufferId)?.name).toBe('Respond With Orion')

    // /clear: same runner, same pty, brand-new chat — and it has NO title yet.
    // (title: '' explicitly — the fixtures default an omitted title to `Chat <id>`,
    // which is why the sibling test never exercised the untitled destination at all.)
    await act(async () => {
      store
        .getState()
        .seedAgentChats([
          dormantChat({ id: 'c1', title: 'Respond With Orion' }),
          liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1', title: '' }),
        ])
    })

    expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c2', runnerId: 'r1' })
    expect(buffer(store, bufferId)?.name).toBe('Untitled chat')
    expect(buffer(store, bufferId)?.name).not.toBe('Respond With Orion')
  })

  // Losing your runner because it MOVED is not your CLI dying. The old pane could
  // not tell those apart, so it offered a Resume button that spawned a SECOND CLI
  // on the old conversation while the first kept running.
  it('does not show the exited state when the runner merely moved', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    await act(async () => {
      store
        .getState()
        .seedAgentChats([
          dormantChat({ id: 'c1' }),
          liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' }),
        ])
    })

    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
    expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
    expect(resumeChatFn).not.toHaveBeenCalled()
    expect(screen.getByTestId('xterm')).toBeTruthy()
  })

  // ── THE SECOND HEADLINE: OPENING A DORMANT CHAT REVIVES IT ─────────
  // The user's report: "resuming conversations keeps the state like this" — the exited
  // message and a Resume button — "instead of Crowbar reviving the agent by itself".
  // Agent PTYs never survive a daemon restart, so dormant is the ordinary state of
  // yesterday's conversation, and a user clicking a chat is asking for the CHAT, not
  // for a button asking whether they meant it. The exited copy is now a FAILURE state,
  // and nothing else.
  describe('auto-revive', () => {
    it('revives a dormant chat on open — spinner, then the agent is back', async () => {
      const resumed = deferred<string>()
      resumeChatFn.mockReturnValue(resumed.promise)

      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      const bufferId = openBuffer(store, 'c1', '')
      await renderPane(store, bufferId)

      // Mid-flight: the EXISTING spinner, and not a trace of the button the user
      // complained about.
      expect(resumeChatFn).toHaveBeenCalledWith('w1', 'c1')
      expect(screen.getByText(/resuming this chat/i)).toBeTruthy()
      expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
      expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()

      await act(async () => {
        resumed.resolve('r9')
      })

      // Landed: the revived runner's PTY is attached and the tab follows it.
      const xterm = await screen.findByTestId('xterm')
      expect(xterm).toHaveAttribute('data-session-id', 'pty-revived')
      expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c1', runnerId: 'r-revived' })
      expect(useTerminalStore.getState().getSession('pty-revived')?.connectionId).toBe(
        'pty-revived',
      )
      expect(toastErrorFn).not.toHaveBeenCalled()
    })

    it('shows the failure message and the Resume button when the revive throws', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      // The real one the user has two of: a chat whose CLI died before it ever bound a
      // conversation. The backend refuses to resume it, always — so this must SETTLE.
      resumeChatFn.mockRejectedValue(new Error('agent: resume chat: no conversation to resume'))

      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openBuffer(store, 'c1', ''))

      expect(screen.getByText(/could not restart this agent/i)).toBeTruthy()
      expect(screen.getByTestId('pane-resume')).toBeTruthy()
      // The reason is REPORTED, and the PATH is not blamed for a failure that has
      // nothing to do with it.
      const [, why] = toastErrorFn.mock.calls[0] as [string, string]
      expect(why).toContain('no conversation to resume')
      expect(why).not.toMatch(/PATH/)
      expect(resumeChatFn).toHaveBeenCalledTimes(1)
      expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()
      err.mockRestore()
    })

    // THE TERMINAL_WAIT LESSON, applied to the OTHER two: a blank chat (nothing
    // ever said yet) never mounts AgentChatView's dock at all — AgentEmptyDocument
    // renders instead — so a reviving/idle-failed signpost that lives ONLY in the
    // composer would be silently missing for exactly the chat most likely to hit
    // it (a CLI that dies before its very first turn ever lands). Proven with a
    // real assertion, not by inspection — see the terminal_wait suite for why that
    // distinction matters.
    it('still shows the failure message and Resume for a chat with no messages yet', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      resumeChatFn.mockRejectedValue(new Error('agent: resume chat: no conversation to resume'))
      listMessagesFn.mockResolvedValue({ cursor: 0, oldestCursor: 0, hasMore: false, items: [] })

      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openBuffer(store, 'c1', ''))

      // The blank surface is genuinely up — this is not accidentally exercising
      // the non-blank path.
      expect(await screen.findByTestId('agent-empty-document')).toBeInTheDocument()
      expect(screen.getByText(/could not restart this agent/i)).toBeTruthy()
      expect(screen.getByTestId('pane-resume')).toBeTruthy()
      err.mockRestore()
    })

    it('still shows the reviving spinner for a chat with no messages yet', async () => {
      const resumed = deferred<string>()
      resumeChatFn.mockReturnValue(resumed.promise)
      listMessagesFn.mockResolvedValue({ cursor: 0, oldestCursor: 0, hasMore: false, items: [] })

      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openBuffer(store, 'c1', ''))

      expect(await screen.findByTestId('agent-empty-document')).toBeInTheDocument()
      expect(screen.getByText(/resuming this chat/i)).toBeTruthy()

      await act(async () => {
        resumed.resolve('r9')
      })
    })

    it('fails honestly when the revived CLI dies on startup (resumed, but nothing on the chat)', async () => {
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      getChatFn.mockResolvedValue(detail(dormantChat({ id: 'c1' }))) // resumed → still nobody there
      await renderPane(store, openBuffer(store, 'c1', ''))

      expect(screen.getByText(/could not restart this agent/i)).toBeTruthy()
      expect(screen.getByTestId('pane-resume')).toBeTruthy()
      expect(resumeChatFn).toHaveBeenCalledTimes(1)
    })

    // THE SPAWN-LOOP GUARD. A revive is spent per chat, per mount — whatever the outcome.
    // A failed revive that re-fired on every store push would hammer the daemon with
    // spawns for a CLI that cannot start; that budget is the entire safety argument for
    // reviving without being asked.
    it('never revives the same chat twice by itself, however often the store pushes', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      resumeChatFn.mockRejectedValue(new Error('claude: not on PATH'))

      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openBuffer(store, 'c1', ''))
      expect(resumeChatFn).toHaveBeenCalledTimes(1)

      // Every WS frame the chat could possibly get, re-seeded — the failure state must
      // not become a retry loop.
      for (const _ of [1, 2, 3]) {
        await act(async () => {
          store.getState().seedAgentChats([dormantChat({ id: 'c1' })])
        })
      }

      expect(resumeChatFn).toHaveBeenCalledTimes(1)
      expect(screen.getByText(/could not restart this agent/i)).toBeTruthy()
      // The manual retry is still there — that is what the button is FOR now.
      await act(async () => {
        fireEvent.click(screen.getByTestId('pane-resume'))
      })
      expect(resumeChatFn).toHaveBeenCalledTimes(2)
      err.mockRestore()
    })

    // A CLI that comes back and dies again is not revived a second time: at that point
    // the user is deliberately quitting it (/exit), and fighting them is the spawn loop
    // wearing a different hat. The exited copy is honest HERE — and only here.
    it('lets an agent stay dead once it has already been brought back this mount', async () => {
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      const bufferId = openBuffer(store, 'c1', '')
      await renderPane(store, bufferId)

      expect(await screen.findByTestId('xterm')).toBeTruthy() // revived once
      expect(resumeChatFn).toHaveBeenCalledTimes(1)

      // It exits again — the daemon reaps the runner and the chat goes dormant.
      await act(async () => {
        store.getState().seedAgentChats([dormantChat({ id: 'c1' })])
      })

      expect(resumeChatFn).toHaveBeenCalledTimes(1) // NOT revived again
      expect(screen.getByText(/this agent has exited/i)).toBeTruthy()
      expect(screen.getByTestId('pane-resume')).toBeTruthy()
    })

    it('never revives from the pending state (the chat list has not landed)', async () => {
      const store = seedWorkspace([]) // the seed is still in flight
      const bufferId = openBuffer(store, 'c1', 'r1')
      await renderPane(store, bufferId)

      // "Not known" is not "dormant". Reviving here would spawn a SECOND CLI onto a
      // chat that may well already have one.
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()

      // ...and the moment the list lands with the chat LIVE, it attaches — no revive.
      await act(async () => {
        store.getState().seedAgentChats([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      })
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
    })

    it('does not revive a chat whose runner merely moved away', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      // c1 is now dormant — but its runner is not dead, it walked into c2, and the pane
      // walks with it. A dormant chat NOBODY IS LOOKING AT must not be revived.
      await act(async () => {
        store
          .getState()
          .seedAgentChats([
            dormantChat({ id: 'c1' }),
            liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' }),
          ])
      })

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
    })

    // Regression: a non-hotswap api-transport runner (codex) is legitimately live
    // with an empty terminalSessionId whenever it has never been switched to its
    // native view — that used to be indistinguishable from dormant (both had an
    // empty terminalSessionId), so this state fired a needless revive onto a chat
    // that already had a perfectly healthy runner on it, and any failure of that
    // SECOND, unwanted resume then latched the whole chat into `idle: failed`
    // permanently — Resume re-running the exact same needless resume every time.
    // Confirmed live. liveRunnerId is the only thing that may mean "no runner".
    it('does not revive a live runner that simply has no terminal to attach', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
      expect(screen.queryByText(/could not restart this agent/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).toBeNull()
    })

    // The terminal surface's own placeholder for this same state, visible once the
    // user actually looks at the terminal view for a runner with nothing attached.
    it('shows a plain placeholder, never Resume, in the terminal view of a live runner with no terminal', async () => {
      landOnTerminal()
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).toBeNull()
      expect(screen.getByText(/no terminal view attached/i)).toBeTruthy()
    })

    // TestRegression_SwitchToTerminal coverage gap: every other test in this file
    // clicks the Terminal tab on a HOTSWAP provider (the fixture default), which
    // takes chooseSurface's `hotswap` branch and never calls the endpoint at all.
    // Codex is the one shipped provider that is NOT hotswap, and nothing exercised
    // its actual click-to-switch path — see runner/attach_internal_test.go for the
    // backend half of this same gap.
    it('calls switchToTerminal (never a direct presentation flip) when an idle non-hotswap provider is asked for its terminal', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      store.getState().setAgentProviders([providers[0], { ...providers[1], hotswap: false }])
      switchToTerminalFn.mockResolvedValue('pty-attached')
      getChatFn.mockImplementation((_wsId: unknown, id: unknown) =>
        Promise.resolve(
          detail(
            liveChat({ id: String(id), runnerId: 'r1', pty: 'pty-attached', provider: 'codex' }),
          ),
        ),
      )
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i }))

      // Not a synchronous flip like the hotswap case — the tab does not read
      // selected until the switch actually resolves.
      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'false',
      )
      await vi.waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1'))
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty-attached')
      await vi.waitFor(() =>
        expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
          'aria-selected',
          'true',
        ),
      )
    })

    // Regression: a refused switch (a turn still in flight, or codex before its
    // first completed turn ever wrote a rollout to resume — both real 409s the
    // backend already returns, see attach.go's ErrTurnInProgress /
    // ErrNativeViewNotYetAvailable) used to be swallowed with no feedback at
    // all: the tab click did nothing visible, which reads as broken rather than
    // refused.
    it('toasts when switchToTerminal is refused, instead of silently doing nothing', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      store.getState().setAgentProviders([providers[0], { ...providers[1], hotswap: false }])
      switchToTerminalFn.mockRejectedValue(
        new ApiError(
          'agent: provider has no completed turn yet to show its native view of: conflict',
          409,
        ),
      )
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i }))

      await vi.waitFor(() => expect(toastErrorFn).toHaveBeenCalledTimes(1))
      const [title] = toastErrorFn.mock.calls[0] as [string, string]
      expect(title).toContain('Codex')
      expect(screen.queryByTestId('xterm')).toBeNull()
      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'false',
      )
    })
  })

  // ── Keep-alive: the hidden-tab revive gate ─────────────────────────
  // The pane now keeps every chat MOUNTED (visibility:hidden) so a tab switch never
  // remounts a live PTY. That makes a chat MOUNTED-BUT-HIDDEN a real state — and a
  // hidden DORMANT chat must not auto-revive: a workspace with N dormant chat tabs would
  // otherwise fire N revives at once, one CLI per hidden tab. Only the visible tab revives.
  describe('hidden keep-alive tab', () => {
    // isVisible=false is the hidden tab; isActivePane=true proves the gate is on
    // VISIBILITY, not pane focus — a dormant chat sitting hidden inside the active pane
    // still must not spawn a CLI (Risk #4: the two flags are distinct).
    it('does not revive a hidden dormant chat', async () => {
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      const bufferId = openBuffer(store, 'c1', '')
      await act(async () => {
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(AgentChatPane, {
              chatId: 'c1',
              runnerId: '',
              wsId: 'w1',
              bufferId,
              isActivePane: true,
              isVisible: false,
            }),
          ),
        )
      })

      // Nothing spawned, and no spinner offering to — the chat just waits, hidden.
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).toBeNull()
    })

    // Switching TO a dormant chat's tab is exactly what makes it visible — and that is
    // when "opening a dormant chat revives it" fires. It must revive EXACTLY ONCE: the
    // attemptedRef budget is spent inside revive(), so re-hiding and re-showing cannot
    // fire a second spawn.
    it('revives a dormant chat exactly once when it becomes visible, never again', async () => {
      const resumed = deferred<string>()
      resumeChatFn.mockReturnValue(resumed.promise)

      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      const bufferId = openBuffer(store, 'c1', '')
      const props = {
        chatId: 'c1',
        runnerId: '',
        wsId: 'w1',
        bufferId,
        isActivePane: true,
      }
      const host = (isVisible: boolean) =>
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(AgentChatPane, { ...props, isVisible }),
        )

      let rerender!: (ui: ReturnType<typeof host>) => void
      await act(async () => {
        rerender = render(host(false)).rerender
      })
      expect(resumeChatFn).not.toHaveBeenCalled() // hidden: dormant, but no CLI spawned

      // Becomes the active tab → the one and only revive fires.
      await act(async () => {
        rerender(host(true))
      })
      expect(resumeChatFn).toHaveBeenCalledTimes(1)
      expect(screen.getByText(/resuming this chat/i)).toBeTruthy()

      // The revive lands: the chat is now attached (live pty in the store).
      await act(async () => {
        resumed.resolve('r9')
      })
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty-revived')

      // Re-hiding then re-showing must NOT spawn a second CLI — budget is spent, and the
      // chat is attached now anyway.
      await act(async () => {
        rerender(host(false))
      })
      await act(async () => {
        rerender(host(true))
      })
      expect(resumeChatFn).toHaveBeenCalledTimes(1)
    })

    // Keep-alive's other half: an ALREADY-ATTACHED chat keeps its live PTY while hidden.
    // It has a sessionId, so it never reaches the revive gate — it seeds/attaches as
    // usual, just not focused and not visible.
    it('keeps an attached chat mounted while hidden (no revive, terminal stays)', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const bufferId = openBuffer(store, 'c1', 'r1')
      await act(async () => {
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(AgentChatPane, {
              chatId: 'c1',
              runnerId: 'r1',
              wsId: 'w1',
              bufferId,
              isActivePane: false,
              isVisible: false,
            }),
          ),
        )
      })

      const xterm = await screen.findByTestId('xterm')
      expect(xterm).toHaveAttribute('data-session-id', 'pty1')
      expect(xterm.getAttribute('data-visible')).toBe('false')
      expect(xterm.getAttribute('data-active')).toBe('false')
      expect(resumeChatFn).not.toHaveBeenCalled()
    })
  })

  // ── Attaching ──────────────────────────────────────────────────────
  it('attaches the live runner PTY: seeds the mapping, then mounts the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    const xterm = await screen.findByTestId('xterm')
    expect(xterm.getAttribute('data-session-id')).toBe('pty1')
    // Chat is the default presentation. The PTY remains attach-only and mounted,
    // but xterm may neither focus nor resize while it is behind Chat.
    expect(xterm.getAttribute('data-active')).toBe('false')
    expect(xterm.getAttribute('data-visible')).toBe('false')
    // Attach-only: a reconnect can never spawn a bare shell into the agent frame.
    expect(xterm.getAttribute('data-attach-only')).toBe('true')
    // The mapping that makes resolveTerminalConnection ATTACH exists at mount.
    expect(useTerminalStore.getState().getSession('pty1')?.connectionId).toBe('pty1')
    expect(saveReconnectFn).toHaveBeenCalledWith('w1', 'pty1', 'pty1')
    // liveRunnerId IS the liveness answer — no second round trip asks the daemon.
    expect(getChatFn).not.toHaveBeenCalled()
  })

  it('renders nothing (not the exited state) while the chat list is still loading', async () => {
    // The seed is in flight: the store does not know this chat yet. "Not known" is
    // not "dormant" — flashing Resume here would offer a button that spawns a
    // second CLI onto a chat that may well be live.
    const store = seedWorkspace([])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    expect(screen.queryByTestId('xterm')).toBeNull()
    expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
  })

  it('threads isActivePane=false through to the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await act(async () => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(AgentChatPane, {
            chatId: 'c1',
            runnerId: 'r1',
            wsId: 'w1',
            bufferId,
            isActivePane: false,
            isVisible: true,
          }),
        ),
      )
    })

    // Chat is selected, so xterm is neither active nor visible regardless of pane focus.
    const xterm = await screen.findByTestId('xterm')
    expect(xterm.getAttribute('data-active')).toBe('false')
    expect(xterm.getAttribute('data-visible')).toBe('false')
  })

  // ── Adopting a new runner on the same chat ─────────────────────────
  // A runner replacement lands a new CLI IN PLACE: same chat, new PTY. The pane's
  // old runner is gone from everywhere, so it adopts whoever is on its chat now —
  // and it does so WITHOUT remounting the terminal. The PTY genuinely changed, but
  // the terminal is the same DOM node with a new sessionId: XtermTerminal swaps the
  // attachment imperatively (detach old PTY, attach new) rather than tearing the
  // whole component — socket, listeners, observers — down and rebuilding it. This is
  // the P4c fix: the terminal used to be key={sessionId} and remounted here.
  it('adopts the chat new runner in place — new PTY, but the SAME terminal (no remount)', async () => {
    landOnTerminal()
    const store = seedWorkspace([
      liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'codex' }),
    ])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    const before = await screen.findByTestId('xterm')
    expect(before).toHaveAttribute('data-session-id', 'pty1')

    await act(async () => {
      store
        .getState()
        .seedAgentChats([liveChat({ id: 'c1', runnerId: 'r2', pty: 'pty2', provider: 'claude' })])
    })

    const after = await screen.findByTestId('xterm')
    expect(after).toHaveAttribute('data-session-id', 'pty2')
    expect(after).toBe(before) // SAME node: the attachment swapped, the terminal did not remount
    expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c1', runnerId: 'r2' })
    expect(useTerminalStore.getState().getSession('pty2')?.connectionId).toBe('pty2')
    expect(screen.getByTestId('provider-switch').getAttribute('data-current')).toBe('claude')
  })

  it('lets go of a dead runner id when its chat goes dormant, and revives the chat', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferId = openBuffer(store, 'c1', 'r1')
    await renderPane(store, bufferId)

    await act(async () => {
      store.getState().seedAgentChats([dormantChat({ id: 'c1' })])
    })

    // The buffer must never go on pointing at a runner that no longer exists — it lets r1
    // go, and takes up the one the revive put there.
    expect(resumeChatFn).toHaveBeenCalledWith('w1', 'c1')
    expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c1', runnerId: 'r-revived' })
    expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty-revived')
  })

  // ── Resume: the MANUAL RETRY of a revive that failed ───────────────
  // The button is no longer how a dormant chat comes back — the pane does that itself.
  // It is what a user presses when the pane could not, which is why the failure state
  // is now the only state that renders it.
  describe('Resume', () => {
    it('retries the revive from the failure state and attaches', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      resumeChatFn.mockRejectedValueOnce(new Error('claude: not on PATH')) // the auto-revive
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      const bufferId = openBuffer(store, 'c1', '')
      await renderPane(store, bufferId)
      expect(screen.getByText(/could not restart this agent/i)).toBeTruthy()

      // The user installs the CLI and presses the button.
      resumeChatFn.mockResolvedValue('r9')
      getChatFn.mockResolvedValue(detail(liveChat({ id: 'c1', runnerId: 'r9', pty: 'pty9' })))
      await act(async () => {
        fireEvent.click(screen.getByTestId('pane-resume'))
      })

      expect(resumeChatFn).toHaveBeenNthCalledWith(2, 'w1', 'c1')
      const xterm = await screen.findByTestId('xterm')
      expect(xterm).toHaveAttribute('data-session-id', 'pty9')
      expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c1', runnerId: 'r9' })
      expect(useTerminalStore.getState().getSession('pty9')?.connectionId).toBe('pty9')
      err.mockRestore()
    })

    it('surfaces a toast and stays resumable when the retry cannot restart the agent', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      resumeChatFn.mockRejectedValue(new Error('claude: not on PATH'))
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openBuffer(store, 'c1', ''))

      await act(async () => {
        fireEvent.click(screen.getByTestId('pane-resume'))
      })

      const [title, why] = toastErrorFn.mock.lastCall as [string, string]
      expect(title).toMatch(/resume/i)
      expect(why).toContain('not on PATH')
      expect(screen.getByText(/could not restart this agent/i)).toBeTruthy()
      expect(screen.getByTestId('pane-resume')).toBeTruthy()
      // The auto-revive, then the click. Each attempt is a SETTLED one — nothing here
      // ever retries on its own.
      expect(resumeChatFn).toHaveBeenCalledTimes(2)
      err.mockRestore()
    })
  })

  // ── The PTY dying under an open pane ───────────────────────────────
  // The terminal runs attach-only and reports the session gone rather than spawning a
  // shell. The pane says so AT ONCE — but it does not resume from this signal, because
  // this is the CLIENT noticing and the DAEMON has not necessarily noticed yet: a resume
  // fired now can be answered with the still-recorded live runner whose PTY is the dead
  // one we are holding, and seeding that spawns the bare shell attachOnly exists to
  // prevent. The revive comes off the server's own verdict — the chat going dormant in
  // the store — one beat later.
  describe('the PTY dies under the open pane', () => {
    it('does not resume off the terminal signal alone', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))
      expect(await screen.findByTestId('xterm')).toBeTruthy()

      await act(async () => {
        fireEvent.click(screen.getByTestId('xterm')) // onSessionGone
      })

      expect(screen.queryByTestId('xterm')).toBeNull()
      expect(screen.getByText(/this agent has exited/i)).toBeTruthy()
      expect(resumeChatFn).not.toHaveBeenCalled() // the store still says the chat is live
      // The pane is still a pane: it reported the exit and offers the way back,
      // rather than tearing itself down over a signal it did not trust.
      expect(screen.getByTestId('pane-resume')).toBeTruthy()
    })

    // REGRESSION: a prompt submission REPLACES the CLI, so the outgoing PTY dies by
    // design — but the terminal reports that death whenever it notices, which can be
    // after the replacement is already attached. Believing a stale report latched
    // "this agent has exited" over a chat whose runner the server still lists as live,
    // and the React prompt queue (which may only dispatch onto a live TUI) stalled
    // there forever. Only the session the pane still WANTS may report it gone.
    it('ignores a displaced PTY reporting its death after the replacement attached', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')

      await act(async () => {
        fireEvent.doubleClick(screen.getByTestId('xterm')) // the OUTGOING pty's late death
      })

      expect(screen.getByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
      expect(screen.queryByText(/this agent has exited/i)).toBeNull()
      expect(resumeChatFn).not.toHaveBeenCalled()
    })

    it('revives once the daemon confirms the chat is dormant', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const bufferId = openBuffer(store, 'c1', 'r1')
      await renderPane(store, bufferId)
      await act(async () => {
        fireEvent.click(screen.getByTestId('xterm')) // onSessionGone
      })

      // The daemon reaps the runner and pushes the chat: NOW it is authoritatively dormant.
      await act(async () => {
        store.getState().seedAgentChats([dormantChat({ id: 'c1' })])
      })

      expect(resumeChatFn).toHaveBeenCalledTimes(1)
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty-revived')
      expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c1', runnerId: 'r-revived' })
    })
  })

  // ── Footer / provider switch ───────────────────────────────────────
  describe('provider switch', () => {
    // The switcher is the terminal surface's chrome. See landOnTerminal.
    beforeEach(landOnTerminal)

    it('renders the switcher beneath the terminal at the chat provider', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'codex' }),
      ])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      const footerControl = screen.getByTestId('provider-switch')
      expect(footerControl.getAttribute('data-current')).toBe('codex')
      expect(footerControl.getAttribute('data-count')).toBe('2')
    })

    it('is one flat surface — no card, and the switcher shares the terminal column', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      // This pane was built on CossUI's Frame first, and seeing it live is what killed
      // the idea: a Frame LIFTS a panel off its background, and a chat pane must not be
      // lifted off anything. The bordered card framed the agent's empty middle rather
      // than hiding it, and the switcher — sitting outside the card — read as a stray
      // button on the desktop. No card is allowed back in.
      expect(document.querySelector('[data-slot="frame"]')).toBeNull()
      expect(document.querySelector('[data-slot="frame-panel"]')).toBeNull()

      // The switcher and the terminal MUST be inset by the same box. Every time the
      // padding lived on one of them instead of on their shared parent, the switcher
      // drifted out of line with the agent's first character and had to be re-tuned by
      // hand (16px, then 17px, then 1px...). Their common ancestor carries the inset, so
      // the alignment cannot rot.
      const term = screen.getByTestId('xterm')
      const pill = screen.getByTestId('provider-switch')
      const column = term.closest('.max-w-4xl')
      expect(column).toBe(pill.closest('.max-w-4xl'))
      expect(column?.className).toMatch(/px-\d/)
      expect(column?.className).toMatch(/max-w-/)

      // The terminal's own 16px inset would double the column's and shove the agent out
      // of line with the switcher again — the pane opts out of it.
      expect(term.getAttribute('data-flush')).toBe('true')
    })

    it('switches the provider on the chat the runner is in NOW, not the one the tab opened on', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const bufferId = openBuffer(store, 'c1', 'r1')
      await renderPane(store, bufferId)

      // The runner /clears into c2 — the tab follows it.
      await act(async () => {
        store
          .getState()
          .seedAgentChats([
            dormantChat({ id: 'c1' }),
            liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' }),
          ])
      })

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      // The switch must target c2. Targeting c1 would hand a CLI the conversation
      // the user has already left, and leave the live one running unattended.
      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c2', 'codex')
    })

    // A SWITCH IS NOT A CHAT THAT NEEDS REVIVING. The backend kills the outgoing CLI
    // BEFORE the incoming one exists, so the chat is briefly dormant on the wire (the
    // `displaced` frame refetches it into exactly that gap) and the dead PTY reports
    // itself gone. Both are the switch working as designed. A pane that read either as
    // "dormant → revive it" would fire a resume into the middle of its own switch and
    // bring the OLD provider back.
    it('does not revive during the transient dormancy of its own switch', async () => {
      const switched = deferred<string>()
      switchProviderFn.mockReturnValue(switched.promise)

      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'claude' }),
      ])
      const bufferId = openBuffer(store, 'c1', 'r1')
      await renderPane(store, bufferId)

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch')) // → codex
      })
      // The outgoing CLI is being killed, so its terminal goes at once — the spinner is
      // what the user waits on, not a dead xterm (and there is no PTY left to report
      // itself gone).
      expect(screen.getByText(/starting codex/i)).toBeTruthy()
      expect(screen.queryByTestId('xterm')).toBeNull()

      // Mid-switch, the `displaced` frame refetches the chat into the gap: DORMANT.
      await act(async () => {
        store.getState().seedAgentChats([dormantChat({ id: 'c1' })])
      })

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.getByText(/starting codex/i)).toBeTruthy() // the spinner stands
      expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()

      // The incoming CLI lands.
      getChatFn.mockResolvedValue(
        detail(liveChat({ id: 'c1', runnerId: 'r2', pty: 'pty2', provider: 'codex' })),
      )
      await act(async () => {
        switched.resolve('r2')
      })

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty2')
      expect(buffer(store, bufferId)).toMatchObject({ chatId: 'c1', runnerId: 'r2' })
    })

    it('settles into the failure state when the incoming CLI never arrives', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      // The switch reports success, but nothing is on the chat — the CLI died on startup.
      getChatFn.mockResolvedValue(detail(dormantChat({ id: 'c1' })))
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      // Settled, not spinning — and not silently resumed either.
      expect(screen.getByText(/could not restart this agent/i)).toBeTruthy()
      expect(screen.queryByText(/starting codex/i)).not.toBeInTheDocument()
      expect(resumeChatFn).not.toHaveBeenCalled()
    })

    it('surfaces a toast when the switch rejects (target CLI missing / spawn failed)', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      // The production failure verbatim: the daemon could not find the codex binary and
      // answered 424 Failed Dependency.
      switchProviderFn.mockRejectedValue(new ApiError('terminal: command not found: codex', 424))
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(toastErrorFn).toHaveBeenCalledTimes(1)
      const [title, description] = toastErrorFn.mock.calls[0] as [string, string]
      expect(title).toContain('Codex') // the target provider's display name
      expect(title).toMatch(/isn.t installed/)
      expect(description).toMatch(/PATH/)
      expect(err).toHaveBeenCalled()
      err.mockRestore()
    })

    it('shows no toast when the switch succeeds', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c1', 'codex')
      expect(toastErrorFn).not.toHaveBeenCalled()
    })

    // Regression: a switch never toggles `presentation` itself (see the test
    // above — it stays on whatever surface the user was already looking at),
    // and switchProvider/adopt alone never re-request an attach. A chat already
    // ON the terminal surface that switches from a hotswap provider (claude) to
    // a non-hotswap one (codex) used to strand the view on the OLD provider's
    // live PTY label with the new runner's empty attachment underneath it —
    // "This agent has no terminal view attached right now" — because nothing
    // ever asked codex for one.
    it('re-requests an attach when switching TO a non-hotswap provider while already on the terminal surface', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'claude' }),
      ])
      store.getState().setAgentProviders([providers[0], { ...providers[1], hotswap: false }])
      switchToTerminalFn.mockResolvedValue('pty-attached')
      getChatFn
        .mockResolvedValueOnce(
          detail(liveChatNoTerminal({ id: 'c1', runnerId: 'r1', title: 'Chat c1' })),
        )
        .mockResolvedValueOnce(
          detail(liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty-attached', provider: 'codex' })),
        )
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      await vi.waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1'))
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty-attached')
      expect(screen.queryByText(/no terminal view attached/i)).not.toBeInTheDocument()
    })
  })

  // ── Tab title ──────────────────────────────────────────────────────
  // openContent snapshots the label at open time; the chat's title changes later
  // (the agent auto-titles it over WS `title_set`, or the user renames it) and both
  // land on the store chat's `title`. The pane mirrors title → buffer name.
  describe('tab title tracks the chat title', () => {
    it('relabels the tab when the chat is titled', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', title: 'Codex chat' }),
      ])
      const bufferId = openBuffer(store, 'c1', 'r1', 'Codex chat')
      await renderPane(store, bufferId)

      await act(async () => {
        store
          .getState()
          .seedAgentChats([
            liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', title: 'Fix the flaky test' }),
          ])
      })

      expect(buffer(store, bufferId)?.name).toBe('Fix the flaky test')
    })

    it('never blanks the tab when the chat title is empty', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', title: '' })])
      const bufferId = openBuffer(store, 'c1', 'r1', 'Codex chat')
      await renderPane(store, bufferId)

      expect(buffer(store, bufferId)?.name).toBe('Codex chat')
    })
  })

  // ── ⌘/ toggles the chat/terminal view, like the ViewSwitcher tabs ─────
  describe('toggle chat/terminal view chord', () => {
    const pressToggle = async () => {
      await act(async () => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key: '/', metaKey: true }))
      })
    }

    it('flips a hotswap provider straight from Chat to Terminal', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const bufferId = openBuffer(store, 'c1', 'r1')
      await renderPane(store, bufferId)

      expect(screen.getByRole('tab', { name: /^chat$/i })).toHaveAttribute('aria-selected', 'true')

      await pressToggle()

      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'true',
      )
    })

    it('flips back from Terminal to Chat on a second press', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const bufferId = openBuffer(store, 'c1', 'r1')
      await renderPane(store, bufferId)

      await pressToggle()
      await pressToggle()

      expect(screen.getByRole('tab', { name: /^chat$/i })).toHaveAttribute('aria-selected', 'true')
    })

    it('still fires when the focused child swallows the key (xterm stopPropagation)', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const bufferId = openBuffer(store, 'c1', 'r1')
      await renderPane(store, bufferId)

      // With a chat open the focus sits in its xterm, which stopPropagations the
      // keys it handles. A bubble-phase listener never sees the chord in the one
      // place the command exists to work — this stands that swallower in, so the
      // test fails if the listener ever goes back to the bubble phase.
      const swallow = (e: Event) => e.stopPropagation()
      document.body.addEventListener('keydown', swallow)
      try {
        await act(async () => {
          document.body.dispatchEvent(
            new KeyboardEvent('keydown', {
              key: '/',
              metaKey: true,
              bubbles: true,
              cancelable: true,
            }),
          )
        })
      } finally {
        document.body.removeEventListener('keydown', swallow)
      }

      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'true',
      )
    })

    // The isVisible/isActivePane gates are read from the pane's OWN workspace
    // store, and a workspace switch changes neither: WorkspaceHost keeps the
    // outgoing workspace MOUNTED (display:none + inert), which hides DOM but
    // does not unregister a window-level keydown listener. So N retained
    // workspaces could each satisfy the guard at once — ⌘/ pressed in workspace
    // B was swallowed by A's hidden listener, which killed B's Monaco comment
    // toggle AND flipped the view on an invisible chat.
    //
    // A non-hotswap provider (codex) here, not the fixture default — the hotswap
    // branch is a synchronous local setState with nothing to assert but the DOM,
    // which two panes sharing one document can't cleanly scope apart. switchToTerminal
    // is a real call carrying the ids of exactly which chat asked for it, so it
    // survives a second pane sharing the page.
    it('a HIDDEN WORKSPACE ignores the chord — only the ACTIVE workspace may switch', async () => {
      switchToTerminalFn.mockResolvedValue('pty-attached')
      const hidden = seedWorkspace(
        [liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'codex' })],
        'w-hidden',
      )
      hidden.getState().setAgentProviders([providers[0], { ...providers[1], hotswap: false }])
      const hiddenBuffer = openBuffer(hidden, 'c1', 'r1', 'Chat', 'w-hidden')
      const shown = seedWorkspace(
        [liveChat({ id: 'c2', runnerId: 'r2', pty: 'pty2', provider: 'codex' })],
        'w-shown',
      )
      shown.getState().setAgentProviders([providers[0], { ...providers[1], hotswap: false }])
      const shownBuffer = openBuffer(shown, 'c2', 'r2', 'Chat', 'w-shown')

      // Both panes are the active, visible tab of their own workspace — exactly
      // what a retained workspace looks like the instant it goes hidden.
      await renderPane(hidden, hiddenBuffer)
      await renderPane(shown, shownBuffer)
      setActiveWorkspaceId('w-shown')

      await pressToggle()

      await vi.waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledTimes(1))
      expect(switchToTerminalFn).toHaveBeenCalledWith('w-shown', 'c2')
    })

    it('a HIDDEN chat ignores the chord — a background split must not switch', async () => {
      switchToTerminalFn.mockResolvedValue('pty-attached')
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'codex' }),
      ])
      store.getState().setAgentProviders([providers[0], { ...providers[1], hotswap: false }])
      const bufferId = openBuffer(store, 'c1', 'r1')
      // Every chat stays mounted for keep-alive, so without the isVisible gate a
      // hidden tab would swallow the chord and flip the view on a chat nobody can see.
      await act(async () => {
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(PaneHost, { bufferId, isVisible: false }),
          ),
        )
      })

      await pressToggle()

      expect(switchToTerminalFn).not.toHaveBeenCalled()
    })
  })

  describe('React chat presentation', () => {
    it('defaults to Chat while retaining the native terminal as an attach-only fallback', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      expect(screen.getByRole('tab', { name: /^chat$/i })).toHaveAttribute('aria-selected', 'true')
      expect(screen.getByRole('textbox', { name: /message the agent/i })).toBeInTheDocument()
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-attach-only', 'true')

      fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i }))
      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'true',
      )
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-visible', 'true')
    })

    it('pauses a busy-chat FIFO in Terminal and resumes only after Return to Chat', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      store.getState().setAgentChatWorking('c1', true)
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      const input = screen.getByRole('textbox', { name: /message the agent/i })
      fireEvent.change(input, { target: { value: 'queued while busy' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(await screen.findByText('queued while busy')).toBeInTheDocument()

      fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i }))
      expect(screen.getByText(/1 prompt pending in Chat/i)).toBeInTheDocument()
      await act(async () => store.getState().setAgentChatWorking('c1', false))
      expect(submitPromptFn).not.toHaveBeenCalled()

      fireEvent.click(screen.getByRole('button', { name: /return to chat/i }))
      await vi.waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
    })

    // The dropdown lives on the terminal's status strip, and it must keep
    // refusing a switch across both the submission window and the hook
    // confirmation that follows it — the window a handover would lose the turn in.
    it('blocks dropdown provider switches through submission and hook confirmation', async () => {
      const submitted = deferred<{ runnerId: string; terminalSessionId: string }>()
      submitPromptFn.mockReturnValue(submitted.promise)
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'claude' }),
      ])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      const input = screen.getByRole('textbox', { name: /message the agent/i })
      fireEvent.change(input, { target: { value: 'deliver exactly once' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      await vi.waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))

      // Chat has no dropdown to press yet — cross to the provider's own view,
      // where the dropdown is, and find it refusing.
      expect(screen.queryByTestId('provider-switch')).toBeNull()
      await act(async () => {
        fireEvent.click(screen.getByLabelText('Terminal'))
      })
      expect(screen.getByTestId('provider-switch')).toBeDisabled()
      fireEvent.click(screen.getByTestId('provider-switch'))
      expect(switchProviderFn).not.toHaveBeenCalled()

      await act(async () => {
        submitted.resolve({ runnerId: 'r-prompt', terminalSessionId: 'pty-prompt' })
      })
      // Still refusing after the submission resolves: the prompt is delivered but
      // the hook has not confirmed it, and that window is exactly the one a
      // handover would lose the turn in.
      expect(screen.getByTestId('provider-switch')).toBeDisabled()
      fireEvent.click(screen.getByTestId('provider-switch'))
      expect(switchProviderFn).not.toHaveBeenCalled()
    })

    it('reconciles a failed replacement to dormant instead of retaining a dead PTY attachment', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      submitPromptFn.mockRejectedValue(new ApiError('replacement failed', 500))
      getChatFn.mockResolvedValue(detail(dormantChat({ id: 'c1' })))
      await renderPane(store, openBuffer(store, 'c1', 'r1'))

      const input = screen.getByRole('textbox', { name: /message the agent/i })
      fireEvent.change(input, { target: { value: 'trigger replacement' } })
      fireEvent.keyDown(input, { key: 'Enter' })

      expect(await screen.findByText(/could not restart this agent/i)).toBeInTheDocument()
      expect(screen.queryByTestId('xterm')).not.toBeInTheDocument()
      expect(screen.getByText(/replacement failed/i)).toBeInTheDocument()
    })
  })

  // Regression: `AgentTerminalWaitBanner` (a pane-level overlay) and the
  // composer's own `signpost` (reason: 'terminal_wait', via `resolveComposerState`)
  // both render off the SAME `waiting` signal — but only the banner is meant to
  // survive once the composer can actually show it. Before this test existed, both
  // rendered at once for any chat with messages, which is exactly the duplication
  // "one box, one occupant, never two stacked" (composer-state.ts) already forbids
  // for every OTHER reason. The banner earns its keep only for a blank, first-turn
  // chat, where AgentChatView renders AgentEmptyDocument instead of AgentComposer
  // and there is no composer slot to mutate — see agent-chat-pane-terminal-wait.test.tsx
  // for that half.
  describe('terminal_wait on a chat that already has messages', () => {
    it('mutates the composer into a signpost instead of duplicating a pane-level banner', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'claude' }),
      ])
      await renderPane(store, openBuffer(store, 'c1', 'r1'))
      // The default beforeEach's listMessagesFn returns one message, so this chat
      // is NOT blank — AgentComposer, not AgentEmptyDocument, is mounted.
      expect(screen.getByRole('textbox', { name: /message the agent/i })).toBeInTheDocument()

      await act(async () => {
        store.getState().setAgentChatTerminalWait('c1', { kind: 'workspace_trust' })
      })

      // The composer itself became the signpost...
      expect(screen.getByText(/waiting for you to trust the workspace/i)).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Terminal' })).toBeInTheDocument()
      // ...and the separate pane-level banner did NOT also render.
      expect(screen.queryByTestId('agent-terminal-wait')).not.toBeInTheDocument()
      // The input itself is gone — one occupant, not an input rendered dead
      // beneath the question.
      expect(screen.queryByRole('textbox', { name: /message the agent/i })).not.toBeInTheDocument()
    })
  })
})
