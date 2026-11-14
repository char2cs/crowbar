import { act, fireEvent, render, screen } from '@testing-library/react'
import { createElement } from 'react'
import { beforeEach, vi } from 'vitest'
import { nanoid } from 'nanoid'
import { useStore } from 'zustand'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { seedChatPaneRecord } from '@/__tests__/__fixtures__/view-state'
import { seedFeed, spokenIn } from '@/__tests__/__fixtures__/ledger-rows'
import { platform } from '@/__tests__/__fixtures__/pane-platform'
import { resetChatPresentationMemoryForTests } from '@/features/agent/hooks/use-chat-presentation'

export const switchProviderFn = vi.fn()
export const resumeChatFn = vi.fn()
export const submitPromptFn = vi.fn()
export const slashCatalogFn = vi.fn()
export const toastErrorFn = vi.fn()
export const switchToTerminalFn = vi.fn()
export const switchToNativeFn = vi.fn()

// The keymap pinned rather than the settings store; an unbound chord is simply absent.
import * as useEffectiveKeymapMod from '@/features/keymaps/hooks/use-effective-keymap'
vi.spyOn(useEffectiveKeymapMod, 'useEffectiveChordMap').mockImplementation(() => ({
  'agent.toggleViewMode': 'mod+/',
}))

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
        onClick: () => onSwitch('provider-b'),
      },
      'switch',
    ),
)

import { AgentChatPaneGate } from '@/features/agent/components/agent-chat-pane-gate'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useZoomStore } from '@/features/window/stores/zoom-store'
import { useSettingsStore } from '@/features/settings/store'
import {
  nextVersion,
  seedChats,
  resetChatStore,
  setProviders,
  provider,
} from '@/__tests__/__fixtures__/agent-chat'

/**
 * Land this pane on the TERMINAL surface.
 *
 * The chat's status strip — its title and the provider switcher — is the
 * provider's-own-view chrome and is drawn only there: Chat states everything it
 * is running as in its own underbar, under the composer. So a test about the
 * switcher has to be on the surface that has one.
 */
export function landOnTerminal() {
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: false },
  }))
}

// Both real descriptors declare hotswap:true and keep a real terminal; the false
// branch has its own coverage in agent-chat-view.test.tsx.
export const providers: AgentProvider[] = [
  provider('provider-a', 'Provider A'),
  provider('provider-b', 'Provider B'),
]

// ── Wire fixtures ────────────────────────────────────────────────────
// A chat is LIVE exactly while a runner is placed on it. liveRunnerId is the whole
// liveness contract — no status flag exists that could disagree with it — and it
// carries that runner's PTY, which is what the pane attaches to.

export function liveChat(o: {
  id: string
  runnerId: string
  pty: string
  title?: string
  provider?: string
  /** The view the daemon holds the chat on; omitted is the provider's default face. */
  surface?: 'chat' | 'terminal'
}): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: o.title ?? `Chat ${o.id}`,
    liveRunnerId: o.runnerId,
    terminalSessionId: o.pty,
    surface: o.surface,
    activeProviderId: o.provider ?? 'provider-b',
    working: false,
    version: nextVersion(),
    phase: 'dormant',
    createdAt: '',
    order: 0,
  }
}

/** A dormant chat: no runner points at it, so there is nothing to attach. It keeps
 *  the provider of its last conversation — who Resume brings back. */
export function dormantChat(o: { id: string; title?: string; provider?: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: o.title ?? `Chat ${o.id}`,
    liveRunnerId: '',
    terminalSessionId: '',
    activeProviderId: o.provider ?? 'provider-b',
    working: false,
    version: nextVersion(),
    phase: 'dormant',
    createdAt: '',
    order: 0,
  }
}

/** A chat the daemon is placing a CLI on (a revive or a switch in flight). */
export function startingChat(o: { id: string }): AgentChat {
  return { ...dormantChat(o), phase: 'starting' }
}

/** A LIVE runner with nothing attached — the correct idle shape for a non-hotswap
 *  api-transport provider (providerB) that has never been switched to its native view.
 *  This is NOT dormancy: liveRunnerId is the only liveness signal, and it is set. */
export function liveChatNoTerminal(o: { id: string; runnerId: string; title?: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: o.title ?? `Chat ${o.id}`,
    liveRunnerId: o.runnerId,
    terminalSessionId: '',
    activeProviderId: 'provider-b',
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

// ── Harness ──────────────────────────────────────────────────────────
// PaneHost does what pane-container.tsx does: read the window-level `PaneGroup`
// and feed its chatId/runnerId back in, so every repoint assertion runs the
// same `setPaneChat` write production does.

// A real `PaneGroup` carries no workspace id (pane-container reads it from the
// ambient WorkspaceStoreContext), so the harness keeps it beside the pane
// rather than inventing a field production does not have.
const paneWorkspace = new Map<string, string>()

export function seedWorkspace(chats: AgentChat[], wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  setProviders(providers)
  seedChats(chats)
  return store
}

/** A workspace whose chat list has NOT arrived yet — no seed has run. Distinct
 *  from seeding an empty list, which is the daemon answering "there are none". */
export function unseededWorkspace(wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  setProviders(providers)
  return store
}

export type Store = ReturnType<typeof seedWorkspace>

// `_name` is vestigial (a chat has no buffer to label); kept so the call sites stay unchanged.
export function openChatPane(
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

export function PaneHost({ paneId, isVisible = true }: { paneId: string; isVisible?: boolean }) {
  const group = useStore(windowPaneStore, (s) => s.panes[paneId])
  if (!group) return null
  return createElement(AgentChatPaneGate, {
    chatId: group.chatId ?? '',
    runnerId: group.runnerId ?? '',
    wsId: paneWorkspace.get(paneId) ?? 'w1',
    paneId: group.id,
    isActivePane: true,
    // Default true: the vast majority of these tests are the ACTIVE, visible pane.
    isVisible,
  })
}

export async function renderPane(store: Store, paneId: string) {
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

export function typeAndSend(text: string) {
  const input = screen.getByRole('textbox', { name: /message the agent/i })
  fireEvent.change(input, { target: { value: text } })
  fireEvent.keyDown(input, { key: 'Enter' })
}

export const paneOf = (_store: Store, id: string) => windowPaneStore.getState().panes[id]

/** Registered by every pane suite: a healthy backend and a spoken-in chat by default. */
function resetPaneHarness() {
  platform.mobile = false
  resetWindowPaneStoreForTests()
  resetChatStore()
  resetChatPresentationMemoryForTests()
  paneWorkspace.clear()
  switchProviderFn.mockReset()
  resumeChatFn.mockReset()
  submitPromptFn.mockReset()
  slashCatalogFn.mockReset()
  toastErrorFn.mockReset()
  switchToTerminalFn.mockReset().mockResolvedValue('')
  switchToNativeFn.mockReset().mockResolvedValue(undefined)
  switchProviderFn.mockResolvedValue('r-new')
  resumeChatFn.mockResolvedValue('r-revived')
  // A chat that has been SPOKEN IN. A chat with no messages is the blank
  // DOCUMENT surface — writing size, no pill under it — so a pane test about the
  // composer, the queue or the switcher has to start from a conversation for its
  // subject to be on screen at all.
  seedFeed('c1', spokenIn('c1'))
  submitPromptFn.mockResolvedValue({ runnerId: 'r-prompt', terminalSessionId: 'pty-prompt' })
  slashCatalogFn.mockResolvedValue({
    providerId: 'provider-b',
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
}

beforeEach(resetPaneHarness)
