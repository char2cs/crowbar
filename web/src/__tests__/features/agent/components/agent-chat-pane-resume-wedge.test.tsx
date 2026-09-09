import { StrictMode, createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from 'zustand'
import type { AgentChat, AgentChatDetail, AgentProvider } from '@/features/agent/api/agent-api'
import type { AgentChatContent } from '@/features/panes/types/pane-content'
import {
  WorkspaceStoreContext,
  useWorkspaceStore,
} from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

const {
  getChatFn,
  resumeChatFn,
  listMessagesFn,
  submitPromptFn,
  slashCatalogFn,
  saveReconnectFn,
  toastErrorFn,
  switchToTerminalFn,
  switchToNativeFn,
  switchProviderFn,
} = vi.hoisted(() => ({
  getChatFn: vi.fn(),
  resumeChatFn: vi.fn(),
  listMessagesFn: vi.fn(),
  submitPromptFn: vi.fn(),
  slashCatalogFn: vi.fn(),
  saveReconnectFn: vi.fn(),
  toastErrorFn: vi.fn(),
  switchToTerminalFn: vi.fn(),
  switchToNativeFn: vi.fn(),
  switchProviderFn: vi.fn(),
}))

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

vi.mock('@/features/terminal/components/terminal', () => ({
  XtermTerminal: ({ sessionId }: { sessionId: string }) =>
    createElement('div', { 'data-testid': 'xterm', 'data-session-id': sessionId }),
}))

vi.mock('@/features/agent/composer/plate/chat-markdown-editor', () => ({
  ChatMarkdownEditor: ({ ariaLabel, placeholder }: { ariaLabel: string; placeholder: string }) =>
    createElement('textarea', { 'aria-label': ariaLabel, placeholder }),
}))

vi.mock('@/features/agent/components/provider-switch-dropdown', () => ({
  ProviderSwitchDropdown: () => createElement('button', { 'data-testid': 'provider-switch' }),
}))

import { AgentChatPane } from '@/features/agent/components/agent-chat-pane'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useSettingsStore } from '@/features/settings/store'

