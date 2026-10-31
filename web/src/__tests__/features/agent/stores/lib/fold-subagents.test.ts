import { describe, expect, it } from 'vitest'
import { emptyFeed, FeedFold } from '@/features/agent/stores/lib/fold-rows'
import type {
  PlanSnapshotItem,
  ReasoningItem,
  SubagentItem,
} from '@/features/agent/stores/lib/transcript-items'
import { applyRows, ledger, subagentSessionRows } from '@/__tests__/__fixtures__/ledger-rows'

const spawn = (
  l: ReturnType<typeof ledger>,
  toolId: string,
  phase: 'pre' | 'post',
  scope?: string,
) =>
  l.row(
    'tool_call',
    { toolId, phase, name: 'Agent', kind: 'subagent', title: 'Find tests', spawnsScope: scope },
    { turnId: 'turn-1' },
  )

describe('subagent cards', () => {
  it('creates a subagent card on its spawn and keeps its key when it binds', () => {
    const l = ledger()
    let feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      spawn(l, 'call-1', 'pre'),
      l.row(
        'subagent',
        { phase: 'start', spawnToolId: 'call-1', title: 'Find tests' },
        { turnId: 'turn-1' },
      ),
    ])
    expect(feed.order).toEqual(['s:call-1'])
    expect(feed.items['t:call-1']).toBeUndefined()
    expect(feed.items['s:call-1']).toMatchObject({
      kind: 'subagent',
      status: 'starting',
      title: 'Find tests',
    })
    feed = applyRows(feed, [
      l.row('subagent', { phase: 'start', type: 'explore' }, { turnId: 'turn-1', scope: 'sub-1' }),
      spawn(l, 'call-1', 'post', 'sub-1'),
      l.row(
        'subagent',
        { phase: 'bound', spawnToolId: 'call-1' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
    ])
    expect(feed.order).toEqual(['s:call-1'])
    expect(feed.items['s:call-1']).toMatchObject({
      status: 'running',
      scope: 'sub-1',
      type: 'explore',
    })
    expect(feed.facts.liveSubagents).toEqual(['s:call-1'])
  })

  it('attaches child rows that precede their card bind', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      spawn(l, 'call-1', 'pre'),
      l.row(
        'tool_call',
        { toolId: 'early', phase: 'pre', name: 'Read', kind: 'read' },
        {
          turnId: 'turn-1',
          scope: 'sub-1',
        },
      ),
      l.row(
        'subagent',
        { phase: 'bound', spawnToolId: 'call-1' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
      l.row(
        'tool_call',
        { toolId: 'late', phase: 'pre', name: 'Read', kind: 'read' },
        {
          turnId: 'turn-1',
          scope: 'sub-1',
        },
      ),
    ])
    const card = feed.items['s:call-1'] as SubagentItem
    expect(card.scope).toBe('sub-1')
    expect(feed.scoped[card.scope!]).toEqual(['t:early', 't:late'])
    expect(feed.order).toEqual(['s:call-1'])
  })

  it('marks running cards interrupted when the turn is stopped', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      spawn(l, 'call-1', 'pre'),
      l.row(
        'subagent',
        { phase: 'bound', spawnToolId: 'call-1' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
      l.row('subagent', { phase: 'start', type: 'plan' }, { turnId: 'turn-1', scope: 'sub-2' }),
      l.row(
        'subagent',
        { phase: 'stop', status: 'interrupted' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
      l.end('turn-1', 'interrupted'),
      l.row('runner_exited', { runnerId: 'r1', reason: 'stopped', exitCode: 0 }),
    ])
    expect(feed.items['s:call-1']).toMatchObject({ status: 'interrupted' })
    expect(feed.items['s:sub-2']).toMatchObject({ status: 'interrupted' })
    expect(feed.facts.liveSubagents).toEqual([])
  })

  it('marks a card abandoned when its parent turn ends before it binds', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      spawn(l, 'call-1', 'pre'),
      l.end('turn-1'),
    ])
    expect(feed.items['s:call-1']).toMatchObject({ status: 'abandoned' })
    expect(feed.facts.liveSubagents).toEqual([])
  })

  it('keeps a bound card running past its parent turn and finishes it with its result', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      spawn(l, 'call-1', 'pre'),
      l.row(
        'subagent',
        { phase: 'bound', spawnToolId: 'call-1' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
      l.end('turn-1'),
      l.row(
        'subagent',
        { phase: 'result', result: 'Found 3', usage: { tokens: 1200, toolUses: 2 } },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
      l.row('subagent', { phase: 'stop', result: 'later' }, { turnId: 'turn-1', scope: 'sub-1' }),
    ])
    expect(feed.items['s:call-1']).toMatchObject({
      status: 'done',
      result: 'Found 3',
      usage: { tokens: 1200, toolUses: 2 },
    })
  })

  it('sets parent waiting while the parent addresses the subagent', () => {
    const l = ledger()
    const wait = (phase: 'pre' | 'post') =>
      l.row(
        'tool_call',
        { toolId: 'w1', phase, name: 'wait', addressesScope: 'sub-1' },
        { turnId: 'turn-1' },
      )
    let feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      spawn(l, 'call-1', 'pre'),
      l.row(
        'subagent',
        { phase: 'bound', spawnToolId: 'call-1' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
      wait('pre'),
    ])
    expect(feed.items['s:call-1']).toMatchObject({ parentWaiting: true })
    feed = applyRows(feed, [wait('post')])
    expect(feed.items['s:call-1']).toMatchObject({ parentWaiting: false })
  })

  it('folds the recorded session into two cards that each hold their own calls and result', () => {
    const feed = applyRows(emptyFeed(), subagentSessionRows())
    const cards = feed.order
      .map((id) => feed.items[id])
      .filter((item): item is SubagentItem => item?.kind === 'subagent')
    expect(cards.map((card) => card.status)).toEqual(['done', 'done'])
    for (const card of cards) {
      expect(
        feed.scoped[card.scope!]?.filter((id) => feed.items[id]?.kind === 'tool'),
      ).toHaveLength(3)
      expect(card.result).toMatch(/alpha/)
    }
    expect(cards[1]?.type).toBe('general-purpose')
    expect(feed.facts.liveSubagents).toEqual([])
  })
})

