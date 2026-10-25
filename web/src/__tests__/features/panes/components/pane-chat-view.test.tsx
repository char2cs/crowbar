import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { useSidebarFocusMode } from '@/components/layout/use-sidebar-focus-mode'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'
import { useSettingsStore } from '@/features/settings/store'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@/features/agent/components/agent-chat-pane-gate', () => ({
  AgentChatPaneGate: () =>
    createElement(
      'div',
      null,
      createElement('p', null, 'transcript message'),
      createElement('textarea', { 'data-testid': 'chat-surface' }),
    ),
}))

type Module = typeof import('@/features/panes/components/pane-chat-view') &
  typeof import('@/features/panes/lib/chat-surface-loader')

// Collection has no test timeout: transform the whole graph here, so a test's timer
// measures its render and act rather than a cold import under a loaded run.
await Promise.all([
  import('@/features/panes/components/pane-chat-view'),
  import('@/features/panes/lib/chat-surface-loader'),
])

// Each test starts from a module that has not loaded the chat surface yet.
async function freshModule(): Promise<Module> {
  vi.resetModules()
  const [view, loader] = await Promise.all([
    import('@/features/panes/components/pane-chat-view'),
    import('@/features/panes/lib/chat-surface-loader'),
  ])
  return { ...view, ...loader }
}

function renderView({ PaneChatView }: Module) {
  render(
    createElement(PaneChatView, {
      ref: null,
      paneId: 'p1',
      chatId: 'c1',
      runnerId: '',
      wsId: 'w1',
      chatWsId: 'w1',
      chatStore: createWorkspaceStore('w1'),
      hidden: false,
      basis: null,
      alongsideEditor: false,
      chatFillsPane: true,
      isBottomPane: false,
      isActivePane: true,
      isVisible: true,
    }),
  )
}

describe('PaneChatView chat surface loading', () => {
  let mod: Module
  beforeEach(async () => {
    vi.useRealTimers()
    mod = await freshModule()
  })

  it('waits for the chat surface chunk when it was not preloaded', async () => {
    renderView(mod)
    expect(screen.queryByTestId('chat-surface')).toBeNull()

    await act(async () => {})

    expect(screen.getByTestId('chat-surface')).toBeTruthy()
  })

  it('renders the chat surface in the first commit once it was preloaded', async () => {
    await mod.preloadAgentChatPane()

    renderView(mod)

    expect(screen.getByTestId('chat-surface')).toBeTruthy()
  })
})

describe('PaneChatView sidebar surface', () => {
  // The mode follows focus only with the opt-in setting on.
  beforeEach(() => {
    useSettingsStore.setState({
      settings: { ...getDefaultSettingsSnapshot(), sidebarFollowsFocus: true },
    })
  })
  function FocusMode() {
    useSidebarFocusMode()
    return null
  }

  async function renderChatColumn() {
    const mod = await freshModule()
    await mod.preloadAgentChatPane()
    useSidebarStore.setState({ ...getInitialState(), activeTab: 'files' })
    render(createElement(FocusMode))
    renderView(mod)
  }

  it('focus in the chat column shows Chats', async () => {
    await renderChatColumn()
    // Focus only moves the mode once the user has pressed something.
    fireEvent.pointerDown(document.body)
    act(() => screen.getByTestId('chat-surface').focus())
    expect(useSidebarStore.getState().activeTab).toBe('chats')
  })

  it('a click on transcript text with nothing focusable under the pointer shows Chats', async () => {
    await renderChatColumn()
    await userEvent.click(screen.getByText('transcript message'))
    expect(useSidebarStore.getState().activeTab).toBe('chats')
  })
})
