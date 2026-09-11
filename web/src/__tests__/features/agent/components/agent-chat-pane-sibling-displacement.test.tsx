/**
 * TWO PANES, ONE CHAT, ONE PANE REPLACES THE CLI.
 *
 * Splitting a chat pane (Cmd+\) mounts a SECOND AgentChatPane on the identical
 * chatId — the same supported gesture `agent-chat-pane-multi-pane-resume-race`
 * covers for the MOUNT race. This file covers the other half: what the sibling
 * does while the first pane DISPLACES the chat's CLI.
 *
 * A provider switch (and an ordinary prompt submission) kills the outgoing
 * process BEFORE the replacement exists, so the chat reads dormant for the width
 * of a spawn. The pane that asked for it held `switchingRef` and sat the window
 * out. The sibling held nothing — `switchingRef` is per-component — so it read
 * its neighbour's deliberate displacement as its own agent dying, and did one of
 * two wrong things depending only on whether it had already spent its revive
 * budget:
 *
 *   budget unspent → fired a SECOND resumeChat into the daemon's per-chat spawn
 *                    gate that the switch was already holding.
 *   budget spent   → latched `idle: 'exited'` — "This agent has exited", with a
 *                    Resume button, over a CLI that was alive and about to
 *                    answer. That also drops the composer's `live`, so the
 *                    prompt queue stops dispatching and a typed message sits as
 *                    "1 queued" for good.
 *
 * Measured live in a two-pane split, switching provider in the LEFT pane only
 * (x = the pane's on-screen position):
 *   x313[reviving:"Starting Codex…"] || x969[live]
 *   x313[reviving:"Starting Codex…"] || x969[idle:exited]   <- the sibling
 *   x313[live]                       || x969[live]
 *
 * The fix is the chat-scoped `displacingByChatId` hold in agent-chat-pane.tsx —
 * the same "this belongs to the chat, not to one pane" shape
 * `reviveInFlightByChatId` already has.
 */
import { createElement } from 'react'
import { act, render, screen } from '@testing-library/react'
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
  switchProviderFn,
  listMessagesFn,
  submitPromptFn,
  slashCatalogFn,
  saveReconnectFn,
} = vi.hoisted(() => ({
  getChatFn: vi.fn(),
  resumeChatFn: vi.fn(),
  switchProviderFn: vi.fn(),
  listMessagesFn: vi.fn(),
  submitPromptFn: vi.fn(),
  slashCatalogFn: vi.fn(),
  saveReconnectFn: vi.fn(),
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
  switchToTerminal: vi.fn(),
  switchToNative: vi.fn(),
}))

