import { IDBFactory } from 'fake-indexeddb'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { readOlderRows, type RowsPage } from '@/features/agent/api/chat-rows-reads'
import { ApiError } from '@/lib/errors/api-error'
import { attachFeed as attachReal, loadOlder } from '@/features/agent/stores/chat-feed'
import { chatStore } from '@/features/agent/stores/chat-store'
import { streamDaemon } from '@/__tests__/__fixtures__/stream-daemon'
import { resetDB } from '@/lib/persistence/idb'
import { applyRows, ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { applyReady } from '@/features/agent/stores/lib/fold-live'
import { emptyFeed, FeedFold } from '@/features/agent/stores/lib/fold-rows'
import { fillFull, prependOlder } from '@/features/agent/stores/lib/older-rows'
import type {
  ReasoningItem,
  SubagentItem,
  ToolItem,
} from '@/features/agent/stores/lib/transcript-items'

vi.mock('@/features/agent/api/chat-rows-reads', () => ({ readOlderRows: vi.fn() }))

/** A feed that opened on a window: it holds the rows after `from` only. */
function windowOf(rows: LedgerRow[], from: number) {
  const fold = new FeedFold(emptyFeed())
  fold.startAt(from)
  for (const row of rows) fold.apply(row)
  return applyReady(fold.finish(), 'e1')
}

function turns(n: number) {
  const l = ledger()
  const rows: LedgerRow[] = []
  for (let i = 1; i <= n; i++) {
    rows.push(l.prompt(`r${i}`, `q${i}`), l.start(`t${i}`, `r${i}`))
    rows.push(l.text(`t${i}`, 'm1', 0, `a${i}`, true), l.end(`t${i}`))
  }
  return rows
}

describe('prependOlder', () => {
  it('lays an older page above the held rows as the whole feed would have folded them', async () => {
    const rows = turns(4)
    const whole = applyReady(applyRows(emptyFeed(), rows), 'e1')
    const held = windowOf(rows.slice(8), 8)
    const merged = prependOlder(held, rows.slice(0, 8), 0)
    expect(merged.order).toEqual(whole.order)
    expect(merged).toMatchObject({ from: 0, seq: 16, status: 'live', epoch: 'e1' })
    expect(merged.facts.prompts).toEqual(whole.facts.prompts)
    expect(merged.facts.running).toEqual(whole.facts.running)
    for (const id of whole.order) expect(merged.items.get(id)).toEqual(whole.items.get(id))
  })

  it('shares the held items it does not touch, and moves from to where the page begins', async () => {
    const rows = turns(3)
    const held = windowOf(rows.slice(8), 8)
    const merged = prependOlder(held, rows.slice(4, 8), 4)
    expect(merged.from).toBe(4)
    expect(merged.order).toEqual(['p:r2', 'm:t2:m1', 'p:r3', 'm:t3:m1'])
    expect(merged.items.get('p:r3')).toBe(held.items.get('p:r3'))
  })

  it('keeps the held copy of an item both pages name', async () => {
    const rows = turns(2)
    const held = windowOf(rows.slice(4), 4)
    const again = prependOlder(held, rows.slice(0, 4), 0)
    expect(prependOlder(again, rows.slice(0, 4), 0).order).toEqual(again.order)
  })
})

describe('fillFull', () => {
  let l = ledger()
  beforeEach(() => {
    l = ledger()
  })
  const edit = (diff: string, trimmed?: string[]) => ({
    ...l.row(
      'tool_call',
      { toolId: 'x1', phase: 'post', name: 'Edit', kind: 'edit', diff },
      { turnId: 't1' },
    ),
    trimmed,
  })

  it('marks an item whose row was cut with the seq of the whole row', async () => {
    const cut = edit('+a\n', ['diff'])
    const feed = applyRows(emptyFeed(), [cut])
    expect(feed.items.get('t:x1')).toMatchObject({ fullSeq: cut.seq })
  })

  it('swaps in the whole diff, text or result and clears the mark', async () => {
    const cut = edit('+a\n', ['diff'])
    const reasoning = {
      ...l.row('reasoning', { text: 'cut' }, { turnId: 't1' }),
      trimmed: ['text'],
    }
    const spawn = l.row('subagent', { phase: 'start', spawnToolId: 'sp' }, { turnId: 't1' })
    const done = {
      ...l.row('subagent', { phase: 'result', spawnToolId: 'sp', result: 'cut' }, { turnId: 't1' }),
      trimmed: ['result'],
    }
    const feed = applyRows(emptyFeed(), [cut, reasoning, spawn, done])
    const rid = `r:${reasoning.seq}`
    let next = fillFull(feed, 't:x1', { ...cut, body: { ...cut.body, diff: '+a\n+b\n' } })
    next = fillFull(next, rid, { ...reasoning, body: { text: 'whole' }, trimmed: undefined })
    next = fillFull(next, 's:sp', { ...done, body: { ...done.body, result: 'whole' } })
    expect(next.items.get('t:x1') as ToolItem).toMatchObject({ body: { diff: '+a\n+b\n' } })
    expect((next.items.get('t:x1') as ToolItem).fullSeq).toBeUndefined()
    expect(next.items.get(rid) as ReasoningItem).toMatchObject({ text: 'whole' })
    expect(next.items.get('s:sp') as SubagentItem).toMatchObject({ result: 'whole' })
    expect((next.items.get('s:sp') as SubagentItem).fullSeq).toBeUndefined()
  })

  it('swaps in the whole output of a previewed call and clears the mark', async () => {
    const cut = {
      ...l.row(
        'tool_call',
        { toolId: 'o1', phase: 'post', name: 'Bash', output: 'cut' },
        { turnId: 't1' },
      ),
      trimmed: ['output'],
    }
    const feed = applyRows(emptyFeed(), [cut])
    expect((feed.items.get('t:o1') as ToolItem).fullSeq).toBe(cut.seq)
    const next = fillFull(feed, 't:o1', { ...cut, body: { ...cut.body, output: 'whole' } })
    expect(next.items.get('t:o1') as ToolItem).toMatchObject({ body: { output: 'whole' } })
    expect((next.items.get('t:o1') as ToolItem).fullSeq).toBeUndefined()
  })

  it('leaves the feed as it was for an item that is gone', async () => {
    const feed = applyRows(emptyFeed(), [])
    expect(fillFull(feed, 't:nope', edit('x'))).toBe(feed)
  })
})

describe('older turns over REST', () => {
  const SCOPE = 'chat:chat-1'
  const feed = () => chatStore.getState().feeds['chat-1']
  const push = (frame: object) => streamDaemon.deliver({ scope: SCOPE, ...frame })
  const subs = () => streamDaemon.sent.filter((r) => r.op === 'sub' && r.scope === SCOPE)
  const held: (() => void)[] = []
  const attachFeed = async (chatId: string) => {
    held.push(attachReal('local', chatId))
    await vi.waitFor(() => expect(subs().length).toBeGreaterThan(0))
  }

  beforeEach(() => {
    globalThis.indexedDB = new IDBFactory()
    resetDB()
    chatStore.setState({ feeds: {} })
    vi.mocked(readOlderRows).mockReset()
    streamDaemon.answer = () => []
    streamDaemon.deliver({ t: 'ready', epoch: 'e1' })
    streamDaemon.sent.length = 0
  })
  afterEach(() => held.splice(0).forEach((detach) => detach()))

  const l = ledger('chat-1', 39)
  const [p39, s39, p41, s41] = [
    l.prompt('r39', 'older'),
    l.start('turn-39', 'r39'),
    l.prompt('r41', 'newest'),
    l.start('turn-41', 'r41'),
  ]
  const opened = async () => {
    await attachFeed('chat-1')
    push({ t: 'row', row: p41, from: 40 })
    push({ t: 'row', row: s41 })
    push({ t: 'synced', seq: 42 })
  }
  it('opens on a window and pages older turns over REST, never through the stream', async () => {
    await opened()
    expect(feed()).toMatchObject({ status: 'live', from: 40, seq: 42, order: ['p:r41'] })
    vi.mocked(readOlderRows).mockResolvedValue({ rows: [p39, s39], from: 38, more: true })

    await loadOlder('chat-1')
    expect(readOlderRows).toHaveBeenCalledWith('local', 'chat-1', 40)
    expect(feed()).toMatchObject({ from: 38, seq: 42, order: ['p:r39', 'p:r41'] })
    expect(subs()).toHaveLength(1)
  })

  it('holds the chat from its start once a page says nothing is older', async () => {
    await opened()
    vi.mocked(readOlderRows).mockResolvedValue({ rows: [p39, s39], from: 38, more: false })
    await loadOlder('chat-1')
    expect(feed()?.from).toBe(0)
    await loadOlder('chat-1')
    expect(readOlderRows).toHaveBeenCalledTimes(1)
  })

  it('asks once at a time, and again after a page failed', async () => {
    await opened()
    const page = { rows: [p39, s39], from: 38, more: true }
    vi.mocked(readOlderRows).mockRejectedValueOnce(new ApiError('offline', 503))
    await loadOlder('chat-1')
    expect(feed()).toMatchObject({ from: 40, order: ['p:r41'] })

    vi.mocked(readOlderRows).mockResolvedValue(page)
    await Promise.all([loadOlder('chat-1'), loadOlder('chat-1')])
    expect(readOlderRows).toHaveBeenCalledTimes(2)
    expect(feed()?.from).toBe(38)
  })

  it('drops a page that lands after the feed was replayed under it', async () => {
    await opened()
    let land!: (page: RowsPage) => void
    vi.mocked(readOlderRows).mockReturnValue(new Promise((resolve) => (land = resolve)))
    const loading = loadOlder('chat-1')
    push({ t: 'closed', reason: 'resync', seq: 42 })
    land({ rows: [p39, s39], from: 38, more: true })
    await loading
    expect(feed()?.order).toEqual(['p:r41'])
  })

  it('asks for no older turns when the feed holds the chat from its start', async () => {
    await attachFeed('chat-1')
    push({ t: 'row', row: ledger().prompt('r1', 'hi') })
    push({ t: 'synced', seq: 1 })
    await loadOlder('chat-1')
    expect(readOlderRows).not.toHaveBeenCalled()
  })
})
