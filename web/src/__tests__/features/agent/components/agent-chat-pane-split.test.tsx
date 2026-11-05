// Split is a diagnostic that sets the chat beside the CLI's own TUI. With its switch
// off the pane must stay what it always was: two surfaces, one live, one dormant.
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

// Both surfaces are stubbed to the ONE thing this file is about: the liveness
// each of them is told it has, plus something real to put the caret in. The chat
// view in particular is stubbed rather than rendered for real because `active`
// is exactly the prop under test — a stub reports it, a real transcript only
// implies it.
vi.mock('@/features/agent/chat/agent-chat-view', async () => {
  const { ViewSwitcher } = await import('@/features/agent/controls/view-switcher')
  return {
    AgentChatView: ({
      active,
      visible,
      presentation,
      splitEnabled,
      onSelectPresentation,
      blankSignpost,
    }: {
      active: boolean
      visible: boolean
      presentation: 'chat' | 'terminal' | 'split'
      splitEnabled: boolean
      onSelectPresentation: (next: 'chat' | 'terminal' | 'split') => void
      blankSignpost?: ReactNode
    }) =>
      createElement(
        'div',
        {
          'data-testid': 'chat-view',
          'data-active': String(active),
          'data-visible': String(visible),
        },
        createElement('textarea', { 'data-testid': 'composer' }),
        // Every seeded chat is blank, so the pane's wait banner renders inside the view.
        blankSignpost,
        // The real switcher: it is a pane control, and this file is its contract.
        presentation === 'terminal'
          ? null
          : createElement(ViewSwitcher, {
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

// Both real descriptors declare hotswap:true — see
// TestShippedDescriptors_DeclareHotswapTrue on the backend — which is what
// offers split at all; this suite is about split's OWN behaviour, not the gate.
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

/** Turn the Developer tab's switch on or off, the way the settings row does. */
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

/** A string `name` is matched against the WHOLE accessible name, which is what
 *  keeps 'Terminal' off the wait banner's own 'Open Terminal' button. */
const surface = (name: 'Chat' | 'Terminal' | 'Split') => screen.getByRole('tab', { name })

/** Which surface the switcher says is showing. Read off aria-pressed rather than
 *  a class, because the class is exactly the thing under test. */
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

/** Real DOM focus, not a synthetic `focus` event: jsdom's own focus() fires the
 *  bubbling `focusin` React actually listens to, which is the whole mechanism
 *  the split's keyboard ownership rides on. */
async function focusInto(testId: string) {
  await act(async () => {
    screen.getByTestId<HTMLTextAreaElement>(testId).focus()
  })
}

beforeEach(() => {
  resetChatStore()
  useTerminalStore.setState({ sessions: new Map() })
  // These cases are about the Chat surface, so they land on it.
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
    // The divider belongs to the split and to nothing else.
    expect(screen.queryByRole('separator')).not.toBeInTheDocument()
  })

  // The invariant that matters most: one surface live, the other dormant.
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

  // The point of the instrument: NEITHER surface is resting. A split where one
  // half is dormant would show a stale TUI beside a live chat and invent
  // discrepancies rather than reveal them.
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

  // isActive is what makes xterm seize the caret, with retries. A pane without
  // focus must never do that, split or not.
  it('does not take the keyboard when its pane has no focus', async () => {
    await renderPane(seed(), { isActivePane: false })
    await choose('Split')

    expect(showing()).toBe('split')
    expect(xterm()).toHaveAttribute('data-active', 'false')
  })

  // A hidden tab is still MOUNTED, for keep-alive. Nothing structural stops it
  // rendering, so the props have to.
  it('goes dormant with the tab that holds it', async () => {
    await renderPane(seed(), { isVisible: false })
    await choose('Split')

    expect(showing()).toBe('split')
    expect(xterm()).toHaveAttribute('data-visible', 'false')
    expect(xterm()).toHaveAttribute('data-active', 'false')
    expect(chatView()).toHaveAttribute('data-visible', 'false')
  })
})

describe('AgentChatPane — who owns the keyboard in a split', () => {
  beforeEach(() => {
    setSplitEnabled(true)
  })

  // Two live surfaces means two things that can eat a keystroke. The composer
  // wins by default: typing a prompt into a TUI by accident is the worse of the
  // two mistakes, and it is silent.
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

  // Arriving from the terminal, the user was mid-sentence at the CLI.
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

  // An escort exists only because the terminal is SOMEWHERE ELSE. In a split it
  // is on screen already, so moving anybody would mean taking away the chat half
  // they are deliberately watching.
  it('does not collapse the split when the CLI blocks on a prompt', async () => {
    await renderPane(seed())
    await choose('Split')

    await setWait({ kind: 'workspace_trust' })

    expect(showing()).toBe('split')
    expect(screen.getByTestId('agent-terminal-wait')).toBeInTheDocument()
  })

  // And having moved nobody, it owes nobody a way back: the "return to chat"
  // offer over a surface with the chat already on it would be nonsense.
  it('offers no way back once the prompt clears, because it took nobody anywhere', async () => {
    await renderPane(seed())
    await choose('Split')

    await setWait({ kind: 'workspace_trust' })
    await setWait(null)

    expect(showing()).toBe('split')
    expect(screen.queryByRole('button', { name: /return to chat/i })).not.toBeInTheDocument()
  })

  // The banner's button still has to do SOMETHING useful. There is nowhere to
  // send anybody, so it sends the caret.
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

  // The control: with the split OFF this is the escort that has always run.
  it('still escorts a user who is on Chat alone', async () => {
    const store = seed()
    setSplitEnabled(false)
    await renderPane(store)
    expect(showing()).toBe('chat')

    await setWait({ kind: 'workspace_trust' })
    expect(showing()).toBe('terminal')

    await setWait(null)
    expect(showing()).toBe('chat')
  })
})
