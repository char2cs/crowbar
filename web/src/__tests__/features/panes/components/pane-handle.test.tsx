import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PaneHandle } from '@/features/panes/components/pane-handle'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

const actions = () => windowPaneStore.getState().paneActions
const pane = (chatId: string) => {
  const { panes } = windowPaneStore.getState()
  return panes[chatPaneIndex(panes).get(chatId)!]
}

beforeEach(() => {
  useSidebarStore.setState(getInitialState())
  resetWindowPaneStoreForTests()
  actions().setActiveProject('p1')
})

function openSplit() {
  actions().openChat('chat-1', { workspaceId: 'ws-1' })
  actions().dropChatOnPane('chat-2', pane('chat-1').id, 'right', 'ws-1')
}

describe('PaneHandle', () => {
  it('is absent on a pane that is the only chat in its view', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    const { container } = render(<PaneHandle pane={pane('chat-1')} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('offers drag, own-tab and close on a split pane', () => {
    openSplit()
    render(<PaneHandle pane={pane('chat-2')} />)
    expect(screen.getByRole('button', { name: 'Drag this chat' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Open in its own tab' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Close this chat' })).toBeInTheDocument()
  })

  it('"own tab" detaches the pane into a view of its own', async () => {
    openSplit()
    const before = windowPaneStore.getState().viewOrder.length
    render(<PaneHandle pane={pane('chat-2')} />)
    await userEvent.click(screen.getByRole('button', { name: 'Open in its own tab' }))
    expect(windowPaneStore.getState().viewOrder.length).toBe(before + 1)
  })

  it('close ends just that chat and leaves its split partner', async () => {
    openSplit()
    render(<PaneHandle pane={pane('chat-2')} />)
    await userEvent.click(screen.getByRole('button', { name: 'Close this chat' }))
    const index = chatPaneIndex(windowPaneStore.getState().panes)
    expect(index.has('chat-2')).toBe(false)
    expect(index.has('chat-1')).toBe(true)
  })
})