const providers: AgentProvider[] = [
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

function liveChat(o: { id: string; runnerId: string; pty: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: `Chat ${o.id}`,
    liveRunnerId: o.runnerId,
    terminalSessionId: o.pty,
    activeProviderId: 'codex',
    createdAt: '',
    order: 0,
  }
}

function dormantChat(o: { id: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: `Chat ${o.id}`,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: 'codex',
    createdAt: '',
    order: 0,
  }
}

function detail(chat: AgentChat): AgentChatDetail {
  return { ...chat, conversations: [] }
}

/**
 * A resume the daemon accepts and then never answers — the shape the wedge was
 * actually made of.
 *
 * It rejects on abort, exactly as `fetch` does, and ONLY on abort. That is what
 * makes this a real test of the pane rather than of the mock: if the pane stops
 * handing the request a signal, nothing here ever settles and the assertions below
 * fail rather than quietly passing on a promise that resolved some other way.
 */
function neverAnswers() {
  return vi.fn((_wsId: unknown, _id: unknown, signal?: AbortSignal) => {
    if (signal?.aborted) return Promise.reject(new DOMException('Aborted', 'AbortError'))
    return new Promise<string>((_resolve, reject) => {
      signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')))
    })
  })
}

function seedWorkspace(chats: AgentChat[], wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  store.getState().setAgentProviders(providers)
  store.getState().seedAgentChats(chats)
  return store
}

type Store = ReturnType<typeof seedWorkspace>

function openBuffer(store: Store, chatId: string, runnerId: string) {
  return store
    .getState()
    .bufferActions.openContent({ type: 'agentChat', chatId, wsId: 'w1', name: 'Chat', runnerId })
}

function PaneHost({ bufferId }: { bufferId: string }) {
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
    isVisible: true,
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

beforeEach(() => {
  for (const f of [
    getChatFn,
    resumeChatFn,
    listMessagesFn,
    submitPromptFn,
    slashCatalogFn,
    saveReconnectFn,
    toastErrorFn,
    switchToTerminalFn,
    switchToNativeFn,
    switchProviderFn,
  ])
    f.mockReset()
  resumeChatFn.mockResolvedValue('r-revived')
  switchProviderFn.mockResolvedValue('r-new')
  switchToTerminalFn.mockResolvedValue('pty-native')
  switchToNativeFn.mockResolvedValue(undefined)
  listMessagesFn.mockResolvedValue({
    cursor: 1,
    oldestCursor: 1,
    hasMore: false,
    items: [
      {
        sequence: 1,
        turnId: 'turn-1',
        role: 'assistant',
        providerId: 'codex',
        text: 'earlier turn',
        at: '2026-08-16T00:00:01Z',
      },
    ],
  })
  submitPromptFn.mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
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
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
  localStorage.clear()
  setActiveWorkspaceId('w1')
})

// ── A resume that never comes back ────────────────────────────────────────
//
// THE BUG, as the user hit it: a chat pinned on "Resuming this chat…" for good,
// with no Resume button and no other way out — the chat had to be abandoned.
//
// `reviving` is the one state in this machine that renders no control at all (see
// ComposerSignpost: a spinner, and nothing to click), and the attach effect refuses
// to overwrite it because a revive might still be coming. Both halves are right on
// their own; together they assume every revive ends. One did not. The daemon
// serialises a chat's spawn paths behind a plain per-chat mutex with no context on
// it — and a stale `working` flag on a dormant chat made `switchProviderLocked` loop
// forever holding it (see TestRegression_DisplaceForSwitch_… on the backend) — while
// `apiFetch` set no deadline on the request. So `resumeChat` sat there, and `revive`'s
// `catch`, which holds the `fail()` that puts Resume on screen, was never reached.
//
// Nothing here waits on a clock: the request settles because the pane ABORTS it, and
// the assertions are on what that abort renders.
describe('AgentChatPane: a resume the daemon never answers', () => {
  it(
    'settles into the resumable state instead of pinning the spinner for good',
    { timeout: 60_000 },
    async () => {
      vi.useFakeTimers()
      const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      try {
        resumeChatFn.mockImplementation(neverAnswers())
        const store = seedWorkspace([dormantChat({ id: 'c1' })])
        await renderPane(store, openBuffer(store, 'c1', ''))

        // The spinner is CORRECT here — the request really is out — and it is
        // deliberately buttonless. That is only safe because it now ends.
        expect(screen.getByText(/resuming this chat/i)).toBeTruthy()
        expect(screen.queryByTestId('pane-resume')).toBeNull()
        expect(resumeChatFn).toHaveBeenCalledTimes(1)

        // The daemon re-states the chat as dormant, over and over, exactly as it did
        // live. On the old code this changed nothing: the effect read the spinner as
        // proof a revive was still coming and left it alone, for ever.
        for (const _ of [1, 2, 3]) {
          await act(async () => {
            store.getState().seedAgentChats([dormantChat({ id: 'c1' })])
          })
        }
        expect(screen.getByText(/resuming this chat/i)).toBeTruthy()

        // The pane gives up on the request — and giving up is an OUTCOME, so it lands
        // in the same place every other refused resume does.
        await act(async () => {
          await vi.advanceTimersByTimeAsync(120_000)
        })

        expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()
        expect(screen.getByText(/could not restart this agent/i)).toBeTruthy()
        const resumeButton = screen.getByTestId('pane-resume')
        expect(resumeButton).toBeTruthy()

        // And the way out actually works: the button starts a fresh attempt rather
        // than re-rendering the same dead end.
        resumeChatFn.mockResolvedValue('r-revived')
        await act(async () => {
          fireEvent.click(resumeButton)
        })
        expect(resumeChatFn).toHaveBeenCalledTimes(2)
        await act(async () => {})
        expect(screen.getByTestId('xterm')).toHaveAttribute('data-session-id', 'pty-revived')
      } finally {
        errSpy.mockRestore()
        vi.useRealTimers()
      }
    },
  )

  it(
    'hands the resume a signal, so the request itself is what ends',
    { timeout: 60_000 },
    async () => {
      vi.useFakeTimers()
      const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      try {
        resumeChatFn.mockImplementation(neverAnswers())
        const store = seedWorkspace([dormantChat({ id: 'c1' })])
        await renderPane(store, openBuffer(store, 'c1', ''))

        // Bounding the UI while leaving the request running would leak a resume per
        // wedge and let a late answer attach a runner the pane has already disowned.
        // The abort is the point, so it is asserted rather than inferred.
        const signal = resumeChatFn.mock.calls[0]?.[2] as AbortSignal | undefined
        expect(signal).toBeInstanceOf(AbortSignal)
        expect(signal?.aborted).toBe(false)

        await act(async () => {
          await vi.advanceTimersByTimeAsync(120_000)
        })
        expect(signal?.aborted).toBe(true)
      } finally {
        errSpy.mockRestore()
        vi.useRealTimers()
      }
    },
  )

  // REGRESSION: resumeChat's own request was bounded, but adopt()'s
  // getChat() call right after it — the one line that actually reads the
  // revived chat back and puts a live PTY on screen — had no signal of its
  // own. A resumeChat that answers fine but an adopt() that then hangs
  // reproduced the exact same "Resuming this chat…" wedge, one call later,
  // inside the fix meant to eliminate it.
  it(
    'settles into the resumable state when resumeChat answers but the FOLLOW-UP adopt() read hangs',
    { timeout: 60_000 },
    async () => {
      vi.useFakeTimers()
      const errSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
      try {
        resumeChatFn.mockResolvedValue('r-revived') // this half answers fine
        getChatFn.mockImplementation(neverAnswers()) // adopt()'s own read never does
        const store = seedWorkspace([dormantChat({ id: 'c1' })])
        await renderPane(store, openBuffer(store, 'c1', ''))

        expect(screen.getByText(/resuming this chat/i)).toBeTruthy()
        expect(screen.queryByTestId('pane-resume')).toBeNull()

        // The SAME bound that covers resumeChat has to also end adopt()'s
        // own hung read — not a second, unbounded wait stacked on top of it.
        await act(async () => {
          await vi.advanceTimersByTimeAsync(120_000)
        })

        expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()
        expect(screen.getByTestId('pane-resume')).toBeTruthy()
      } finally {
        errSpy.mockRestore()
        vi.useRealTimers()
      }
    },
  )

  // REGRESSION: the effect that fires revive() automatically had no cleanup
  // function at all — a pane that genuinely unmounts mid-resume (its
  // buffer/tab closing) left the request running for up to the full bound,
  // still holding the daemon's per-chat spawn-gate mutex, with nothing left
  // on screen to show for it.
  it('aborts the in-flight auto-revive as soon as the pane unmounts, not 120s later', async () => {
    resumeChatFn.mockImplementation(neverAnswers())
    const store = seedWorkspace([dormantChat({ id: 'c1' })])
    const bufferId = openBuffer(store, 'c1', '')
    let unmount!: () => void
    await act(async () => {
      const result = render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(PaneHost, { bufferId }),
        ),
      )
      unmount = result.unmount
    })

    expect(resumeChatFn).toHaveBeenCalledTimes(1)
    const signal = resumeChatFn.mock.calls[0]?.[2] as AbortSignal | undefined
    expect(signal).toBeInstanceOf(AbortSignal)
    expect(signal?.aborted).toBe(false)

    act(() => unmount())

    expect(signal?.aborted).toBe(true)
  })

  it('never gives up on a resume that is answering normally', { timeout: 60_000 }, async () => {
    vi.useFakeTimers()
    try {
      // The guard against the fix becoming its own bug: a resume that lands well
      // inside the bound must be untouched by it, spinner and all.
      let settle!: (id: string) => void
      resumeChatFn.mockImplementation(
        () =>
          new Promise<string>((resolve) => {
            settle = resolve
          }),
      )
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openBuffer(store, 'c1', ''))

      await act(async () => {
        await vi.advanceTimersByTimeAsync(119_000)
      })
      expect(screen.getByText(/resuming this chat/i)).toBeTruthy()
      expect(screen.queryByText(/could not restart this agent/i)).not.toBeInTheDocument()

      await act(async () => {
        settle('r-revived')
      })
      await act(async () => {})
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-session-id', 'pty-revived')

      // The bound must not fire retroactively on a revive that already landed.
      await act(async () => {
        await vi.advanceTimersByTimeAsync(120_000)
      })
      expect(screen.getByTestId('xterm')).toBeTruthy()
      expect(screen.queryByText(/could not restart this agent/i)).not.toBeInTheDocument()
    } finally {
      vi.useRealTimers()
    }
  })
})

