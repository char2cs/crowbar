import { createElement } from 'react'
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from 'zustand'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { ApiError } from '@/lib/errors/api-error'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { seedChatPaneRecord } from '@/__tests__/__fixtures__/view-state'
import { chatStore } from '@/features/agent/stores/chat-store'
import { ledger, seedFeed, spokenIn } from '@/__tests__/__fixtures__/ledger-rows'
import { heldScopes } from '@/__tests__/__fixtures__/stream-daemon'
import { resetChatPresentationMemoryForTests } from '@/features/agent/hooks/use-chat-presentation'
import { nanoid } from 'nanoid'

// Hoisted fakes — declared before the vi.mock calls that reference them.
const {
  switchProviderFn,
  resumeChatFn,
  submitPromptFn,
  slashCatalogFn,
  toastErrorFn,
  switchToTerminalFn,
  switchToNativeFn,
} = vi.hoisted(() => ({
  switchProviderFn: vi.fn(),
  resumeChatFn: vi.fn(),
  submitPromptFn: vi.fn(),
  slashCatalogFn: vi.fn(),
  toastErrorFn: vi.fn(),
  switchToTerminalFn: vi.fn(),
  switchToNativeFn: vi.fn(),
}))

// The pane resolves its toggle-view chord through the keymap (so it stays
// rebindable); pin it here rather than standing up the settings store.
// 'agent.cycleProvider' is deliberately absent — it now ships unbound by
// default, and an absent key resolves the same way (falsy) as its real ''.
import * as useEffectiveKeymapMod from '@/features/keymaps/hooks/use-effective-keymap'
vi.spyOn(useEffectiveKeymapMod, 'useEffectiveChordMap').mockImplementation(() => ({
  'agent.toggleViewMode': 'mod+/',
}))

// The toggle-chord tests below dispatch metaKey (Cmd) keydowns, and
// HEADER_ROW_HEIGHT_PX is platform-dependent — both resolve through IS_MAC
// (chord.ts's eventMatchesChord checks metaKey on macOS, ctrlKey elsewhere).
// Force macOS regardless of whatever OS the test happens to run on (jsdom's
// UA bakes in the CI runner's own host platform, which is Linux on GitHub
// Actions).
vi.mock('@/utils/platform', () => ({ IS_MAC: true }))

import * as readsApi from '@/features/agent/api/agent-chat-reads'
vi.spyOn(readsApi, 'getSlashCatalog').mockImplementation(slashCatalogFn)
import * as commandsApi from '@/features/agent/api/agent-chat-commands'
vi.spyOn(commandsApi, 'switchProvider').mockImplementation(switchProviderFn)
vi.spyOn(commandsApi, 'resumeChat').mockImplementation(resumeChatFn)
vi.spyOn(commandsApi, 'submitAgentPrompt').mockImplementation(submitPromptFn)
vi.spyOn(commandsApi, 'switchToTerminal').mockImplementation(switchToTerminalFn)
vi.spyOn(commandsApi, 'switchToNative').mockImplementation(switchToNativeFn)

import * as toastStoreMod from '@/features/window/stores/toast-store'
vi.spyOn(toastStoreMod.toast, 'error').mockImplementation((...a: unknown[]) => toastErrorFn(...a))

// jsdom can't run xterm/WebGL — stub the terminal renderer to a passive marker
// that records the sessionId it was mounted with (that's what the attach seam is
// proven by) plus the isActive/isVisible/attachOnly props threaded from the pane.
import * as lazyTerminalMod from '@/features/terminal/components/lazy-terminal'
vi.spyOn(lazyTerminalMod, 'LazyXtermTerminal').mockImplementation(
  ({
    sessionId,
    isActive,
    isVisible,
    attachOnly,
    flush,
  }: {
    sessionId: string
    isActive: boolean
    isVisible?: boolean
    attachOnly?: boolean
    flush?: boolean
  }) =>
    createElement('div', {
      'data-testid': 'xterm',
      'data-session-id': sessionId,
      'data-active': String(isActive),
      'data-visible': String(isVisible),
      'data-attach-only': String(Boolean(attachOnly)),
      'data-flush': String(Boolean(flush)),
    }),
)

