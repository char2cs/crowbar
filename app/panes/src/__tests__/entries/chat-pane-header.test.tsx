import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import {
  ChatHeaderSlotProvider,
  useChatHeaderSlotElement,
} from '@/features/agent/hooks/chat-header-slot'

vi.mock('@/features/tabs/components/chat-branch-header', () => ({
  ChatBranchHeader: ({ chatId }: { chatId: string }) => <div data-testid="branch">{chatId}</div>,
}))

const { ChatPaneHeader } = await import('@/entries/chat-pane-header')

function SlotProbe() {
  const slot = useChatHeaderSlotElement()
  return <div data-testid="slot">{slot ? 'mounted' : 'none'}</div>
}

describe('ChatPaneHeader', () => {
  it('names the chat and hosts the slot the chat surface draws its switch into', () => {
    render(
      <ChatHeaderSlotProvider>
        <ChatPaneHeader chatId="chat-1" wsId="ws-1" />
        <SlotProbe />
      </ChatHeaderSlotProvider>,
    )
    expect(screen.getByTestId('branch').textContent).toBe('chat-1')
    expect(screen.getByTestId('slot').textContent).toBe('mounted')
  })

  it('offers to open the editor only while it is given the way to', async () => {
    const onOpenEditor = vi.fn()
    const { rerender } = render(
      <ChatHeaderSlotProvider>
        <ChatPaneHeader chatId="chat-1" wsId="ws-1" />
      </ChatHeaderSlotProvider>,
    )
    expect(screen.queryByRole('button', { name: 'Open editor' })).not.toBeInTheDocument()
    rerender(
      <ChatHeaderSlotProvider>
        <ChatPaneHeader chatId="chat-1" wsId="ws-1" onOpenEditor={onOpenEditor} />
      </ChatHeaderSlotProvider>,
    )
    await userEvent.click(screen.getByRole('button', { name: 'Open editor' }))
    expect(onOpenEditor).toHaveBeenCalledOnce()
  })
})
