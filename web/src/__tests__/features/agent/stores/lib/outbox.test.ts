import { describe, expect, it } from 'vitest'
import { applyRows, ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'
import {
  deliveryPending,
  newestRetractable,
  nextToSend,
  onRowsApplied,
  onSendFailed,
  onSending,
  onSent,
  refusal,
  retryFailed,
  retryRow,
  SESSION_LOST_TEXT,
  type OutboxItem,
} from '@/features/agent/stores/lib/outbox'
import type { UserItem } from '@/features/agent/stores/lib/transcript-items'

const item = (requestId: string, extra: Partial<OutboxItem> = {}): OutboxItem => ({
  requestId,
  wsId: 'w1',
  text: `text of ${requestId}`,
  state: 'queued',
  steer: false,
  ...extra,
})

const idle = () => emptyFeed().facts

function running(requestId = 'r0') {
  const l = ledger()
  return applyRows(emptyFeed(), [l.prompt(requestId, 'earlier'), l.start('turn-1', requestId)])
}

describe('nextToSend', () => {
  it('sends the head only when no turn runs and no prompt is pending', () => {
    const items = [item('a'), item('b')]
    expect(nextToSend(items, emptyFeed(), false)?.requestId).toBe('a')
    expect(nextToSend(items, running(), false)).toBeUndefined()

    const l = ledger()
    const pending = applyRows(emptyFeed(), [l.prompt('r0', 'held')])
    expect(nextToSend(items, pending, false)).toBeUndefined()
    expect(nextToSend(onSending(items, 'a'), emptyFeed(), false)).toBeUndefined()
  })

  it('holds the head while the chat compacts or a terminal prompt waits', () => {
    const l = ledger()
    const compacting = applyRows(emptyFeed(), [l.row('marker', { kind: 'compaction_started' })])
    expect(nextToSend([item('a')], compacting, false)).toBeUndefined()
    expect(nextToSend([item('a')], emptyFeed(), true)).toBeUndefined()
  })

  it('holds the head while a reply streams ahead of its rows', () => {
    const streaming = { ...emptyFeed(), liveTail: ['m:turn-1:m1'] }
    expect(nextToSend([item('a')], streaming, false)).toBeUndefined()
    expect(nextToSend([item('a', { steer: true })], streaming, false)?.requestId).toBe('a')
  })

  it('steers a steerable prompt at once while a turn runs', () => {
    expect(nextToSend([item('a', { steer: true })], running(), false)?.requestId).toBe('a')
  })

  it('never lets a later prompt overtake a failed head', () => {
    const items = onSendFailed([item('a'), item('b')], 'a', { status: 400, message: 'no' })
    expect(nextToSend(items, emptyFeed(), false)).toBeUndefined()
  })

  it('sends the next prompt on the turn_ended row', () => {
    const l = ledger()
    const rows = [l.prompt('r0', 'earlier'), l.start('turn-1', 'r0')]
    const busy = applyRows(emptyFeed(), rows)
    expect(nextToSend([item('a')], busy, false)).toBeUndefined()
    const done = applyRows(busy, [l.end('turn-1')])
    expect(nextToSend([item('a')], done, false)?.requestId).toBe('a')
  })
})

describe('send outcomes', () => {
  it('keeps a prompt queued when the send answers busy', () => {
    const items = onSendFailed(onSending([item('a')], 'a'), 'a', {
      status: 409,
      code: 'chat_busy',
      message: 'busy: conflict',
    })
    expect(items[0]).toMatchObject({ state: 'queued', requestId: 'a' })
    expect(items[0]?.error).toBeUndefined()
  })

  it('marks a prompt failed with the service reason on any other error', () => {
    const items = onSendFailed([item('a')], 'a', {
      status: 400,
      code: 'invalid_argument',
      message: 'model is not in the catalogue',
    })
    expect(items[0]).toMatchObject({ state: 'failed', error: 'model is not in the catalogue' })
  })

  it('C27: a send refused with session_lost says the session was lost, never that the provider may have it', () => {
    const items = onSendFailed([item('a')], 'a', {
      status: 409,
      code: 'session_lost',
      message: 'prompt: session lost: conflict',
    })
    expect(items[0]?.error).toBe(SESSION_LOST_TEXT)
    expect(items[0]?.error).not.toMatch(/may already have/i)
    expect(retryFailed(items, 'a', 'fresh')[0]).toMatchObject({
      requestId: 'fresh',
      state: 'queued',
    })
  })

  it('C27: Retry after request_id_conflict uses a fresh request id', () => {
    for (const code of ['request_id_conflict', 'request_outcome_uncertain'] as const) {
      const items = onSendFailed([item('a')], 'a', { status: 409, code, message: 'refused' })
      expect(retryFailed(items, 'a', 'fresh')[0]?.requestId).toBe('fresh')
    }
  })

  it('retries a send that failed on a network error under the same request id', () => {
    const offline = onSendFailed([item('a')], 'a', { message: 'Failed to fetch' })
    expect(retryFailed(offline, 'a', 'fresh')[0]).toMatchObject({ requestId: 'a', state: 'queued' })
    const crashed = onSendFailed([item('a')], 'a', { status: 502, message: 'bad gateway' })
    expect(retryFailed(crashed, 'a', 'fresh')[0]?.requestId).toBe('a')
  })

  it('C27: Retry on a settled or refused prompt row sends the text under a fresh request id', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'one'),
      l.settle('r1'),
      l.prompt('r2', 'two'),
      l.settle('r2', false, 'session_lost'),
      l.prompt('r3', 'three'),
    ])
    const fresh = item('fresh')
    for (const id of ['p:r1', 'p:r2']) {
      const row = feed.items[id] as UserItem
      expect(retryRow([], feed.facts, row, fresh)).toEqual([{ ...fresh, retryOf: row.requestId }])
    }
    expect(retryRow([], feed.facts, feed.items['p:r3'] as UserItem, fresh)).toEqual([])
  })
})

