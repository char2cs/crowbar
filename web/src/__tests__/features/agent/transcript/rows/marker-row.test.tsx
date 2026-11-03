import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow, MarkerKind } from '@/features/agent/api/chat-rows'
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

beforeEach(() => chatStore.setState({ feeds: {}, outbox: {} }))

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

  const failedTurn = (l: ReturnType<typeof ledger>, n: number) => [
    l.row(
      'user_message',
      { requestId: `r${n}`, text: `prompt ${n}`, origin: 'crowbar' },
      { turnId: `turn-${n}` },
    ),
    l.start(`turn-${n}`, `r${n}`),
    l.row('turn_failed', { reason: 'provider_error' }, { turnId: `turn-${n}` }),
  ]
  const markers = (feed: ReturnType<typeof seed>) =>
    feed.order.filter((id) => feed.items[id]?.kind === 'marker')

  it('offers Retry once, on the newest failed turn only, with that turn’s prompt', () => {
    const l = ledger()
    const feed = seed([...failedTurn(l, 1), ...failedTurn(l, 2)])
    const actions = { ...noRowActions, resend: vi.fn() }
    const [older, newer] = markers(feed)
    const first = draw(older!, actions)
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
    first.unmount()

    draw(newer!, actions)
    const retry = screen.getByRole('button', { name: 'Retry' })
    fireEvent.click(retry)
    fireEvent.click(retry)
    expect(actions.resend).toHaveBeenCalledTimes(1)
    expect(actions.resend).toHaveBeenCalledWith('prompt 2')
  })

  it('offers no Retry once a later prompt was sent', () => {
    const l = ledger()
    const feed = seed([...failedTurn(l, 1), l.prompt('r9', 'something else')])
    draw(markers(feed)[0]!)
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Open terminal' })).toBeInTheDocument()
  })

  it('offers no Retry while a prompt is on its way', () => {
    const l = ledger()
    const feed = seed(failedTurn(l, 1))
    chatStore.setState((s) => ({
      outbox: {
        ...s.outbox,
        'chat-1': [{ requestId: 'q', wsId: 'w', text: 'x', state: 'queued', steer: false }],
      },
    }))
    draw(markers(feed)[0]!)
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
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

  it('says, in the daemon’s words, how many legacy records an upgrade could not convert', () => {
    const l = ledger()
    const feed = seed([
      l.row('marker', {
        kind: 'legacy_unreadable',
        detail: '3 legacy records could not be converted',
      }),
    ])
    draw(markerOf(feed)!)
    expect(screen.getByTestId('agent-marker-divider')).toHaveTextContent(
      '3 legacy records could not be converted',
    )
  })

  // Every marker kind the daemon accepts (written by the api's ledger tests),
  // and whether the transcript draws it; MarkerKind must name exactly these.
  const drawn: Record<MarkerKind, true | string> = {
    compaction_started: 'opens a compaction; its end draws the pill',
    compaction_ended: true,
    provider_switched: true,
    model_changed: true,
    effort_changed: true,
    legacy_unreadable: true,
    stopped: 'the turn it ends draws “you stopped this turn”',
    session_changed: 'the CLI’s own bookkeeping; the transcript reads on',
    session_conflict: 'which chat owns a session is the daemon’s to settle, not the reader’s',
    dead_letter: 'counted in the daemon’s health, not drawn in a chat',
    notification:
      'what the CLI told its user repeats what the transcript shows; a legacy notice outside a turn stays in the ledger only',
  }

  it('draws exactly the marker kinds it decided to, of every kind the daemon writes', () => {
    const fixture = resolve(import.meta.dirname, '../../../../__fixtures__/ledger-kinds.json')
    const { markerKinds } = JSON.parse(readFileSync(fixture, 'utf-8')) as { markerKinds: string[] }
    expect(Object.keys(drawn).sort()).toEqual(markerKinds)
    for (const kind of markerKinds) {
      chatStore.setState({ feeds: {}, outbox: {} })
      const feed = seed([ledger().row('marker', { kind: kind as MarkerKind, detail: 'd' })])
      expect(markerOf(feed) !== undefined, kind).toBe(drawn[kind as MarkerKind] === true)
    }
  })
})
