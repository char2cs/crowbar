import { createElement, type ReactNode } from 'react'
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentChat } from '@/features/agent/api/agent-chat-reads'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

import { stubPaneChrome } from '@/__tests__/__fixtures__/agent-pane'
stubPaneChrome()
import * as readsApi from '@/features/agent/api/agent-chat-reads'
vi.spyOn(readsApi, 'getSlashCatalog').mockImplementation(vi.fn())
import * as commandsApi from '@/features/agent/api/agent-chat-commands'
vi.spyOn(commandsApi, 'switchProvider').mockImplementation(vi.fn())
vi.spyOn(commandsApi, 'resumeChat').mockImplementation(vi.fn())
vi.spyOn(commandsApi, 'submitAgentPrompt').mockImplementation(vi.fn())
const switchToTerminal = vi.spyOn(commandsApi, 'switchToTerminal').mockResolvedValue('pty-1')
vi.spyOn(commandsApi, 'switchToNative').mockResolvedValue()

const handed = vi.hoisted(() => new Set<unknown>())
vi.mock('@/features/agent/chat/agent-chat-view', async () => {
  const { ViewSwitch } = await import('@/features/agent/controls/view-switch')
  return {
    AgentChatView: ({
      active,
      visible,
      presentation,
      splitEnabled,
      onSelectPresentation,
      onOpenTerminal,
      blankSignpost,
    }: {
      active: boolean
      visible: boolean
      presentation: 'chat' | 'terminal' | 'split'
      splitEnabled: boolean
      onSelectPresentation: (next: 'chat' | 'terminal' | 'split') => void
      onOpenTerminal: () => void
      blankSignpost?: ReactNode
    }) =>
      handed.add(onOpenTerminal).add(onSelectPresentation) &&
      createElement(
        'div',
        {
          'data-testid': 'chat-view',
          'data-active': String(active),
          'data-visible': String(visible),
        },
        createElement('textarea', { 'data-testid': 'composer' }),
        blankSignpost,
        presentation === 'terminal'
          ? null
          : createElement(ViewSwitch, {
              presentation,
              splitEnabled,
              onSelect: onSelectPresentation,
            }),
      ),
  }
})

import * as lazyTerminalMod from '@/features/terminal/components/lazy-terminal'
vi.spyOn(lazyTerminalMod, 'LazyXtermTerminal').mockImplementation(
  ({
    sessionId,
    isActive,
    isVisible,
  }: {
    sessionId: string
    isActive: boolean
    isVisible?: boolean
  }) =>
    createElement(
      'div',
      {
        'data-testid': 'xterm',
        'data-session-id': sessionId,
        'data-active': String(isActive),
        'data-visible': String(isVisible),
      },
      createElement('textarea', { 'data-testid': 'tty' }),
    ),
)

import { ChatHeaderSlot, ChatHeaderSlotProvider } from '@/features/agent/hooks/chat-header-slot'
import { AgentChatPaneGate as AgentChatPane } from '@/features/agent/components/agent-chat-pane-gate'
import { chatStore } from '@/features/agent/stores/chat-store'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
import { useTerminalStore } from '@/features/terminal/stores/terminal-store'
import {
  nextVersion,
  provider,
  resetChatStore,
  seedChats,
  setChatTerminalWait,
  setProviders,
} from '@/__tests__/__fixtures__/agent-chat'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'

const providers: AgentProvider[] = [provider('claude', 'Claude')]

function liveChat(): AgentChat {
  return {
    id: 'c1',
    workspaceId: 'w1',
    title: 'Chat c1',
    liveRunnerId: 'r1',
    terminalSessionId: 'pty-1',
    activeProviderId: 'claude',
    working: false,
    version: nextVersion(),
    phase: 'dormant',
    createdAt: '',
    order: 0,
  }
}

function seed() {
  const store = createWorkspaceStore('w1')
  setProviders(providers)
  seedChats([liveChat()])
  return store
}

type Store = ReturnType<typeof seed>

function setSplitEnabled(enabled: boolean) {
  useSettingsStore.setState({
    settings: { ...useSettingsStore.getState().settings, chatSplitPresentationEnabled: enabled },
  })
}

function pane(store: Store, opts: { isActivePane?: boolean; isVisible?: boolean } = {}) {
  return createElement(
    WorkspaceStoreContext.Provider,
    { value: store },
    createElement(AgentChatPane, {
      chatId: 'c1',
      runnerId: 'r1',
      wsId: 'w1',
      paneId: 'b1',
      isActivePane: opts.isActivePane ?? true,
      isVisible: opts.isVisible ?? true,
    }),
  )
}

async function renderPane(store: Store, opts?: { isActivePane?: boolean; isVisible?: boolean }) {
  await act(async () => render(pane(store, opts)))
}

const surface = (name: 'Chat' | 'Terminal' | 'Split') => screen.getByRole('tab', { name })

