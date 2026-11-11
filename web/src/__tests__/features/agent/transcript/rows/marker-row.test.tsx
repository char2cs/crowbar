import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow, MarkerKind } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { TranscriptRow } from '@/features/agent/transcript/rows/transcript-row'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { noRowActions } from '@/__tests__/__fixtures__/row-actions'
import { resetChatStore, writeChat } from '@/__tests__/__fixtures__/agent-chat'
import * as chatCommands from '@/features/agent/api/agent-chat-commands'

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
  it('keeps a failed turn as one divider in the board’s words, with no notice of its own', () => {
    const l = ledger()
    const feed = seed([
      l.prompt('r1', 'hi'),
      l.start('turn-1', 'r1'),
      l.row(
        'turn_failed',
        { reason: 'rate_limit', detail: '429 rate_limit_error: limit reached, resets at 7pm' },
        { turnId: 'turn-1' },
      ),
    ])
    draw(markerOf(feed)!)

    expect(screen.getByTestId('agent-failed-divider')).toHaveTextContent(
      'Stopped · usage limit reached',
    )
    expect(screen.queryByTestId('agent-failure-notice')).toBeNull()
    expect(screen.queryByText(/429|rate_limit_error|7pm/)).toBeNull()
    expect(screen.queryByRole('button')).toBeNull()
  })

  const markers = (feed: ReturnType<typeof seed>) =>
    feed.order.filter((id) => feed.items.get(id)?.kind === 'marker')

  // The CLI died under the turn: what it said stays above, and the prompt can go again.
  it('draws a turn its CLI’s exit interrupted as a divider below the reply it had begun', () => {
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
    draw(interrupted!)

    expect(feed.order.some((id) => feed.items.get(id)?.kind === 'text')).toBe(true)
    expect(screen.getByTestId('agent-interrupted-divider')).toHaveTextContent('Interrupted')
    expect(screen.queryByRole('button')).toBeNull()
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

  it('draws a lost session as one divider, with no sentence or button', () => {
    const l = ledger()
    const feed = seed([
      l.row('session_lost', { provider: 'acme', sessionId: 'S1', reason: 'resume_refused' }),
    ])
    draw(markerOf(feed)!)

    expect(screen.getByTestId('agent-session-lost-divider')).toHaveTextContent(
      'Stopped · session lost',
    )
    expect(screen.queryByRole('button')).toBeNull()
  })

  it('draws no exit sentence for a CLI that exited; only one that never started', () => {
    const l = ledger()
    for (const reason of ['exited', 'connection_lost', 'transport_overflow', 'retracted']) {
      chatStore.setState({ feeds: {}, outbox: {} })
      const feed = seed([l.row('runner_exited', { runnerId: 'r1', reason, exitCode: 1 })])
      expect(markerOf(feed), reason).toBeUndefined()
    }
    chatStore.setState({ feeds: {}, outbox: {} })
    const feed = seed([
      ledger().row('runner_exited', { runnerId: 'r2', reason: 'spawn_failed', exitCode: 1 }),
    ])
    draw(markerOf(feed)!)
    expect(screen.getByTestId('agent-exit-divider')).toHaveTextContent(
      'Stopped · The agent could not start',
    )
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

  it('says how many older records an upgrade could not convert', () => {
    const l = ledger()
    const feed = seed([
      l.row('marker', {
        kind: 'legacy_unreadable',
        detail: '3 legacy records could not be converted',
      }),
    ])
    draw(markerOf(feed)!)
    expect(screen.getByTestId('agent-marker-notice')).toHaveTextContent(
      '3 older records could not be converted. They were written before the last upgrade.',
    )
  })

  it('lists the older records on Show them, each in its own words', () => {
    const l = ledger()
    const feed = seed([
      l.row('marker', {
        kind: 'legacy_unreadable',
        detail: '2 legacy records could not be converted',
        records: ['agent_turns u-1: bad time', 'prompt p-2: no turn holds it'],
      }),
    ])
    draw(markerOf(feed)!)
    expect(screen.queryByTestId('agent-marker-records')).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Show them' }))
    expect(screen.getByTestId('agent-marker-records')).toHaveTextContent(
      'agent_turns u-1: bad time',
    )
    fireEvent.click(screen.getByRole('button', { name: 'Hide them' }))
    expect(screen.queryByTestId('agent-marker-records')).toBeNull()
  })

  it('offers Show them only where the upgrade named its records', () => {
    const feed = seed([
      ledger().row('marker', {
        kind: 'legacy_unreadable',
        detail: '3 legacy records could not be converted',
      }),
    ])
    draw(markerOf(feed)!)
    expect(screen.queryByRole('button', { name: 'Show them' })).toBeNull()
  })

  it('says a lost session carried on from Crowbar’s record, never silently', () => {
    const l = ledger()
    const feed = seed([l.row('marker', { kind: 'from_record' })])
    draw(markerOf(feed)!)
    expect(screen.getByTestId('agent-from-record-divider')).toHaveTextContent(
      'Session lost · continued from Crowbar’s record',
    )
  })

  it('says another chat holds the session, not the session id', () => {
    const l = ledger()
    const feed = seed([l.row('marker', { kind: 'session_conflict', detail: 'S-1' })])
    draw(markerOf(feed)!)
    expect(screen.getByTestId('agent-marker-notice')).toHaveTextContent(
      'This session moved to another chat.',
    )
    expect(screen.queryByText(/S-1/)).toBeNull()
  })

  it('opens the chat that holds the session from Open that chat', () => {
    const actions = { ...noRowActions, openChat: vi.fn() }
    const feed = seed([
      ledger().row('marker', { kind: 'session_conflict', detail: 'S-1', owner: 'chat-2' }),
    ])
    draw(markerOf(feed)!, actions)
    fireEvent.click(screen.getByRole('button', { name: 'Open that chat' }))
    expect(actions.openChat).toHaveBeenCalledWith('chat-2')
  })

  it('compacts from a context warning, only while compacting is on offer', () => {
    const said = 'Context is almost full. 4% left. The chat will compact on its own.'
    const feed = seed([ledger().row('marker', { kind: 'warning', detail: said })])
    const id = markerOf(feed)!
    const { unmount } = draw(id)
    expect(screen.queryByRole('button', { name: 'Compact now' })).toBeNull()
    unmount()
    const actions = { ...noRowActions, compact: vi.fn() }
    draw(id, actions)
    fireEvent.click(screen.getByRole('button', { name: 'Compact now' }))
    expect(actions.compact).toHaveBeenCalled()
  })

  // codex's own words (0.164): bold first sentence, the rest on the line after it.
  it('says a warning by its first sentence in bold, the rest beside it, with no Details', () => {
    const said =
      'Codex could not find bubblewrap on PATH. Install bubblewrap with your OS package manager.'
    draw(markerOf(seed([ledger().row('marker', { kind: 'warning', detail: said })]))!)
    const notice = screen.getByTestId('agent-marker-notice')
    expect(notice.querySelector('b')).toHaveTextContent('Codex could not find bubblewrap on PATH.')
    expect(notice).toHaveTextContent('Install bubblewrap with your OS package manager.')
    expect(screen.queryByRole('button', { name: 'Details' })).toBeNull()
  })

  // A move is one line at its newest phase; what a later phase leaves out, its request named.
  describe('a move', () => {
    const asked = {
      phase: 'requested' as const,
      requestId: 'm1',
      to: 'ws-b',
      parentId: 'folder-b',
      order: 2,
      label: 'feature-b',
    }
    const moveLine = (feed: ReturnType<typeof seed>) => {
      expect(feed.order.filter((id) => feed.items.get(id)?.kind === 'marker')).toHaveLength(1)
      draw(markerOf(feed)!)
      return screen.getByTestId('agent-move-divider')
    }
    beforeEach(() => resetChatStore())

    it('says where it goes while it waits for the turn', () => {
      const feed = seed([ledger().row('move', asked)])
      expect(moveLine(feed)).toHaveTextContent('Moving to feature-b after this turn')
    })

    it('says where it went and the folder the chat now works in', () => {
      const l = ledger()
      const done = { phase: 'done' as const, requestId: 'm1', toDir: '/src/b', label: 'feature-b' }
      const feed = seed([l.row('move', asked), l.row('move', done)])
      expect(moveLine(feed)).toHaveTextContent('Moved to feature-b — now working in /src/b')
    })

    it('says a thread made a branch by the branch it became', () => {
      const l = ledger()
      const cut = { phase: 'requested' as const, requestId: 'm1', branchOut: true }
      const done = { phase: 'done' as const, requestId: 'm1', branch: 'heron', toDir: '/src/heron' }
      const feed = seed([l.row('move', cut), l.row('move', done)])
      expect(moveLine(feed)).toHaveTextContent('Now on branch heron — working in /src/heron')
    })

    it('asks again for a branch that could not be made, under the name it asked for', () => {
      const cut = vi.spyOn(chatCommands, 'branchOutChat').mockResolvedValue(undefined)
      writeChat({ id: 'chat-1', workspaceId: 'ws-a' })
      const l = ledger()
      const failed = { phase: 'failed' as const, requestId: 'm1', branchOut: true, branch: 'heron' }
      const feed = seed([l.row('move', { ...failed, reason: 'branch exists' })])
      expect(moveLine(feed)).toHaveTextContent('Making a branch failed: branch exists')
      fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
      expect(cut).toHaveBeenCalledWith('ws-a', 'chat-1', 'heron')
    })

    it('says a withdrawn move was cancelled', () => {
      const l = ledger()
      const feed = seed([
        l.row('move', asked),
        l.row('move', { phase: 'cancelled', requestId: 'm1' }),
      ])
      expect(moveLine(feed)).toHaveTextContent('Move cancelled')
    })

    it('says why a move failed and asks for it again where its request named', () => {
      const place = vi.spyOn(chatCommands, 'setChatPlacement').mockResolvedValue({
        chat: {} as never,
        shifted: [],
      })
      writeChat({ id: 'chat-1', workspaceId: 'ws-a' })
      const l = ledger()
      const failed = { phase: 'failed' as const, requestId: 'm1', reason: 'branch is gone' }
      const feed = seed([l.row('move', asked), l.row('move', failed)])
      expect(moveLine(feed)).toHaveTextContent('Move to feature-b failed: branch is gone')

      fireEvent.click(screen.getByRole('button', { name: 'Retry' }))

      expect(place).toHaveBeenCalledWith('ws-a', 'chat-1', {
        parentId: 'folder-b',
        order: 2,
        workspaceId: 'ws-b',
        label: 'feature-b',
      })
    })

    it('offers no retry while another move is waiting', () => {
      writeChat({ id: 'chat-1', workspaceId: 'ws-a', pendingMove: { requestId: 'm2' } })
      const l = ledger()
      const failed = { phase: 'failed' as const, requestId: 'm1', reason: 'gone' }
      moveLine(seed([l.row('move', asked), l.row('move', failed)]))
      expect(screen.queryByRole('button', { name: 'Retry' })).toBeNull()
    })
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
    from_record: true,
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
