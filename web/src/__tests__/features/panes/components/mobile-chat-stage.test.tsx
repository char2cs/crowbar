import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
// The chat surface has its own suite; this one is about WHICH chat the phone shows.
vi.mock('@/features/panes/components/pane-chat-view', () => ({
  PaneChatView: (props: { chatId: string; alongsideEditor: boolean; chatFillsPane: boolean }) =>
    createElement('div', {
      'data-testid': 'chat-view',
      'data-chat': props.chatId,
      'data-header': String(props.alongsideEditor || props.chatFillsPane),
    }),
}))

const { MobileChatStage } = await import('@/features/panes/components/mobile-chat-stage')

// The window's ambient workspace, as WorkspaceHost provides it.
const ambient = createWorkspaceStore('ws-1')
const renderStage = () =>
  render(
    <WorkspaceStoreContext.Provider value={ambient}>
      <MobileChatStage />
    </WorkspaceStoreContext.Provider>,
  )

const actions = () => windowPaneStore.getState().paneActions
const open = (chatId: string) =>
  act(() => actions().openChat(chatId, { projectId: 'p1', workspaceId: 'ws-1' }))

beforeEach(() => resetWindowPaneStoreForTests())
afterEach(() => resetWindowPaneStoreForTests())

describe('MobileChatStage', () => {
  it('shows the focused chat alone, with no pane header', () => {
    renderStage()
    open('c1')
    open('c2')
    const views = screen.getAllByTestId('chat-view')
    expect(views).toHaveLength(1)
    expect(views[0]).toHaveAttribute('data-chat', 'c2')
    expect(views[0]).toHaveAttribute('data-header', 'false')
  })

  // A split restored from an earlier session still has one focused chat.
  it('collapses a split to its focused pane', () => {
    open('c1')
    const c1Pane = chatPaneIndex(windowPaneStore.getState().panes).get('c1')!
    act(() => actions().dropChatOnPane('c2', c1Pane, 'right', 'ws-1'))
    act(() => actions().setActivePane(c1Pane))
    renderStage()
    const views = screen.getAllByTestId('chat-view')
    expect(views).toHaveLength(1)
    expect(views[0]).toHaveAttribute('data-chat', 'c1')
  })

  it('offers the chat list while no chat is open', () => {
    useMobileScreenStore.setState({ chatListOpen: false })
    renderStage()
    expect(screen.queryByTestId('chat-view')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: /choose a chat/i }))
    expect(useMobileScreenStore.getState().chatListOpen).toBe(true)
  })
})
