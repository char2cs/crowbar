import { createElement } from 'react'
import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'

vi.mock('@/features/agent/components/agent-chat-pane', () => ({
  AgentChatPane: () => createElement('div', { 'data-testid': 'chat-surface' }),
}))

type Module = typeof import('@/features/panes/components/pane-chat-view') &
  typeof import('@/features/panes/lib/chat-surface-loader')

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
  beforeEach(() => vi.useRealTimers())

  it('waits for the chat surface chunk when it was not preloaded', async () => {
    const mod = await freshModule()
    renderView(mod)
    expect(screen.queryByTestId('chat-surface')).toBeNull()

    await act(async () => {})

    expect(screen.getByTestId('chat-surface')).toBeTruthy()
  })

  it('renders the chat surface in the first commit once it was preloaded', async () => {
    const mod = await freshModule()
    await mod.preloadAgentChatPane()

    renderView(mod)

    expect(screen.getByTestId('chat-surface')).toBeTruthy()
  })
})