function showing(): 'chat' | 'terminal' | 'split' {
  if (screen.queryByRole('tab', { name: 'Split' })?.getAttribute('aria-selected') === 'true') {
    return 'split'
  }
  return surface('Terminal').getAttribute('aria-selected') === 'true' ? 'terminal' : 'chat'
}

const chatView = () => screen.getByTestId('chat-view')
const xterm = () => screen.getByTestId('xterm')
const chatHalf = () => screen.getByTestId('agent-chat-surface')
const terminalHalf = () => screen.getByTestId('agent-terminal-surface')

async function choose(name: 'Chat' | 'Terminal' | 'Split') {
  await act(async () => {
    fireEvent.click(surface(name))
  })
}

async function focusInto(testId: string) {
  await act(async () => {
    screen.getByTestId<HTMLTextAreaElement>(testId).focus()
  })
}

beforeEach(() => {
  resetChatStore()
  useTerminalStore.setState({ sessions: new Map() })
  useSettingsStore.setState({
    settings: { ...getDefaultSettingsSnapshot(), chatIsDefaultPresentation: true },
  })
  localStorage.clear()
  setActiveWorkspaceId('w1')
})

afterEach(() => {
  cleanup()
  useSettingsStore.setState({ settings: getDefaultSettingsSnapshot() })
  localStorage.clear()
})

describe('AgentChatPane — with the split view switched off', () => {
  it('is off before anybody touches it', () => {
    expect(getDefaultSettingsSnapshot().chatSplitPresentationEnabled).toBe(false)
  })

  it('offers the two surfaces it has always offered, and no third', async () => {
    await renderPane(seed())

    expect(surface('Chat')).toBeInTheDocument()
    expect(surface('Terminal')).toBeInTheDocument()
    expect(screen.queryByRole('tab', { name: 'Split' })).not.toBeInTheDocument()
    expect(screen.queryByRole('separator')).not.toBeInTheDocument()
  })

  it('keeps exactly one surface live, whichever one is in front', async () => {
    await renderPane(seed())

    expect(showing()).toBe('chat')
    expect(chatView()).toHaveAttribute('data-active', 'true')
    expect(xterm()).toHaveAttribute('data-visible', 'false')
    expect(xterm()).toHaveAttribute('data-active', 'false')
    expect(terminalHalf()).toHaveClass('hidden')

    await choose('Terminal')

    expect(showing()).toBe('terminal')
    expect(chatView()).toHaveAttribute('data-active', 'false')
    expect(chatHalf()).toHaveClass('hidden')
    expect(xterm()).toHaveAttribute('data-visible', 'true')
    expect(xterm()).toHaveAttribute('data-active', 'true')
  })
})

describe('AgentChatPane — terminal controls', () => {
  it('draws the switcher and provider changer in the header row, not under the terminal', async () => {
    await act(async () =>
      render(
        createElement(
          ChatHeaderSlotProvider,
          null,
          createElement('header', { 'data-testid': 'header' }, createElement(ChatHeaderSlot)),
          pane(seed()),
        ),
      ),
    )
    await choose('Terminal')

    const header = screen.getByTestId('header')
    expect(within(header).getByRole('tab', { name: 'Terminal' })).toBeInTheDocument()
    expect(within(header).getByTestId('provider-switch')).toBeInTheDocument()
    expect(within(terminalHalf()).queryByTestId('provider-switch')).not.toBeInTheDocument()
  })
})

describe('AgentChatPane — the split view', () => {
  beforeEach(() => {
    setSplitEnabled(true)
  })

  it('is reachable from the chat’s own switcher', async () => {
    await renderPane(seed())

    expect(surface('Split')).toBeInTheDocument()
    await choose('Split')

    expect(showing()).toBe('split')
    expect(chatHalf()).not.toHaveClass('hidden')
    expect(terminalHalf()).not.toHaveClass('hidden')
  })

  it('runs both surfaces at once', async () => {
    await renderPane(seed())
    await choose('Split')

    expect(chatView()).toHaveAttribute('data-active', 'true')
    expect(chatView()).toHaveAttribute('data-visible', 'true')
    expect(xterm()).toHaveAttribute('data-visible', 'true')
    expect(xterm()).toHaveAttribute('data-session-id', 'pty-1')
  })

  it('puts a draggable divider between them', async () => {
    await renderPane(seed())
    await choose('Split')

    expect(screen.getByRole('separator')).toHaveAttribute('aria-orientation', 'vertical')
  })

  it('hands the surface back to Chat if the dev switch goes off under it', async () => {
    await renderPane(seed())
    await choose('Split')
    expect(showing()).toBe('split')

    await act(async () => {
      setSplitEnabled(false)
    })

    expect(screen.queryByRole('tab', { name: 'Split' })).not.toBeInTheDocument()
    expect(showing()).toBe('chat')
    expect(chatView()).toHaveAttribute('data-active', 'true')
    expect(xterm()).toHaveAttribute('data-visible', 'false')
  })
})

