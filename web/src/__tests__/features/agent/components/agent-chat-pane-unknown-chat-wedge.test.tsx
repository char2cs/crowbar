import { createElement } from 'react'
import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useStore } from 'zustand'
import { nanoid } from 'nanoid'
import type { AgentChat, AgentChatDetail, AgentProvider } from '@/features/agent/api/agent-api'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import { seedChatPaneRecord } from '@/__tests__/__fixtures__/view-state'

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
  getPendingPrompt: vi.fn().mockResolvedValue(null),
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
import { savePromptQueue } from '@/features/agent/lib/prompt-queue-persistence'
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

function detail(chat: AgentChat): AgentChatDetail {
  return { ...chat, conversations: [] }
}

function seedWorkspace(chats: AgentChat[], wsId = 'w1') {
  const store = createWorkspaceStore(wsId)
  store.getState().setAgentProviders(providers)
  // An AUTHORITATIVE list — the initial load / reconnect reseed. This is the
  // whole point of the fixture: the list HAS arrived, and the pane's chat is
  // simply not in it.
  store.getState().seedAgentChats(chats)
  return store
}

type Store = ReturnType<typeof seedWorkspace>

// A chat is a PANE, not a buffer: panes are window-level (windowPaneStore) and
// carry chatId/runnerId as fields of their own. That is exactly the shape this
// file needs — a pane can be pointed at a chatId the workspace store's own list
// has never carried, which is the whole fixture.
//
// A real `PaneGroup` carries no workspace id (pane-container reads it from the
// ambient WorkspaceStoreContext), so the harness keeps it beside the pane.
const paneWorkspace = new Map<string, string>()

function openChatPane(_store: Store, chatId: string, runnerId: string, wsId = 'w1') {
  const id = nanoid()
  seedChatPaneRecord(windowPaneStore, id, chatId, runnerId || null)
  paneWorkspace.set(id, wsId)
  return id
}

function PaneHost({ paneId }: { paneId: string }) {
  const group = useStore(windowPaneStore, (s) => s.panes[paneId])
  if (!group) return null
  return createElement(AgentChatPane, {
    chatId: group.chatId ?? '',
    runnerId: group.runnerId ?? '',
    wsId: paneWorkspace.get(paneId) ?? 'w1',
    paneId: group.id,
    isActivePane: true,
    isVisible: true,
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

/**
 * Drain React's effect/promise chain to a FIXED POINT — no clock, no sleep.
 *
 * The wedge is the absence of a network call, and proving an absence needs the
 * app to have run out of work to do rather than merely "not got there yet".
 * Each round flushes one layer of effect -> promise -> effect, and the pane's
 * deepest honest settle path (resolve the chat, adopt it, dispatch the head) is
 * only a few layers deep, so this bounds it comfortably.
 */
async function settle(rounds = 12) {
  for (let index = 0; index < rounds; index++) {
    await act(async () => {
      await Promise.resolve()
    })
  }
}

beforeEach(() => {
  resetWindowPaneStoreForTests()
  paneWorkspace.clear()
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
  listMessagesFn.mockResolvedValue({
    cursor: 1,
    oldestCursor: 1,
    hasMore: false,
    items: [],
  })
  submitPromptFn.mockResolvedValue({ runnerId: 'r-live', terminalSessionId: 'pty-live' })
  slashCatalogFn.mockResolvedValue({
    providerId: 'codex',
    completeness: 'model_visible',
    items: [],
    warnings: [],
  })
  useTerminalStore.setState({ sessions: new Map() })
  useSettingsStore.setState((state) => ({
    settings: { ...state.settings, chatIsDefaultPresentation: true },
  }))
  localStorage.clear()
  setActiveWorkspaceId('w1')
})

// ── A queued prompt for a chat the store's list does not contain ───────────
//
// THE BUG, as the user hit it: a message sits "1 queued" in the composer for
// good, and the daemon's access log shows the frontend never once attempted the
// POST — no failure, no retry, nothing. The backend meanwhile reports the chat
// perfectly healthy: a live runner on it and `working: false`.
//
// The pane's `live` is `attachment.state === 'attached'`, and `attachment` is
// pinned at `pending` whenever `known` is false — `known` being nothing more
// than "is this chat in the store's chat list". That check cannot tell the two
// cases apart:
//
//   the list has not arrived yet          → waiting is right, and brief.
//   the list arrived and lacks this chat  → waiting is FOREVER.
//
// And it is self-sealing. Everything that could teach the store about the chat
// — adopt(), refreshChatWorking() — is reached only through a path gated on
// `live`, which is gated on `known`. So `pending` renders nothing, spawns
// nothing, asks the daemon nothing, and the FIFO's `!live` bails on every pass.
describe('a queued prompt for a chat missing from the seeded list', () => {
  it('still reaches the daemon', async () => {
    // The list is authoritative and complete — and this pane's chat is not in it.
    const store = seedWorkspace([
      liveChat({ id: 'c-other', runnerId: 'r-other', pty: 'pty-other' }),
    ])

    // BACKEND TRUTH, exactly as the user's curl reported it: the chat has a live
    // runner and is idle. Nothing here justifies holding a prompt back.
    getChatFn.mockImplementation((_wsId: unknown, id: unknown) =>
      Promise.resolve({
        ...detail(liveChat({ id: String(id), runnerId: 'r-live', pty: 'pty-live' })),
        working: false,
      }),
    )

    // A prompt already queued for this chat — the composer's "1 queued".
    expect(
      savePromptQueue('w1', 'c-wedged', [
        {
          clientRequestId: 'req-wedged',
          text: 'does this ever reach the daemon',
          state: 'queued',
          createdAt: new Date().toISOString(),
          baselineSequence: 0,
        },
      ]),
    ).toBe(true)

    const paneId = openChatPane(store, 'c-wedged', '')
    await renderPane(store, paneId)
    await settle()

    // The whole bug in one assertion: a queued head, an idle chat, a live runner
    // on the backend and a visible, active pane — and not one POST attempted.
    // undefined for model/effort: this item was restored from storage and
    // staged neither, and '' would now be a PICK of the provider's own
    // default rather than silence.
    expect(submitPromptFn).toHaveBeenCalledWith(
      'w1',
      'c-wedged',
      'does this ever reach the daemon',
      'req-wedged',
      '',
      undefined,
      undefined,
    )
  })

  it('does not strand the pane when the chat is genuinely gone', async () => {
    const store = seedWorkspace([
      liveChat({ id: 'c-other', runnerId: 'r-other', pty: 'pty-other' }),
    ])
    // The daemon has never heard of it. The pane must ask ONCE and then stop —
    // a missing chat is not a reason to hammer the daemon forever.
    getChatFn.mockRejectedValue(new Error('not found'))

    const paneId = openChatPane(store, 'c-ghost', '')
    await renderPane(store, paneId)
    await settle()

    expect(getChatFn.mock.calls.filter((c) => c[1] === 'c-ghost')).toHaveLength(1)
    expect(submitPromptFn).not.toHaveBeenCalled()
  })
})
