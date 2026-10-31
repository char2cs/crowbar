import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { TranscriptRow } from '@/features/agent/transcript/rows/transcript-row'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { noRowActions } from '@/__tests__/__fixtures__/row-actions'

function seed(rows: LedgerRow[]) {
  openFeed('chat-1')
  for (const row of rows) applyFeedFrame('chat-1', { type: 'row', row })
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
  return chatStore.getState().feeds['chat-1']!
}

function draw(id: string, actions = noRowActions) {
  return render(<TranscriptRow actions={actions} chatId="chat-1" id={id} providers={[]} />)
}

const markerOf = (feed: ReturnType<typeof seed>) =>
  feed.order.find((id) => feed.items[id]?.kind === 'marker')

beforeEach(() => chatStore.setState({ feeds: {} }))

describe('MarkerRow', () => {
  it('offers Retry and Open terminal on a failed turn', () => {
    const l = ledger()
    const feed = seed([
      l.row(
        'user_message',
        { requestId: 'r1', text: 'fix the build', origin: 'crowbar' },
        { turnId: 'turn-1' },
      ),
      l.start('turn-1', 'r1'),
      l.row(
        'turn_failed',
        { reason: 'provider_error', detail: 'rate limited\nretry later' },
        { turnId: 'turn-1' },
      ),
    ])
    const actions = { ...noRowActions, resend: vi.fn(), openTerminal: vi.fn() }
    draw(markerOf(feed)!, actions)

    expect(screen.getByTestId('agent-failed-divider')).toHaveTextContent('Turn failed')
    expect(screen.queryByText(/rate limited/)).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Details' }))
    expect(screen.getByText(/rate limited/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(actions.resend).toHaveBeenCalledWith('fix the build')
    fireEvent.click(screen.getByRole('button', { name: 'Open terminal' }))
    expect(actions.openTerminal).toHaveBeenCalled()
  })

  it('reads Interrupted · you stopped this turn after Stop', () => {
    const l = ledger()
    const feed = seed([
      l.start('turn-1'),
      l.row('turn_ended', { status: 'interrupted', reason: 'stopped' }, { turnId: 'turn-1' }),
    ])
    draw(markerOf(feed)!)
    expect(screen.getByTestId('agent-interrupted-divider')).toHaveTextContent(
      'Interrupted · you stopped this turn',
    )
  })

  it('offers Start a new conversation on a lost session, while nothing follows it', () => {
    const l = ledger()
    const feed = seed([
      l.row('session_lost', { provider: 'acme', sessionId: 'S1', reason: 'resume_refused' }),
    ])
    const actions = { ...noRowActions, newConversation: vi.fn() }
    const view = draw(markerOf(feed)!, actions)

    expect(screen.getByTestId('agent-session-lost-divider')).toHaveTextContent(
      'The session was lost',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Start a new conversation' }))
    expect(actions.newConversation).toHaveBeenCalled()

    applyFeedFrame('chat-1', { type: 'row', row: l.prompt('r2', 'again') })
    view.rerender(
      <TranscriptRow actions={actions} chatId="chat-1" id={markerOf(feed)!} providers={[]} />,
    )
    expect(screen.queryByRole('button', { name: 'Start a new conversation' })).toBeNull()
  })

  it('says an exit nobody asked for, and none for a runner that exited retracted', () => {
    const l = ledger()
    let feed = seed([l.row('runner_exited', { runnerId: 'r1', reason: 'retracted', exitCode: 0 })])
    expect(markerOf(feed)).toBeUndefined()
    feed = seed([
      l.row('runner_exited', { runnerId: 'r2', reason: 'connection_lost', exitCode: 1 }),
    ])
    draw(markerOf(feed)!)
    expect(screen.getByTestId('agent-exit-divider')).toHaveTextContent(
      'Crowbar lost its connection to the agent',
    )
    expect(screen.getByRole('button', { name: 'Open terminal' })).toBeInTheDocument()
  })
})