// These suites are about the queue, the catalog and the ledger: the editor is a
// textarea with its contract, tested for real in its own suite and the view's.
import * as chatMarkdownEditorMod from '@/features/agent/composer/plate/chat-markdown-editor'
import { TextareaComposerEditor } from '@/__tests__/__fixtures__/composer-editor'
vi.spyOn(chatMarkdownEditorMod, 'ChatMarkdownEditor').mockImplementation(TextareaComposerEditor)

// Stub the dropdown to expose its props and a one-click switch, so the footer
// wiring is asserted without the shared Dropdown's framer-motion machinery.
import * as providerSwitchDropdownMod from '@/features/agent/components/provider-switch-dropdown'
vi.spyOn(providerSwitchDropdownMod, 'ProviderSwitchDropdown').mockImplementation(
  ({
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
)

import { AgentChatPane } from '@/features/agent/components/agent-chat-pane'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { useSettingsStore } from '@/features/settings/store'
import {
  nextVersion,
  seedChats,
  setChatTerminalWait,
  setChatWorking,
  writeChat,
  resetChatStore,
  setProviders,
} from '@/__tests__/__fixtures__/agent-chat'

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
    modelSelect: false,
    effortSelect: false,
    compaction: false,
    terminalStartHere: false,
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
    modelSelect: false,
    effortSelect: false,
    compaction: false,
    terminalStartHere: false,
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
  /** The chat's own durable landing surface (design spec 2.5). Omitted is the
   *  provider's default face, which is every fixture here but the terminal-born
   *  one below. */
  surface?: 'chat' | 'terminal'
}): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: o.title ?? `Chat ${o.id}`,
    liveRunnerId: o.runnerId,
    terminalSessionId: o.pty,
    surface: o.surface,
    activeProviderId: o.provider ?? 'codex',
    working: false,
    version: nextVersion(),
    phase: 'dormant',
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
    working: false,
    version: nextVersion(),
    phase: 'dormant',
    createdAt: '',
    order: 0,
  }
}

/** A chat the daemon is placing a CLI on (a revive or a switch in flight). */
function startingChat(o: { id: string }): AgentChat {
  return { ...dormantChat(o), phase: 'starting' }
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
    working: false,
    version: nextVersion(),
    phase: 'dormant',
    createdAt: '',
    order: 0,
  }
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
// PaneHost does what pane-container.tsx does: read the window-level `PaneGroup`
// and feed its chatId/runnerId back in, so every repoint assertion runs the
// same `setPaneChat` write production does.

// A real `PaneGroup` carries no workspace id (pane-container reads it from the
// ambient WorkspaceStoreContext), so the harness keeps it beside the pane
// rather than inventing a field production does not have.
const paneWorkspace = new Map<string, string>()

function seedWorkspace(chats: AgentChat[], wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  setProviders(providers)
  seedChats(chats)
  return store
}

/** A workspace whose chat list has NOT arrived yet — no seed has run. Distinct
 *  from seeding an empty list, which is the daemon answering "there are none". */
function unseededWorkspace(wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  setProviders(providers)
  return store
}

type Store = ReturnType<typeof seedWorkspace>

// `_name` is vestigial — it was the fake buffer's tab label, and a chat has no
// buffer to label any more (ChatHead reads the title straight off the store).
// Kept in the signature so the ~50 call sites below stay unchanged.
function openChatPane(
  _store: Store,
  chatId: string,
  runnerId: string,
  _name = 'Chat',
  wsId = 'w1',
) {
  const id = nanoid()
  seedChatPaneRecord(windowPaneStore, id, chatId, runnerId || null)
  paneWorkspace.set(id, wsId)
  return id
}

function PaneHost({ paneId, isVisible = true }: { paneId: string; isVisible?: boolean }) {
  const group = useStore(windowPaneStore, (s) => s.panes[paneId])
  if (!group) return null
  return createElement(AgentChatPane, {
    chatId: group.chatId ?? '',
    runnerId: group.runnerId ?? '',
    wsId: paneWorkspace.get(paneId) ?? 'w1',
    paneId: group.id,
    isActivePane: true,
    // Default true: the vast majority of these tests are the ACTIVE, visible pane.
    isVisible,
  })
}

async function renderPane(store: Store, paneId: string) {
  await act(async () => {
    render(
      createElement(
        WorkspaceStoreContext.Provider,
        { value: store },
        createElement(PaneHost, { paneId }),
      ),
    )
  })
}

