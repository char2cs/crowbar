import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow, MarkerKind } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { TranscriptRow } from '@/features/agent/transcript/rows/transcript-row'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { noRowActions } from '@/__tests__/__fixtures__/row-actions'
import { provider } from '@/__tests__/__fixtures__/agent-chat'
import * as chatEntity from '@/features/agent/hooks/use-chat-entity'

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
  feed.order.find((id) => feed.items.get(id)?.kind === 'marker')

beforeEach(() => chatStore.setState({ feeds: {}, outbox: {} }))
afterEach(() => vi.restoreAllMocks())

describe('MarkerRow', () => {
  it('says why a turn failed, in the provider’s words, and retries that prompt', () => {
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
    const actions = { ...noRowActions, retry: vi.fn(), openTerminal: vi.fn() }
    draw(markerOf(feed)!, actions)

    expect(screen.getByTestId('agent-failed-divider')).toHaveTextContent('Turn failed')
    expect(screen.getByTestId('agent-failed-reason')).toHaveTextContent('rate limited')
    fireEvent.click(screen.getByRole('button', { name: 'Details' }))
    expect(screen.getByText(/retry later/)).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(actions.retry).toHaveBeenCalledWith('r1')
    fireEvent.click(screen.getByRole('button', { name: 'Open terminal' }))
    expect(actions.openTerminal).toHaveBeenCalled()
  })

  const failure = (reason: string, detail: string) => {
    const l = ledger()
    return seed([
      l.prompt('r1', 'hi'),
      l.start('turn-1', 'r1'),
      l.row('turn_failed', { reason, detail }, { turnId: 'turn-1' }),
    ])
  }
  const codex = provider('fake', 'Codex', { signIn: 'codex login' })

  it('tells a signed-out person how to sign in, with the provider’s own command', () => {
    const detail = 'unexpected status 401 Unauthorized: Missing bearer, cf-ray: a5678360fab4b516'
    const feed = failure(detail, detail)
    render(
      <TranscriptRow
        actions={noRowActions}
        chatId="chat-1"
        id={markerOf(feed)!}
        providers={[codex]}
      />,
    )

    expect(screen.getByTestId('agent-failed-hint')).toHaveTextContent(
      'Sign in: run codex login in a terminal',
    )
  })

  // A failure before its turn started: no turn names the provider, the CLI does.
  it('still says how to sign in when the failure refused the prompt that follows it', () => {
    const l = ledger()
    const detail = 'unexpected status 401 Unauthorized'
    const feed = seed([
      l.row('runner_started', {
        runnerId: 'run-1',
        provider: 'fake',
        terminalSession: '',
        launchModel: '',
        launchEffort: '',
        launchPermissionLevel: '',
        resumeSessionId: '',
      }),
      l.row(
        'user_message',
        { requestId: 'r1', text: 'hi', origin: 'crowbar' },
        { turnId: 'turn-1' },
      ),
      l.row('turn_failed', { reason: detail, detail }, { turnId: 'fresh-1' }),
      l.row('prompt_settled', { requestId: 'r1', consumed: false, reason: 'turn_failed' }),
    ])
    render(
      <TranscriptRow
        actions={noRowActions}
        chatId="chat-1"
        id={markerOf(feed)!}
        providers={[codex]}
      />,
    )
    expect(screen.getByTestId('agent-failed-hint')).toHaveTextContent('Sign in: run codex login')
  })

  it('says when a rate limit lifts, and gives no sign-in hint for it', () => {
    vi.spyOn(chatEntity, 'useChatTelemetry').mockReturnValue({
      observedAt: '2026-08-24T18:00:00Z',
      source: 'callback',
      rateLimits: [{ usedPercent: 100, resetsAt: '2026-08-24T19:00:00Z' }],
    } as never)
    const feed = failure('rate_limit', 'Rate limit reached. Please try again later.')
    render(
      <TranscriptRow
        actions={noRowActions}
        chatId="chat-1"
        id={markerOf(feed)!}
        providers={[codex]}
      />,
    )

    expect(screen.getByTestId('agent-failed-hint')).toHaveTextContent(/^Resets /)
    expect(screen.queryByText(/Sign in/)).toBeNull()
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
    feed.order.filter((id) => feed.items.get(id)?.kind === 'marker')

  it('offers Retry once, on the newest failed turn only, with that turn’s prompt', () => {
    const l = ledger()
    const feed = seed([...failedTurn(l, 1), ...failedTurn(l, 2)])
    const actions = { ...noRowActions, retry: vi.fn() }
    const [older, newer] = markers(feed)
    const first = draw(older!, actions)
    expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
    first.unmount()

    draw(newer!, actions)
    const retry = screen.getByRole('button', { name: 'Retry' })
    fireEvent.click(retry)
    fireEvent.click(retry)
    expect(actions.retry).toHaveBeenCalledTimes(1)
    expect(actions.retry).toHaveBeenCalledWith('r2')
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

  // The CLI died under the turn: what it said stays above, and the prompt can go again.
  it('offers Retry on a turn its CLI’s exit interrupted, below the reply it had begun', () => {
    const l = ledger()
    const feed = seed([
      l.row(
        'user_message',
        { requestId: 'r1', text: 'write a story', origin: 'crowbar' },
        { turnId: 'turn-1' },
      ),
      l.start('turn-1', 'r1'),
      l.text('turn-1', 'm1', 0, 'Once upon a', false),
      l.row('turn_ended', { status: 'interrupted', reason: 'runner_exited' }, { turnId: 'turn-1' }),
      l.row('runner_exited', { runnerId: 'run-1', reason: 'exited', exitCode: 137 }),
    ])
    const [interrupted] = markers(feed)
    const actions = { ...noRowActions, retry: vi.fn() }
    draw(interrupted!, actions)

    expect(feed.order.some((id) => feed.items.get(id)?.kind === 'text')).toBe(true)
    expect(screen.getByTestId('agent-interrupted-divider')).toHaveTextContent('Interrupted')
    fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(actions.retry).toHaveBeenCalledWith('r1')
  })

  it('offers no Retry on a turn the person stopped', () => {
    const l = ledger()
    const feed = seed([
      l.row('user_message', { requestId: 'r1', text: 'x', origin: 'crowbar' }, { turnId: 't1' }),
      l.start('t1', 'r1'),
      l.row('turn_ended', { status: 'interrupted', reason: 'stopped' }, { turnId: 't1' }),
    ])
    draw(markerOf(feed)!)
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

  it('reads a switch of provider, model and effort as one divider', () => {
    const l = ledger()
    const feed = seed([
      l.row('marker', { kind: 'provider_switched', detail: 'codex' }),
      l.row('marker', { kind: 'model_changed', detail: 'gpt-big' }),
      l.row('marker', { kind: 'effort_changed', detail: 'low' }),
    ])
    render(
      <TranscriptRow
        actions={noRowActions}
        chatId="chat-1"
        id={markerOf(feed)!}
        providers={[{ id: 'codex', displayName: 'Codex' } as never]}
      />,
    )

    expect(screen.getByTestId('agent-switch-divider')).toHaveTextContent(
      'Switched to Codex · gpt-big · low effort',
    )
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

  it('says the chat kept its session and ended the CLI that left it, not the session id', () => {
    const l = ledger()
    const feed = seed([l.row('marker', { kind: 'session_conflict', detail: 'S-1' })])
    draw(markerOf(feed)!)
    expect(screen.getByTestId('agent-session-conflict-divider')).toHaveTextContent(
      'Another chat holds that session; its CLI was ended so nothing goes unrecorded',
    )
  })

  // codex's own words (0.164): the pill says its first sentence; the rest waits behind Details.
  it('says a warning by its first sentence, the provider’s whole message one click away', () => {
    const said =
      'Codex could not find bubblewrap on PATH. Install bubblewrap with your OS package manager. ' +
      'Codex will use the bundled bubblewrap in the meantime.'
    draw(markerOf(seed([ledger().row('marker', { kind: 'warning', detail: said })]))!)
    expect(screen.getByTestId('agent-warning')).toHaveTextContent(
      /^Codex could not find bubblewrap on PATH\.$/,
    )
    fireEvent.click(screen.getByRole('button', { name: 'Details' }))
    expect(screen.getByText(/use the bundled bubblewrap in the meantime/)).toBeInTheDocument()
  })

  it('offers no Details for a warning said in one sentence', () => {
    draw(markerOf(seed([ledger().row('marker', { kind: 'warning', detail: 'Model changed.' })]))!)
    expect(screen.getByTestId('agent-warning')).toHaveTextContent('Model changed.')
    expect(screen.queryByRole('button', { name: 'Details' })).toBeNull()
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
    session_conflict: true,
    dead_letter: 'counted in the daemon’s health, not drawn in a chat',
    warning: true,
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
