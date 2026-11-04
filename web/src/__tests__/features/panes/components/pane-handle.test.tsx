import { beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
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

  describe('dragging the handle', () => {
    function grab() {
      openSplit()
      render(<PaneHandle pane={pane('chat-2')} />)
      const button = screen.getByRole('button', { name: 'Drag this chat' })
      Element.prototype.setPointerCapture = () => {}
      document.elementsFromPoint = () => []
      fireEvent.pointerDown(button, { button: 0, clientX: 300, clientY: 200, pointerId: 1 })
      act(() => {
        window.dispatchEvent(
          new MouseEvent('pointermove', { clientX: 340, clientY: 240, buttons: 1 }),
        )
      })
    }

    it('previews a tab of ordinary width when there is no strip to take it from', () => {
      grab()
      const ghost = document.querySelector<HTMLElement>('[data-drag-ghost]')!
      expect(ghost.firstElementChild).toHaveStyle({ width: '160px' })
    })

    it('draws its drop line on the body, clear of the handle that fades out', () => {
      grab()
      const line = document.querySelector('[data-drop-indicator]')!
      expect(line.parentElement).toBe(document.body)
    })
  })
})
