import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import {
  ChatIdContext,
  useChatId,
} from '@/features/agent/composer/plate/attachments/chat-id-context'

function Probe() {
  const chatId = useChatId()
  return <span data-testid="probe">{chatId ?? 'none'}</span>
}

describe('useChatId', () => {
  it('returns null outside a provider', () => {
    render(<Probe />)
    expect(screen.getByTestId('probe').textContent).toBe('none')
  })

  it('returns the id supplied by an ancestor provider', () => {
    render(
      <ChatIdContext.Provider value="c1">
        <Probe />
      </ChatIdContext.Provider>,
    )
    expect(screen.getByTestId('probe').textContent).toBe('c1')
  })
})