describe('reasoning and plan rows', () => {
  it('folds a reasoning row into a collapsed item above its turn reply', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'hi'),
      l.start('turn-1', 'r1'),
      l.text('turn-1', 'm1', 0, 'Hello', true),
      l.row('reasoning', { messageId: 'm1', text: 'Thinking hard' }, { turnId: 'turn-1' }),
      l.end('turn-1'),
    ])
    expect(feed.order).toEqual(['p:r1', 'r:turn-1:m1', 'm:turn-1:m1'])
    const item = feed.items['r:turn-1:m1'] as ReasoningItem
    expect(item).toMatchObject({ kind: 'reasoning', text: 'Thinking hard' })
    expect(Date.parse(item.endedAt) - Date.parse(item.startedAt)).toBe(2000)
  })

  it('leaves a snapshot of the main plan in the transcript when its turn ends', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      l.row(
        'plan',
        { op: 'upsert', items: [{ id: '1', text: 'Read', status: 'active' }] },
        { turnId: 'turn-1' },
      ),
      l.row(
        'plan',
        { op: 'upsert', items: [{ id: '2', text: 'Gone', status: 'pending' }] },
        { turnId: 'turn-1' },
      ),
      l.row(
        'plan',
        { op: 'upsert', items: [{ id: '1', text: 'Read', status: 'done' }] },
        { turnId: 'turn-1' },
      ),
      l.row(
        'plan',
        { op: 'upsert', items: [{ id: '2', text: 'Gone', status: 'removed' }] },
        { turnId: 'turn-1' },
      ),
      l.end('turn-1'),
    ])
    expect(feed.order).toEqual(['l:turn-1'])
    expect((feed.items['l:turn-1'] as PlanSnapshotItem).items).toEqual([
      { id: '1', text: 'Read', status: 'done' },
    ])
  })

  // The strip and the working line read the live plan: it ends with its turn.
  const ends = {
    completed: (l: ReturnType<typeof ledger>) => l.end('turn-1'),
    interrupted: (l: ReturnType<typeof ledger>) => l.end('turn-1', 'interrupted'),
    failed: (l: ReturnType<typeof ledger>) =>
      l.row('turn_failed', { reason: 'provider_error' }, { turnId: 'turn-1' }),
  }
  it.each(Object.entries(ends))(
    'ends the live plan with a %s turn, live and on reload',
    (_, end) => {
      const l = ledger()
      const rows = [
        l.start('turn-1'),
        l.row(
          'plan',
          { op: 'replace', items: [{ id: '1', text: 'Fix', status: 'active' }] },
          { turnId: 'turn-1' },
        ),
        end(l),
      ]
      const live = rows.reduce((feed, row) => applyRows(feed, [row]), emptyFeed())
      const fold = new FeedFold(emptyFeed())
      for (const row of rows) fold.apply(row)
      for (const feed of [live, fold.finish()]) {
        expect(feed.facts.plan['']).toBeUndefined()
        expect((feed.items['l:turn-1'] as PlanSnapshotItem).items).toEqual([
          { id: '1', text: 'Fix', status: 'active' },
        ])
      }
    },
  )
})