// ── A resume that answers must not be reported as one that didn't ─────────
//
// REGRESSION, live-reproduced against a real claude chat (make dev-desktop):
// reopening a dormant chat from the sidebar showed "Couldn't resume Claude
// chat: The daemon did not answer the resume" — while the daemon's own access
// log recorded that exact resume request answering 200 in under 60ms, and the
// chat's transcript and composer came back correct and live a moment later.
//
// Root cause: the auto-revive effect's cleanup (`() => controller.abort()`,
// see the effect above `if (sessionId) seedAttach(...)`) shares ONE
// AbortController with revive()'s own request. That cleanup fires not only on
// a genuine unmount, but on every change to the effect's own dependencies —
// including `liveRunnerId`, which revive()'s own adopt() call writes into the
// store the instant resumeChat succeeds. React StrictMode's dev-only
// mount→cleanup→mount double-invoke (this codebase's own established way of
// catching exactly this class of bug — see dnd-scope.test.tsx) exercises the
// identical path on literally the first mount of every pane: the thrown-away
// first invocation's revive() is aborted by the simulated cleanup before its
// request can resolve, and the catch block reported that abort as "the
// daemon did not answer" — a false claim about a request the daemon in fact
// answered fine a moment later, over a connection nobody was using any more.
//
// This fix only closes THAT false claim (no toast, no `failed` banner) for an
// externally-torn-down attempt. It does not touch attemptedRef's own,
// separately-reasoned "revived exactly once, ever, per chat per mount"
// invariant a few lines below — so under this exact StrictMode double-invoke,
// the thrown-away first attempt still spends that one-shot budget, and this
// pane's own retry machinery will not fire a second resumeChat call on its
// own. Live, the chat came back correctly anyway (this file's own commit
// message has the daemon log to show it), which means something OUTSIDE this
// effect — most likely a live push once the daemon's hooks report the runner
// — is what actually delivers the attach in that case. This test does not
// model that channel, so it asserts only what this fix actually guarantees:
// no false failure surfaces over an attempt nobody refused.
describe('AgentChatPane: a resume that answers must not report the wrong outcome', () => {
  it('does not toast a resume failure when StrictMode double-invokes the auto-revive effect', async () => {
    // Mimics apiFetch's real behaviour, unlike a bare mockResolvedValue():
    // rejects at once if already aborted, otherwise resolves on the next
    // tick, rejecting instead if aborted before that tick fires. A mock that
    // never looks at the signal could never fail this test either way.
    resumeChatFn.mockImplementation(
      (_wsId: unknown, _id: unknown, signal?: AbortSignal) =>
        new Promise<string>((resolve, reject) => {
          if (signal?.aborted) {
            reject(new DOMException('Aborted', 'AbortError'))
            return
          }
          const timer = setTimeout(() => resolve('r-revived'), 0)
          signal?.addEventListener('abort', () => {
            clearTimeout(timer)
            reject(new DOMException('Aborted', 'AbortError'))
          })
        }),
    )
    const store = seedWorkspace([dormantChat({ id: 'c1' })])
    const bufferId = openBuffer(store, 'c1', '')

    await act(async () => {
      render(
        createElement(
          StrictMode,
          null,
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(PaneHost, { bufferId }),
          ),
        ),
      )
    })
    // Let the surviving (second) invocation's own request settle, then flush
    // the state update it lands — two real ticks, matching this file's own
    // pattern elsewhere for a resolved-mid-flight revive.
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 10))
    })
    await act(async () => {})

    // The toast this regression is about, and the visible state that would
    // carry the SAME false claim (StrictMode's thrown-away first invocation
    // still marks the chat's one-shot revive budget spent — see attemptedRef
    // — so nothing here retries the request a second time; that budget is a
    // separate, deliberately-scoped invariant this fix does not touch).
    // Neither the toast nor a "could not restart" banner may appear over an
    // attempt that was torn down by our own caller, not refused by the daemon.
    expect(toastErrorFn).not.toHaveBeenCalled()
    expect(screen.queryByText(/could not restart this agent/i)).not.toBeInTheDocument()
  })
})
