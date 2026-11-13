import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { chatStore } from '@/features/agent/stores/chat-store'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import { SendFailureTray } from '@/features/agent/trays/send-failure-tray'

const { retryPrompt, discardPrompt } = vi.hoisted(() => ({
  retryPrompt: vi.fn(),
  discardPrompt: vi.fn(),
}))
vi.mock('@/features/agent/stores/chat-outbox', () => ({ retryPrompt, discardPrompt }))

const failed = (patch: Partial<OutboxItem> = {}): OutboxItem => ({
  requestId: 'r1',
  wsId: 'ws-1',
  text: 'ship it',
  state: 'failed',
  steer: true,
  ...patch,
})

const onEdit = vi.fn()

beforeEach(() => {
  vi.clearAllMocks()
  chatStore.setState({ feeds: {}, outbox: {} })
})

describe('SendFailureTray', () => {
  it('renders nothing while no send has failed', () => {
    chatStore.setState({ outbox: { 'chat-1': [failed({ state: 'queued' })] } })
    const { container } = render(
      <SendFailureTray chatId="chat-1" item={undefined} onEdit={onEdit} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('says a dropped connection in plain words and offers Retry and Discard', () => {
    const item = failed({ error: 'fetch failed: ECONNRESET', sameIdRetry: true })
    render(<SendFailureTray chatId="chat-1" item={item} onEdit={onEdit} />)
    expect(screen.getByTestId('agent-send-failure')).toHaveTextContent('Your message was not sent')
    expect(screen.queryByText(/ECONNRESET/)).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(retryPrompt).toHaveBeenCalledWith('ws-1', 'chat-1', 'r1', true)
    fireEvent.click(screen.getByRole('button', { name: 'Discard' }))
    expect(discardPrompt).toHaveBeenCalledWith('chat-1', 'r1')
  })

  it('offers only Discard when a resend cannot work', () => {
    render(
      <SendFailureTray chatId="chat-1" item={failed({ code: 'session_lost' })} onEdit={onEdit} />,
    )
    expect(screen.getByTestId('agent-send-failure')).toHaveTextContent('The session was lost')
    expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Discard' })).toBeInTheDocument()
  })

  it('Edit puts the failed text back in the composer and discards the item, even when a resend cannot work', () => {
    render(
      <SendFailureTray chatId="chat-1" item={failed({ code: 'session_lost' })} onEdit={onEdit} />,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }))
    expect(discardPrompt).toHaveBeenCalledWith('chat-1', 'r1')
    expect(onEdit).toHaveBeenCalledWith('ship it')
  })
})