describe('AgentChatPane — a split nobody is looking at', () => {
  beforeEach(() => {
    setSplitEnabled(true)
  })

  it('does not take the keyboard when its pane has no focus', async () => {
    await renderPane(seed(), { isActivePane: false })
    await choose('Split')

    expect(showing()).toBe('split')
    expect(xterm()).toHaveAttribute('data-active', 'false')
  })

  it('goes dormant with the tab that holds it', async () => {
    const store = seed()
    const view = await act(async () => render(pane(store)))
    await act(async () => view.rerender(pane(store, { isVisible: false })))
    await choose('Split')

    expect(showing()).toBe('split')
    expect(xterm()).toHaveAttribute('data-visible', 'false')
    expect(xterm()).toHaveAttribute('data-active', 'false')
    expect(chatView()).toHaveAttribute('data-visible', 'false')
  })
})

describe('AgentChatPane — what it hands the chat view', () => {
  it('hands the same terminal and surface handlers across its renders', async () => {
    const store = seed()
    const view = await act(async () => render(pane(store)))
    handed.clear()
    await act(async () => view.rerender(pane(store, { isActivePane: false })))
    await act(async () => view.rerender(pane(store)))
    expect(handed.size).toBe(2)
  })
})

describe('AgentChatPane — who owns the keyboard in a split', () => {
  beforeEach(() => {
    setSplitEnabled(true)
  })

  it('gives the chat the keyboard by default', async () => {
    await renderPane(seed())
    await choose('Split')

    expect(chatHalf()).toHaveAttribute('data-surface-focused', 'true')
    expect(terminalHalf()).toHaveAttribute('data-surface-focused', 'false')
    expect(xterm()).toHaveAttribute('data-active', 'false')
  })

  it('follows the caret, whichever half the user is typing in', async () => {
    await renderPane(seed())
    await choose('Split')

    await focusInto('tty')
    expect(terminalHalf()).toHaveAttribute('data-surface-focused', 'true')
    expect(chatHalf()).toHaveAttribute('data-surface-focused', 'false')
    expect(xterm()).toHaveAttribute('data-active', 'true')

    await focusInto('composer')
    expect(chatHalf()).toHaveAttribute('data-surface-focused', 'true')
    expect(terminalHalf()).toHaveAttribute('data-surface-focused', 'false')
    expect(xterm()).toHaveAttribute('data-active', 'false')
  })

  it('arrives with the caret where the user just was', async () => {
    await renderPane(seed())
    await choose('Terminal')
    await choose('Split')

    expect(terminalHalf()).toHaveAttribute('data-surface-focused', 'true')
    expect(xterm()).toHaveAttribute('data-active', 'true')
  })
})

describe('AgentChatPane — the escort has nothing to do in a split', () => {
  beforeEach(() => {
    setSplitEnabled(true)
  })

  async function setWait(wait: { kind: string } | null) {
    await act(async () => {
      setChatTerminalWait('c1', wait)
    })
  }

  it('does not collapse the split when the CLI blocks on a prompt', async () => {
    await renderPane(seed())
    await choose('Split')

    await setWait({ kind: 'workspace_trust' })

    expect(showing()).toBe('split')
    expect(screen.getByTestId('agent-terminal-wait')).toBeInTheDocument()
  })

  it('offers no way back once the prompt clears, because it took nobody anywhere', async () => {
    await renderPane(seed())
    await choose('Split')

    await setWait({ kind: 'workspace_trust' })
    await setWait(null)

    expect(showing()).toBe('split')
    expect(screen.queryByRole('button', { name: /return to chat/i })).not.toBeInTheDocument()
  })

  it('points the banner’s button at the terminal’s caret instead', async () => {
    await renderPane(seed())
    await choose('Split')
    await setWait({ kind: 'workspace_trust' })

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /open terminal/i }))
    })

    expect(showing()).toBe('split')
    expect(terminalHalf()).toHaveAttribute('data-surface-focused', 'true')
  })
})

describe('AgentChatPane — a hot-swap chat after a reload', () => {
  it('lands on the recorded surface over the default, and records the next pick', async () => {
    useSettingsStore.setState({
      settings: { ...getDefaultSettingsSnapshot(), chatIsDefaultPresentation: false },
    })
    chatStore.setState({ presentations: {} })
    const store = seed()
    await renderPane(store)
    expect(showing()).toBe('terminal')

    await act(async () => seedChats([{ ...liveChat(), surface: 'chat' }]))
    expect(showing()).toBe('chat')

    await choose('Terminal')
    expect(switchToTerminal).toHaveBeenCalledWith('w1', 'c1')
  })
})
