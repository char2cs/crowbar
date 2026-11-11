import { act, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { chatStore } from '@/features/agent/stores/chat-store'
import { TopTray } from '@/features/agent/trays/top-tray'

const plan = [{ id: '1', text: 'Survey docs', status: 'active' }]

beforeEach(() => chatStore.setState({ feeds: {}, outbox: {} }))

describe('TopTray', () => {
  it('renders nothing when no section has anything to say', () => {
    const { container } = render(<TopTray chatId="chat-1" plan={undefined} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('holds the plan, queued messages and the failure slot in one tray', () => {
    render(<TopTray chatId="chat-1" plan={plan} failure={<p>Sign in again</p>} />)
    act(
      () =>
        void chatStore.setState({
          outbox: {
            'chat-1': [
              { requestId: 'r1', wsId: 'ws', text: 'later', state: 'queued', steer: false },
            ],
          },
        }),
    )
    const tray = screen.getByTestId('agent-top-tray')
    expect(tray).toHaveTextContent('Sign in again')
    expect(tray).toHaveTextContent('0 of 1 done')
    expect(tray).toHaveTextContent('later')
    expect(document.querySelectorAll('[data-testid="agent-top-tray"]')).toHaveLength(1)
  })

  it('shows a failure alone', () => {
    render(<TopTray chatId="chat-1" plan={undefined} failure={<p>Sign in again</p>} />)
    expect(screen.getByTestId('agent-top-tray')).toHaveTextContent('Sign in again')
  })
})
