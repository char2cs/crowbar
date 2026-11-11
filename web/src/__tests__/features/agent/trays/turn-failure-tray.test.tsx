import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, openFeed } from '@/features/agent/stores/chat-store'
import { AgentSelectionPicker } from '@/features/agent/controls/agent-selection-picker'
import { TopTray } from '@/features/agent/trays/top-tray'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { noRowActions } from '@/__tests__/__fixtures__/row-actions'
import { provider, resetChatStore } from '@/__tests__/__fixtures__/agent-chat'

function seed(rows: LedgerRow[]) {
  openFeed('chat-1')
  for (const row of rows) applyFeedFrame('chat-1', { type: 'row', row })
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
}

function show(providers = [provider('fake', 'Claude')], actions = noRowActions) {
  return render(
    <TopTray
      chatId="chat-1"
      plan={undefined}
      providers={providers}
      actions={actions}
      onEdit={() => {}}
    />,
  )
}

const failed = (reason: string, detail = '', from = ledger()) => [
  from.row(
    'user_message',
    { requestId: 'r1', text: 'hi', origin: 'crowbar' },
    { turnId: 'turn-1' },
  ),
  from.start('turn-1', 'r1'),
  from.row('turn_failed', { reason, detail }, { turnId: 'turn-1' }),
]

beforeEach(() => resetChatStore())

describe('the stopped turn’s notice', () => {
  it('says a usage limit in two plain lines with Switch model, and no code or raw text', () => {
    seed(failed('rate_limit', '429 rate_limit_error: limit reached. Your limit will reset at 7pm.'))
    show([provider('fake', 'Claude', { modelSelect: true, models: ['opus'] })])

    expect(screen.getByTestId('agent-failed-reason')).toHaveTextContent('Usage limit reached')
    expect(screen.getByTestId('agent-failed-hint')).toHaveTextContent(
      /Resets at .*Another model can keep going now\./,
    )
    expect(screen.getByRole('button', { name: 'Switch model' })).toBeInTheDocument()
    expect(screen.queryByText(/429|rate_limit_error/)).toBeNull()
    expect(screen.queryByRole('button', { name: /details|open terminal/i })).toBeNull()
  })

  it('opens the footer’s model picker from Switch model', () => {
    seed(failed('rate_limit'))
    const providers = [provider('fake', 'Claude', { modelSelect: true, models: ['opus'] })]
    render(
      <AgentSelectionPicker
        provider={providers[0]}
        providers={providers}
        model="opus"
        effort=""
        permission={{ wsId: 'ws-1', chatId: 'chat-1' }}
        onSelectionChange={() => {}}
      />,
    )
    show(providers)
    const trigger = screen.getByRole('button', { name: /Agent:/ })
    expect(trigger).toHaveAttribute('aria-expanded', 'false')
    fireEvent.click(screen.getByRole('button', { name: 'Switch model' }))
    expect(trigger).toHaveAttribute('aria-expanded', 'true')
  })

  it('offers no Switch model where no provider has a model to pick', () => {
    seed(failed('rate_limit'))
    show()
    expect(screen.queryByRole('button', { name: 'Switch model' })).toBeNull()
  })

  it('names the provider a signed-out person must sign in to, and opens its terminal', () => {
    seed(failed('401 Unauthorized'))
    const actions = { ...noRowActions, openTerminal: vi.fn() }
    show([provider('fake', 'Claude')], actions)

    expect(screen.getByTestId('agent-failed-reason')).toHaveTextContent('Signed out of')
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))
    expect(actions.openTerminal).toHaveBeenCalled()
  })

  it('words a provider fault plainly and retries the prompt once', () => {
    seed(failed('provider_error', 'API Error: 500 Internal server error'))
    const actions = { ...noRowActions, retry: vi.fn() }
    show([provider('fake', 'Claude')], actions)

    expect(screen.getByTestId('agent-failed-reason')).toHaveTextContent(
      'Claude had a problem on its side',
    )
    expect(screen.queryByText(/500|Internal server error/)).toBeNull()
    const retry = screen.getByRole('button', { name: 'Retry' })
    fireEvent.click(retry)
    fireEvent.click(retry)
    expect(actions.retry).toHaveBeenCalledTimes(1)
    expect(actions.retry).toHaveBeenCalledWith('r1')
  })

  it('goes once a later prompt is sent or on its way', () => {
    const l = ledger()
    seed([...failed('provider_error', '', l), l.prompt('r2', 'again')])
    const { container } = show()
    expect(container).toBeEmptyDOMElement()
  })

  it('has nothing to say about a turn the person stopped', () => {
    const l = ledger()
    seed([
      l.row('user_message', { requestId: 'r1', text: 'x', origin: 'crowbar' }, { turnId: 't1' }),
      l.start('t1', 'r1'),
      l.row('turn_ended', { status: 'interrupted', reason: 'stopped' }, { turnId: 't1' }),
    ])
    const { container } = show()
    expect(container).toBeEmptyDOMElement()
  })

  it('offers a new conversation on a lost session', () => {
    seed([ledger().row('session_lost', { provider: 'acme', sessionId: 'S1', reason: 'x' })])
    const actions = { ...noRowActions, newConversation: vi.fn() }
    show([provider('fake', 'Claude')], actions)
    fireEvent.click(screen.getByRole('button', { name: 'New conversation' }))
    expect(actions.newConversation).toHaveBeenCalled()
  })
})
