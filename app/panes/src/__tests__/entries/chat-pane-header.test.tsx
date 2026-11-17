import { render, screen } from '@testing-library/react'
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
})