describe('onRowsApplied', () => {
  it('hands the prompt to the rows once its user_message lands', () => {
    const items = onSent(onSending([item('a'), item('b')], 'a'), 'a')
    const l = ledger()
    const before = applyRows(emptyFeed(), [l.start('turn-0'), l.end('turn-0')])
    expect(onRowsApplied(items, before.facts)).toBe(items)
    const landed = applyRows(before, [l.prompt('a', 'text of a')])
    expect(onRowsApplied(items, landed.facts).map((left) => left.requestId)).toEqual(['b'])
  })
})

describe('newestRetractable', () => {
  it('takes back the newest unsent prompt before anything the daemon holds', () => {
    const items = [onSent(onSending([item('a')], 'a'), 'a')[0]!, item('b')]
    expect(newestRetractable(items, emptyFeed())).toEqual({ kind: 'unsent', item: items[1] })
  })

  it('waits for an in-flight send, and retracts one the daemon acknowledged', () => {
    const sending = onSending([item('a')], 'a')
    expect(newestRetractable(sending, emptyFeed())?.kind).toBe('in_flight')
    expect(newestRetractable(onSent(sending, 'a'), emptyFeed())).toEqual({
      kind: 'held',
      requestId: 'a',
      text: 'text of a',
    })
  })

  it('retracts a pending prompt, or a delivered one whose turn has said nothing', () => {
    const l = ledger()
    const pending = applyRows(emptyFeed(), [l.prompt('r1', 'one')])
    expect(newestRetractable([], pending)).toMatchObject({ kind: 'held', requestId: 'r1' })
    const delivered = applyRows(pending, [l.start('turn-1', 'r1')])
    expect(newestRetractable([], delivered)).toEqual({
      kind: 'held',
      requestId: 'r1',
      text: 'one',
      turnId: 'turn-1',
    })
    const spoke = applyRows(delivered, [l.text('turn-1', 'm1', 0, 'Working on it')])
    expect(newestRetractable([], spoke)).toEqual({ kind: 'stop' })
    expect(newestRetractable([], applyRows(spoke, [l.end('turn-1')]))).toBeNull()
  })
})

describe('deliveryPending', () => {
  it('is true from the send until a turn takes the prompt', () => {
    const l = ledger()
    const pending = applyRows(emptyFeed(), [l.prompt('r1', 'one')])
    expect(deliveryPending(onSending([item('a')], 'a'), idle())).toBe(true)
    expect(deliveryPending([], pending.facts)).toBe(true)
    expect(deliveryPending([item('a')], applyRows(pending, [l.start('t', 'r1')]).facts)).toBe(false)
  })
})

describe('refusal', () => {
  it('refuses an empty box silently, and a photo still uploading or an oversized prompt aloud', () => {
    expect(refusal('')).toBe('')
    expect(refusal('see ![shot.png](blob:local-id)')).toMatch(/finish uploading/)
    expect(refusal('x'.repeat(64 * 1024 + 1))).toMatch(/too large/)
    expect(refusal('see ![shot.png](chats/c1/attachments/shot.png)')).toBeNull()
  })
})
