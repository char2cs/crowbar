import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { chatStore } from '@/features/agent/stores/chat-store'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import { QueuedTray } from '@/features/agent/trays/queued-tray'

const item = (requestId: string, text: string): OutboxItem => ({
  requestId,
  wsId: 'ws-1',
  text,
  state: 'queued',
  steer: false,
})

const hold = (items: OutboxItem[]) => chatStore.setState({ outbox: { 'chat-1': items } })

beforeEach(() => chatStore.setState({ outbox: {} }))

describe('QueuedTray', () => {
  it('renders nothing when no message waits', () => {
    const { container } = render(<QueuedTray chatId="chat-1" items={[]} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows one waiting message as a single removable row', () => {
    const items = [item('r1', 'Also update the snapshots')]
    hold(items)
    render(<QueuedTray chatId="chat-1" items={items} />)
    expect(screen.getByText('Queued')).toBeInTheDocument()
    expect(screen.getByText('Also update the snapshots')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Remove queued message' }))
    expect(chatStore.getState().outbox['chat-1']).toBeUndefined()
  })

  it('counts several and removes only the one asked', () => {
    const items = [item('r1', 'first'), item('r2', 'second')]
    hold(items)
    render(<QueuedTray chatId="chat-1" items={items} />)
    expect(screen.getByText('2 queued')).toBeInTheDocument()
    fireEvent.click(screen.getAllByRole('button', { name: 'Remove queued message' })[0]!)
    expect(chatStore.getState().outbox['chat-1']?.map((i) => i.requestId)).toEqual(['r2'])
  })
})
