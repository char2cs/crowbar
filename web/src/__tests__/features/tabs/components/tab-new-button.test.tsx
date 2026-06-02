import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import TabNewButton from '@/features/tabs/components/tab-new-button'

const baseProps = {
  isBottomPane: false,
  disablePaneActions: false,
  isInSplit: false,
  onNewConversation: vi.fn(),
  onNewTerminal: vi.fn(),
  onOpenUrl: vi.fn(),
  onClosePane: vi.fn(),
}

describe('TabNewButton', () => {
  it('renders the + trigger button', () => {
    render(<TabNewButton {...baseProps} />)
    expect(screen.getByRole('button', { name: 'New tab' })).toBeDefined()
  })

  it('calls onNewConversation when "New Conversation" is clicked', async () => {
    const onNewConversation = vi.fn()
    render(<TabNewButton {...baseProps} onNewConversation={onNewConversation} />)
    const trigger = screen.getByRole('button', { name: 'New tab' })
    await userEvent.click(trigger)
    const newConversationItem = await screen.findByText('New Conversation')
    await userEvent.click(newConversationItem)
    expect(onNewConversation).toHaveBeenCalledOnce()
  })
})