vi.mock('@/features/terminal/lib/terminal-reconnect-map', () => ({
  saveReconnect: (...a: unknown[]) => saveReconnectFn(...a),
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn() },
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

// The real ProviderBar is where `onSwitchProvider` (AgentChatPane's handleSwitch)
// reaches the user. Standing in for it with a plain button keeps the switch
// going through the REAL handleSwitch — the thing under test — without driving a
// popup menu.
vi.mock('@/features/agent/controls/provider-bar', () => ({
  ProviderBar: ({ onSwitchProvider }: { onSwitchProvider?: (id: string) => Promise<boolean> }) =>
    createElement('button', {
      'data-testid': 'switch-to-codex',
      onClick: () => void onSwitchProvider?.('codex'),
    }),
}))

import { AgentChatPane } from '@/features/agent/components/agent-chat-pane'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import { useSettingsStore } from '@/features/settings/store'

const providers: AgentProvider[] = [
  {
    id: 'claude',
    displayName: 'Claude',
    icon: '<svg/>',
    connected: true,
    enabled: true,
    mcpEnabled: true,
    hasTerminal: true,
    hotswap: false,
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

function chatRow(o: { id: string; runnerId: string; pty: string }): AgentChat {
  return {
    id: o.id,
    workspaceId: 'w1',
    title: `Chat ${o.id}`,
    liveRunnerId: o.runnerId,
    terminalSessionId: o.pty,
    activeProviderId: 'claude',
    createdAt: '',
    order: 0,
  }
}

const dormant = (id: string) => chatRow({ id, runnerId: '', pty: '' })
const detail = (chat: AgentChat): AgentChatDetail => ({ ...chat, conversations: [] })

function seedWorkspace(chats: AgentChat[]) {
  const store = createWorkspaceStore('w1')
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

/** One pane. `active` is the pane with focus — only ever one of the two, which is
 *  what makes the OTHER one the sibling this file is about. */
function PaneHost({ bufferId, active }: { bufferId: string; active: boolean }) {
  const store = useWorkspaceStore()
  const buf = useStore(store, (s) => s.buffers.find((b) => b.id === bufferId)) as
    AgentChatContent | undefined
  if (!buf) return null
  return createElement(AgentChatPane, {
    chatId: buf.chatId,
    runnerId: buf.runnerId,
    wsId: buf.wsId,
    bufferId: buf.id,
    isActivePane: active,
    isVisible: true,
  })
}

function renderSplit(store: Store, bufferA: string, bufferB: string) {
  return render(
    createElement(
      WorkspaceStoreContext.Provider,
      { value: store },
      createElement(
        'div',
        null,
        createElement(PaneHost, { bufferId: bufferA, active: true }),
        createElement(PaneHost, { bufferId: bufferB, active: false }),
      ),
    ),
  )
}

/** What the daemon does to the store mid-switch: the outgoing CLI is killed
 *  first, so the chat is dormant until the replacement lands. This is the WS
 *  `displaced` frame, reproduced. */
function goDormant(store: Store, chatId: string) {
  store.getState().upsertAgentChat(detail(dormant(chatId)))
}

beforeEach(() => {
  for (const f of [
    getChatFn,
    resumeChatFn,
    switchProviderFn,
    listMessagesFn,
    submitPromptFn,
    slashCatalogFn,
    saveReconnectFn,
  ])
    f.mockReset()
  // A chat with nothing said in it renders AgentEmptyDocument, which has no
  // ProviderBar — the switch has to start from a real conversation for its
  // control to be on screen at all.
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
  submitPromptFn.mockResolvedValue({ runnerId: 'r1', terminalSessionId: 'pty1' })
  slashCatalogFn.mockResolvedValue({
    providerId: 'claude',
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

describe("AgentChatPane: a sibling pane during its neighbour's CLI displacement", () => {
  it('TestRegression_SiblingPaneDoesNotResumeDuringAnotherPanesSwitch', async () => {
    getChatFn.mockImplementation((_wsId: unknown, id: unknown) =>
      Promise.resolve(detail(chatRow({ id: String(id), runnerId: 'r2', pty: 'pty2' }))),
    )
    let finishSwitch!: () => void
    switchProviderFn.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finishSwitch = () => resolve()
        }),
    )

    const store = seedWorkspace([chatRow({ id: 'c1', runnerId: 'r1', pty: 'pty1' })])
    const bufferA = openBuffer(store, 'c1', 'r1')
    const bufferB = openBuffer(store, 'c1', 'r1')
    await act(async () => {
      renderSplit(store, bufferA, bufferB)
    })

    // Pane A switches provider. The daemon kills the outgoing CLI first, so the
    // chat goes dormant while the request is still out.
    await act(async () => {
      screen.getAllByTestId('switch-to-codex')[0]?.click()
    })
    await act(async () => {
      goDormant(store, 'c1')
    })

    // THE REGRESSION ASSERTION. Without the chat-scoped hold the sibling pane
    // reads this gap as its own agent dying and fires a resumeChat straight into
    // the per-chat spawn gate the switch is already holding.
    expect(resumeChatFn).not.toHaveBeenCalled()

    await act(async () => {
      finishSwitch()
    })
    await act(async () => {})

    // The switch's own replacement is what brings the chat back — for both panes.
    expect(resumeChatFn).not.toHaveBeenCalled()
    const xterms = await screen.findAllByTestId('xterm')
    expect(xterms).toHaveLength(2)
    for (const el of xterms) expect(el).toHaveAttribute('data-session-id', 'pty2')
  })

  it('TestRegression_SiblingPaneDoesNotSayExitedDuringAnotherPanesSwitch', async () => {
    // Spend BOTH panes' revive budget first, exactly as a real session does: the
    // chat starts dormant, the shared revive brings it up, and every pane has now
    // used its one unattended attempt on this chat. That is the state in which the
    // sibling stops reviving and starts ASSERTING the agent has exited.
    resumeChatFn.mockResolvedValue('r1')
    getChatFn.mockImplementation((_wsId: unknown, id: unknown) =>
      Promise.resolve(detail(chatRow({ id: String(id), runnerId: 'r1', pty: 'pty1' }))),
    )

    const store = seedWorkspace([dormant('c1')])
    const bufferA = openBuffer(store, 'c1', '')
    const bufferB = openBuffer(store, 'c1', '')
    await act(async () => {
      renderSplit(store, bufferA, bufferB)
    })
    await act(async () => {})
    expect(resumeChatFn).toHaveBeenCalledTimes(1)
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()

    let finishSwitch!: () => void
    switchProviderFn.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finishSwitch = () => resolve()
        }),
    )
    getChatFn.mockImplementation((_wsId: unknown, id: unknown) =>
      Promise.resolve(detail(chatRow({ id: String(id), runnerId: 'r2', pty: 'pty2' }))),
    )

    await act(async () => {
      screen.getAllByTestId('switch-to-codex')[0]?.click()
    })
    await act(async () => {
      goDormant(store, 'c1')
    })

    // THE REGRESSION ASSERTION — the user-visible half. The sibling pane, which
    // did nothing, announced "This agent has exited" with a Resume button over a
    // CLI that was alive and about to answer, and froze its own prompt queue
    // behind the `live` that state drops.
    expect(screen.queryByText(/this agent has exited/i)).not.toBeInTheDocument()
    expect(screen.queryAllByTestId('pane-resume')).toHaveLength(0)
    // The budget must still be spendable for a REAL death later: the sibling is
    // waiting, not retired, so no second resume went out either.
    expect(resumeChatFn).toHaveBeenCalledTimes(1)

    await act(async () => {
      finishSwitch()
    })
    await act(async () => {})

    const xterms = await screen.findAllByTestId('xterm')
    expect(xterms).toHaveLength(2)
    for (const el of xterms) expect(el).toHaveAttribute('data-session-id', 'pty2')
  })
})