function typeAndSend(text: string) {
  const input = screen.getByRole('textbox', { name: /message the agent/i })
  fireEvent.change(input, { target: { value: text } })
  fireEvent.keyDown(input, { key: 'Enter' })
}

const paneOf = (_store: Store, id: string) => windowPaneStore.getState().panes[id]

// The default backend is a HEALTHY one: a resume brings the chat's CLI back, and reading
// the chat back afterwards shows the runner now on it. Tests that are about failure say
// so explicitly by overriding these — nothing else has to opt in to "it worked".
beforeEach(() => {
  resetWindowPaneStoreForTests()
  resetChatStore()
  resetChatPresentationMemoryForTests()
  paneWorkspace.clear()
  switchProviderFn.mockReset()
  resumeChatFn.mockReset()
  submitPromptFn.mockReset()
  slashCatalogFn.mockReset()
  toastErrorFn.mockReset()
  switchToTerminalFn.mockReset()
  switchToNativeFn.mockReset()
  switchProviderFn.mockResolvedValue('r-new')
  resumeChatFn.mockResolvedValue('r-revived')
  // A chat that has been SPOKEN IN. A chat with no messages is the blank
  // DOCUMENT surface — writing size, no pill under it — so a pane test about the
  // composer, the queue or the switcher has to start from a conversation for its
  // subject to be on screen at all.
  seedFeed('c1', spokenIn('c1'))
  submitPromptFn.mockResolvedValue({ runnerId: 'r-prompt', terminalSessionId: 'pty-prompt' })
  slashCatalogFn.mockResolvedValue({
    providerId: 'codex',
    completeness: 'model_visible',
    items: [],
    warnings: [],
  })
  useTerminalStore.setState({ sessions: new Map() })
  // Global singleton, same as the settings store above — reset so a zoom test
  // never leaks its level into the next test.
  useZoomStore.setState({ zoom: 1, editorZoomLevel: 1, terminalZoomLevel: 1 })
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
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    const term = await screen.findByTestId('xterm')
    expect(term).toHaveAttribute('data-session-id', 'pty1')

    // The runner /clears into a brand-new chat — carrying the SAME pty.
    await act(async () => {
      seedChats([
        dormantChat({ id: 'c1' }),
        liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1', title: 'Fresh' }),
      ])
    })

    // The surface follows the runner at once; the PANE record is retargeted by
    // the stream's `moved` frame alone (one writer), never by this component.
    expect(paneOf(store, paneId)).toMatchObject({ chatId: 'c1', runnerId: 'r1' })
    expect(await screen.findByTestId('xterm')).toBe(term)
    await act(async () => {
      windowPaneStore.getState().paneActions.retargetPane(paneId, 'c2', 'r1')
    })
    expect(paneOf(store, paneId)).toMatchObject({ chatId: 'c2', runnerId: 'r1' })
    // ...while the terminal is the SAME DOM NODE. Not a remount: the very same
    // xterm instance, still attached to the same live PTY.
    expect(await screen.findByTestId('xterm')).toBe(term)
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
  })

  // DELETED (final fix wave): 'relabels to the untitled placeholder when the runner
  // /clears into a fresh chat'. The behaviour it pinned — mirroring the shown chat's
  // title onto a companion BUFFER's tab label — no longer exists: a chat is not a
  // buffer, and ChatHead reads the live title by chat id, so there is no snapshot to
  // go stale and no placeholder to fall back to.

  // Losing your runner because it MOVED is not your CLI dying. The old pane could
  // not tell those apart, so it offered a Resume button that spawned a SECOND CLI
  // on the old conversation while the first kept running.
  it('does not show the exited state when the runner merely moved', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    await act(async () => {
      seedChats([dormantChat({ id: 'c1' }), liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' })])
    })

    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
    expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
    expect(resumeChatFn).not.toHaveBeenCalled()
    expect(screen.getByTestId('xterm')).toBeTruthy()
  })

  // ── Liveness is the daemon's answer; the pane never infers or revives ──
  describe('liveness', () => {
    it('symptom 5: the composer accepts a prompt while the runner is spawning', async () => {
      const store = seedWorkspace([startingChat({ id: 'c1' })])
      seedFeed('c1', spokenIn('c1'))
      await renderPane(store, openChatPane(store, 'c1', ''))

      typeAndSend('while it starts')

      await vi.waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
      expect(submitPromptFn.mock.calls[0]?.[2]).toBe('while it starts')
    })

    it('never revives from the pending state (the chat list has not landed)', async () => {
      const store = seedWorkspace([]) // the seed is still in flight
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      // "Not known" is not "dormant". Reviving here would spawn a SECOND CLI onto a
      // chat that may well already have one.
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.queryByText(/resuming this chat/i)).not.toBeInTheDocument()

      // ...and the moment the list lands with the chat LIVE, it attaches — no revive.
      await act(async () => {
        seedChats([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      })
      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
    })

    it('does not revive a chat whose runner merely moved away', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      // c1 is now dormant — but its runner is not dead, it walked into c2, and the pane
      // walks with it. A dormant chat NOBODY IS LOOKING AT must not be revived.
      await act(async () => {
        seedChats([dormantChat({ id: 'c1' }), liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' })])
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
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

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
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

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
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      // The daemon's snapshot frame carries the forked PTY; the pane reads nothing back.
      switchToTerminalFn.mockImplementation(() => {
        writeChat(liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty-attached', provider: 'codex' }))
        return Promise.resolve('pty-attached')
      })
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

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

    // A refused switch (a turn still in flight: attach.go's ErrTurnInProgress)
    // must say so; a click that does nothing visible reads as broken.
    it('toasts when switchToTerminal is refused, instead of silently doing nothing', async () => {
      const store = seedWorkspace([liveChatNoTerminal({ id: 'c1', runnerId: 'r1' })])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      switchToTerminalFn.mockRejectedValue(
        new ApiError('agent: provider cannot hand a live turn to its native view: conflict', 409),
      )
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

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

  // The overlay header has no fill but a real hit-box: a blank chat's signpost rides
  // `.dochandle`, sharing the row's one clearance (`--agent-header-clearance`).
  describe('header clearance (overlay chat-blur header)', () => {
    function renderBelowOverlayHeader(store: Store, chatId: string, runnerId: string) {
      const paneId = openChatPane(store, chatId, runnerId)
      return act(() =>
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(AgentChatPane, {
              chatId,
              runnerId,
              wsId: 'w1',
              paneId,
              isActivePane: true,
              isVisible: true,
              belowOverlayHeader: true,
            }),
          ),
        ),
      )
    }

    it('clears the header for the reviving signpost when an overlay header sits above', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([startingChat({ id: 'c1' })])
      await renderBelowOverlayHeader(store, 'c1', '')

      await screen.findByTestId('agent-reviving-banner')
      const section = document.querySelector('.agent-chat.chat') as HTMLElement
      expect(section.style.getPropertyValue('--agent-header-clearance')).toBe('52px')
    })

    it('still clears the header inside AgentChatView once the chat is attached and no banner covers it', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderBelowOverlayHeader(store, 'c1', 'r1')

      await screen.findByTestId('agent-empty-document')
      expect(screen.queryByTestId('agent-idle-banner')).not.toBeInTheDocument()
      expect(screen.queryByTestId('agent-reviving-banner')).not.toBeInTheDocument()
      const section = document.querySelector('.agent-chat.chat') as HTMLElement
      expect(section.style.getPropertyValue('--agent-header-clearance')).toBe('52px')
    })

    // The transcript needs to clear the header's FULL EdgeDissolve zone
    // (ROW_HEIGHT_PX + CHAT_BLUR_EXTRA_PX = 100px Mac), not just the 52px
    // click-target the banners above clear — text left resting between the
    // two still renders visibly blurred by the dissolve's own mask layers.
    // See CHAT_BLUR_ZONE_PX in agent-chat-pane.tsx.
    it('hands the transcript its OWN, larger clearance — the full dissolve zone, not the banner click-target', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderBelowOverlayHeader(store, 'c1', 'r1')

      await screen.findByTestId('agent-empty-document')
      const section = document.querySelector('.agent-chat.chat') as HTMLElement
      expect(section.style.getPropertyValue('--agent-transcript-header-clearance')).toBe('100px')
    })

    it('gives the transcript no extra clearance with no overlay header above', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      await screen.findByTestId('agent-empty-document')
      const section = document.querySelector('.agent-chat.chat') as HTMLElement
      expect(section.style.getPropertyValue('--agent-transcript-header-clearance')).toBe('0px')
    })
  })

  // ── The "second input box" shape ────────────────────────────────────
  // Regression: the reported crop — the banner's own text WRAPPED to more
  // lines as the pane narrowed (a free-height card), which is what let it
  // crop against AgentEmptyDocument's own handle underneath it. It is now
  // ComposerSignpost's exact `.pill.halted` shape — the same one AgentComposer
  // wears for this same state once the chat has messages — a single,
  // ellipsis-truncated line with a height nothing else has to guess at.
  describe('reviving signpost shape', () => {
    it('renders the reviving banner in the same pill shape', async () => {
      chatStore.setState({ feeds: {} })

      const store = seedWorkspace([startingChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))

      const banner = await screen.findByTestId('agent-reviving-banner')
      expect(banner.querySelector('.pill.halted')).not.toBeNull()
    })
  })

  // ── Keep-alive: a hidden tab stays mounted and spawns nothing ──────
  describe('hidden keep-alive tab', () => {
    // isVisible=false is the hidden tab; isActivePane=true proves the gate is on
    // VISIBILITY, not pane focus — a dormant chat sitting hidden inside the active pane
    // still must not spawn a CLI (Risk #4: the two flags are distinct).
    it('does not revive a hidden dormant chat', async () => {
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      const paneId = openChatPane(store, 'c1', '')
      await act(async () => {
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(AgentChatPane, {
              chatId: 'c1',
              runnerId: '',
              wsId: 'w1',
              paneId,
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

    // Keep-alive's other half: an ALREADY-ATTACHED chat keeps its live PTY while hidden.
    // It has a sessionId, so it never reaches the revive gate — it seeds/attaches as
    // usual, just not focused and not visible.
    it('keeps an attached chat mounted while hidden (no revive, terminal stays)', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await act(async () => {
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(AgentChatPane, {
              chatId: 'c1',
              runnerId: 'r1',
              wsId: 'w1',
              paneId,
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
  it('attaches the live runner PTY: mounts an attach-only terminal onto it', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    const xterm = await screen.findByTestId('xterm')
    expect(xterm.getAttribute('data-session-id')).toBe('pty1')
    // Chat is the default presentation. The PTY remains attach-only and mounted,
    // but xterm may neither focus nor resize while it is behind Chat.
    expect(xterm.getAttribute('data-active')).toBe('false')
    expect(xterm.getAttribute('data-visible')).toBe('false')
    // Attach-only: a reconnect can never spawn a bare shell into the agent frame.
    expect(xterm.getAttribute('data-attach-only')).toBe('true')
    // liveRunnerId IS the liveness answer — no second round trip asks the daemon.
  })

  it('renders nothing (not the exited state) while the chat list is still loading', async () => {
    // The seed is in flight: the store does not know this chat yet. "Not known" is
    // not "dormant" — flashing Resume here would offer a button that spawns a
    // second CLI onto a chat that may well be live.
    //
    // NO SEED HAS RUN, which is the actual condition being described. Seeding an
    // empty list is a different fact — the daemon answering "there are none" —
    // and a pane pointed at a chat that answer does not carry resolves it rather
    // than waiting (see agent-chat-pane-unknown-chat-wedge.test.tsx).
    const store = unseededWorkspace()
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    expect(screen.queryByTestId('xterm')).toBeNull()
    expect(screen.queryByTestId('pane-resume')).not.toBeInTheDocument()
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
  })

  it('reads a restored chat before the chat list lands, when its pane records the workspace', async () => {
    const store = unseededWorkspace()
    const paneId = openChatPane(store, 'c1', 'r1')
    windowPaneStore.setState((s) => {
      s.panes[paneId].workspaceId = 'w1'
      return s
    })
    await renderPane(store, paneId)

    expect(heldScopes('chat:')).toContain('chat:c1')
  })

  // The feed is addressed by chat id alone, so no workspace has to vouch for it.
  it('opens the feed of a chat with no recorded workspace before the list lands', async () => {
    const store = unseededWorkspace()
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    expect(heldScopes('chat:')).toContain('chat:c1')
  })

  it('threads isActivePane=false through to the terminal', async () => {
    const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const paneId = openChatPane(store, 'c1', 'r1')
    await act(async () => {
      render(
        createElement(
          WorkspaceStoreContext.Provider,
          { value: store },
          createElement(AgentChatPane, {
            chatId: 'c1',
            runnerId: 'r1',
            wsId: 'w1',
            paneId,
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
    const paneId = openChatPane(store, 'c1', 'r1')
    await renderPane(store, paneId)

    const before = await screen.findByTestId('xterm')
    expect(before).toHaveAttribute('data-session-id', 'pty1')

    await act(async () => {
      seedChats([liveChat({ id: 'c1', runnerId: 'r2', pty: 'pty2', provider: 'claude' })])
    })

    const after = await screen.findByTestId('xterm')
    expect(after).toHaveAttribute('data-session-id', 'pty2')
    expect(after).toBe(before) // SAME node: the attachment swapped, the terminal did not remount
    expect(paneOf(store, paneId)).toMatchObject({ chatId: 'c1', runnerId: 'r2' })
    expect(screen.getByTestId('provider-switch').getAttribute('data-current')).toBe('claude')
  })

  // ── The daemon owns the lifecycle; the pane only sends intents ──────
  describe('server-owned lifecycle', () => {
    it('never resumes a dormant chat on open, and still takes a message', async () => {
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))

      expect(resumeChatFn).not.toHaveBeenCalled()
      expect(screen.getByTestId('agent-session-note')).toHaveTextContent(/send a message/i)

      typeAndSend('wake up')
      await waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
      expect(resumeChatFn).not.toHaveBeenCalled()
    })

    it.each([
      ['stopped', 'Stopped. Send a message to continue the conversation.'],
      ['retracted', 'You took the prompt back. Send a message to continue.'],
    ])('shows the exit reason from the last runner_exited row (%s)', async (reason, note) => {
      const l = ledger('c1', 4)
      const exited = l.row('runner_exited', { runnerId: 'r1', reason, exitCode: 0 })
      seedFeed('c1', [...spokenIn('c1'), exited])
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))
      expect(screen.getByTestId('agent-session-note')).toHaveTextContent(note)
    })

    it('starts a session from the terminal surface only when asked', async () => {
      landOnTerminal()
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))

      expect(resumeChatFn).not.toHaveBeenCalled()
      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /start session/i }))
      })
      expect(resumeChatFn).toHaveBeenCalledWith('w1', 'c1')
    })

    it('toasts when starting a session fails', async () => {
      landOnTerminal()
      resumeChatFn.mockRejectedValue(new ApiError('terminal: command not found: codex', 424))
      const store = seedWorkspace([dormantChat({ id: 'c1' })])
      await renderPane(store, openChatPane(store, 'c1', ''))

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /start session/i }))
      })
      await waitFor(() => expect(toastErrorFn).toHaveBeenCalledTimes(1))
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
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      const footerControl = screen.getByTestId('provider-switch')
      expect(footerControl.getAttribute('data-current')).toBe('codex')
      expect(footerControl.getAttribute('data-count')).toBe('2')
    })

    it('is one flat surface — no card, and the switcher shares the terminal column', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

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
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      // The runner /clears into c2 — the tab follows it.
      await act(async () => {
        seedChats([dormantChat({ id: 'c1' }), liveChat({ id: 'c2', runnerId: 'r1', pty: 'pty1' })])
      })

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      // The switch must target c2. Targeting c1 would hand a CLI the conversation
      // the user has already left, and leave the live one running unattended.
      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c2', 'codex')
    })

    it('surfaces a toast when the switch rejects (target CLI missing / spawn failed)', async () => {
      const err = vi.spyOn(console, 'error').mockImplementation(() => {})
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      // The production failure verbatim: the daemon could not find the codex binary and
      // answered 424 Failed Dependency.
      switchProviderFn.mockRejectedValue(new ApiError('terminal: command not found: codex', 424))
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

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
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      expect(switchProviderFn).toHaveBeenCalledWith('w1', 'c1', 'codex')
      expect(toastErrorFn).not.toHaveBeenCalled()
    })

    // A switch keeps the surface, and only an attach request gives a non-hotswap
    // provider a terminal view to show on it.
    it('re-requests an attach when switching TO a non-hotswap provider while already on the terminal surface', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'claude' }),
      ])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      // Each intent's answer arrives as the daemon's snapshot frame.
      switchProviderFn.mockImplementation(() => {
        writeChat(liveChatNoTerminal({ id: 'c1', runnerId: 'r2' }))
        return Promise.resolve('r2')
      })
      switchToTerminalFn.mockImplementation(() => {
        writeChat(liveChat({ id: 'c1', runnerId: 'r2', pty: 'pty-attached', provider: 'codex' }))
        return Promise.resolve('pty-attached')
      })
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      await act(async () => {
        fireEvent.click(screen.getByTestId('provider-switch'))
      })

      await vi.waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledWith('w1', 'c1'))
      expect(await screen.findByTestId('xterm')).toHaveAttribute('data-session-id', 'pty-attached')
      expect(screen.queryByText(/no terminal view attached/i)).not.toBeInTheDocument()
    })
  })

  // ── Tab title ──────────────────────────────────────────────────────
  // DELETED (final fix wave): 'tab title tracks the chat title' (2 tests). They
  // asserted `renameBuffer` mirrored the chat title onto the pane's companion
  // buffer's tab label. A chat has had no buffer since Task 1, and Task 17's
  // ChatHead subscribes to the chat's own `title` directly, so there is nothing
  // left to mirror — chat-head.tsx's own tests cover what the head shows.

  // ── ⌘/ toggles the chat/terminal view, like the ViewSwitcher tabs ─────
  describe('toggle chat/terminal view chord', () => {
    const pressToggle = async () => {
      await act(async () => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key: '/', metaKey: true }))
      })
    }

    it('flips a hotswap provider straight from Chat to Terminal', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      expect(screen.getByRole('tab', { name: /^chat$/i })).toHaveAttribute('aria-selected', 'true')

      await pressToggle()

      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'true',
      )
    })

    it('flips back from Terminal to Chat on a second press', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      await pressToggle()
      await pressToggle()

      expect(screen.getByRole('tab', { name: /^chat$/i })).toHaveAttribute('aria-selected', 'true')
    })

    it('still fires when the focused child swallows the key (xterm stopPropagation)', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

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
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      const hiddenPaneId = openChatPane(hidden, 'c1', 'r1', 'Chat', 'w-hidden')
      const shown = seedWorkspace(
        [liveChat({ id: 'c2', runnerId: 'r2', pty: 'pty2', provider: 'codex' })],
        'w-shown',
      )
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      const shownPaneId = openChatPane(shown, 'c2', 'r2', 'Chat', 'w-shown')

      // Both panes are the active, visible tab of their own workspace — exactly
      // what a retained workspace looks like the instant it goes hidden.
      await renderPane(hidden, hiddenPaneId)
      await renderPane(shown, shownPaneId)
      setActiveWorkspaceId('w-shown')

      await pressToggle()

      await vi.waitFor(() => expect(switchToTerminalFn).toHaveBeenCalledTimes(1))
      expect(switchToTerminalFn).toHaveBeenCalledWith('w-shown', 'c2')
    })

    // THE BUG, as the user reported it: "Can't start a chat with codex TUI
    // alone. It's only allowing me to start a thread on the native chat
    // interface, and then go TUI."
    //
    // A chat BORN on the terminal surface has no api connection behind it —
    // the daemon never opened one (spawnRunner's surfaceForSpawn) — so its own
    // PTY IS the conversation, with nothing to ask the daemon for.
    it('a terminal-BORN chat shows its own PTY without asking for an attach', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'codex', surface: 'terminal' }),
      ])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      await pressToggle()

      expect(switchToTerminalFn).not.toHaveBeenCalled()
      expect(screen.getByRole('tab', { name: /^terminal$/i })).toHaveAttribute(
        'aria-selected',
        'true',
      )
      expect(screen.getByTestId('xterm')).toHaveAttribute('data-session-id', 'pty1')
      expect(toastErrorFn).not.toHaveBeenCalled()
    })

    it('a HIDDEN chat ignores the chord — a background split must not switch', async () => {
      switchToTerminalFn.mockResolvedValue('pty-attached')
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'codex' }),
      ])
      setProviders([providers[0], { ...providers[1], hotswap: false }])
      const paneId = openChatPane(store, 'c1', 'r1')
      // Every chat stays mounted for keep-alive, so without the isVisible gate a
      // hidden tab would swallow the chord and flip the view on a chat nobody can see.
      await act(async () => {
        render(
          createElement(
            WorkspaceStoreContext.Provider,
            { value: store },
            createElement(PaneHost, { paneId, isVisible: false }),
          ),
        )
      })

      await pressToggle()

      expect(switchToTerminalFn).not.toHaveBeenCalled()
    })
  })

  describe('chat zoom', () => {
    it('applies the zoom-store level as CSS zoom on the chat surface', async () => {
      useZoomStore.setState({ zoom: 1.4 })
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      expect(screen.getByTestId('agent-chat-surface')).toHaveStyle({ zoom: '1.4' })
    })

    it('follows the store live as it changes', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const paneId = openChatPane(store, 'c1', 'r1')
      await renderPane(store, paneId)

      expect(screen.getByTestId('agent-chat-surface')).toHaveStyle({ zoom: '1' })

      await act(async () => {
        useZoomStore.getState().actions.zoomIn()
      })

      expect(screen.getByTestId('agent-chat-surface')).toHaveStyle({ zoom: '1.1' })
    })
  })

  describe('React chat presentation', () => {
    it('defaults to Chat while retaining the native terminal as an attach-only fallback', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

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

    it('holds prompts queued in Chat while the Terminal shows: counts, cancels, sends on return', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      const turn = ledger('c1', 4)
      seedFeed('c1', [turn.start('turn-2')])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))
      const queueThenShowTerminal = (text: string) => {
        typeAndSend(text)
        fireEvent.click(screen.getByRole('tab', { name: /^terminal$/i }))
        expect(screen.getByText(/1 prompt pending in Chat/i)).toBeInTheDocument()
      }
      queueThenShowTerminal('cancelled from there')
      fireEvent.click(screen.getByRole('button', { name: /cancel unsent prompts/i }))
      expect(screen.queryByText(/pending in Chat/i)).not.toBeInTheDocument()
      fireEvent.click(screen.getByRole('tab', { name: /^chat$/i }))
      queueThenShowTerminal('queued while busy')
      await act(async () => seedFeed('c1', [turn.end('turn-2')]))
      expect(submitPromptFn).not.toHaveBeenCalled()
      fireEvent.click(screen.getByRole('button', { name: /return to chat/i }))
      await vi.waitFor(() => expect(submitPromptFn).toHaveBeenCalledTimes(1))
      expect(submitPromptFn.mock.calls[0]?.[2]).toBe('queued while busy')
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
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      typeAndSend('deliver exactly once')
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

    // The daemon refuses a switch while a turn runs; the pane offers none.
    it('offers no provider switch while the chat is working', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      setChatWorking('c1', true)
      await renderPane(store, openChatPane(store, 'c1', 'r1'))
      await act(async () => {
        fireEvent.click(screen.getByLabelText('Terminal'))
      })

      expect(screen.getByTestId('provider-switch')).toBeDisabled()
      await act(async () => setChatWorking('c1', false))
      expect(screen.getByTestId('provider-switch')).toBeEnabled()
    })

    it('reconciles a failed send to dormant instead of retaining a dead PTY attachment', async () => {
      const store = seedWorkspace([liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
      submitPromptFn.mockRejectedValue(new ApiError('replacement failed', 500))
      await renderPane(store, openChatPane(store, 'c1', 'r1'))

      typeAndSend('trigger replacement')
      // The daemon's lifecycle frame says the runner is gone.
      await act(async () => writeChat(dormantChat({ id: 'c1' })))

      expect(await screen.findByTestId('agent-session-note')).toHaveTextContent(/not running/i)
      expect(screen.queryByTestId('xterm')).not.toBeInTheDocument()
      expect(chatStore.getState().outbox.c1?.[0]).toMatchObject({
        state: 'failed',
        error: 'replacement failed',
      })
    })
  })

  // The banner and the composer's signpost render off the same `waiting` signal;
  // once a composer can show it, the signpost is its one occupant. The blank-chat
  // half is in agent-chat-pane-terminal-wait.test.tsx.
  describe('terminal_wait on a chat that already has messages', () => {
    it('mutates the composer into a signpost instead of duplicating a pane-level banner', async () => {
      const store = seedWorkspace([
        liveChat({ id: 'c1', runnerId: 'r1', pty: 'pty1', provider: 'claude' }),
      ])
      await renderPane(store, openChatPane(store, 'c1', 'r1'))
      // The default beforeEach seeds one finished turn, so this chat
      // is NOT blank — AgentComposer, not AgentEmptyDocument, is mounted.
      expect(screen.getByRole('textbox', { name: /message the agent/i })).toBeInTheDocument()

      await act(async () => {
        setChatTerminalWait('c1', { kind: 'workspace_trust' })
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
