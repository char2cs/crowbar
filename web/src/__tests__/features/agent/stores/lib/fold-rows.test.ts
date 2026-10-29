import { describe, expect, it } from 'vitest'
import { applyRow, applyRows, emptyFeed } from '@/features/agent/stores/lib/fold-rows'
import type { AskItem, TextItem, ToolItem } from '@/features/agent/stores/lib/transcript-items'
import { ledger } from '@/__tests__/__fixtures__/ledger-rows'

describe('applyRow', () => {
  it('orders items by the seq of their first row and never re-sorts', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'hi'),
      l.start('turn-1', 'r1'),
      l.text('turn-1', 'm1', 0, 'Hel'),
      l.tool('turn-1', 'tool-1', 'pre'),
      l.text('turn-1', 'm1', 1, 'lo', true),
      l.tool('turn-1', 'tool-1', 'post'),
    ])
    expect(feed.order).toEqual(['p:r1', 'm:turn-1:m1', 't:tool-1'])
    expect(feed.items['m:turn-1:m1']?.seq).toBe(3)
  })

  it('keeps the p:<requestId> id from user_message through settle', () => {
    const l = ledger()
    let feed = applyRow(emptyFeed(), l.prompt('r1', 'hi'))
    expect(feed.facts.prompts.r1).toEqual({ state: 'pending', seq: 1 })
    feed = applyRow(feed, l.start('turn-1', 'r1'))
    expect(feed.facts.prompts.r1).toEqual({ state: 'delivered', turnId: 'turn-1' })
    feed = applyRow(feed, l.settle('r1'))
    expect(feed.facts.prompts.r1).toEqual({ state: 'settled' })
    expect(feed.order).toEqual(['p:r1'])
    expect(feed.facts.running?.turnId).toBe('turn-1')
  })

  it('reads a prompt settled unconsumed as refused, or retracted when it says so', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'a'),
      l.settle('r1', false, 'runner_exited'),
      l.prompt('r2', 'b'),
      l.settle('r2', false, 'retracted'),
    ])
    expect(feed.facts.prompts.r1).toEqual({ state: 'refused', reason: 'runner_exited' })
    expect(feed.facts.prompts.r2).toEqual({ state: 'retracted' })
  })

  it('delivers a prompt steered into the running turn by its own row', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'a'),
      l.start('turn-1', 'r1'),
      l.row(
        'user_message',
        { requestId: 'r2', text: 'b', origin: 'crowbar', delivery: 'steer' },
        { turnId: 'turn-1' },
      ),
    ])
    expect(feed.facts.prompts.r2).toEqual({ state: 'delivered', turnId: 'turn-1' })
  })

  it('folds numbered chunks into one text and settles on the final chunk', () => {
    const l = ledger()
    let feed = applyRows(emptyFeed(), [
      l.text('turn-1', 'm1', 1, 'lo'),
      l.text('turn-1', 'm1', 0, 'Hel'),
    ])
    let item = feed.items['m:turn-1:m1'] as TextItem
    expect(item).toMatchObject({ text: 'Hello', settled: false })
    feed = applyRow(feed, l.text('turn-1', 'm1', 2, '!', true))
    item = feed.items['m:turn-1:m1'] as TextItem
    expect(item).toMatchObject({ text: 'Hello!', settled: true })
  })

  it('prefers the complete row over chunks once it lands', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.text('turn-1', 'm1', 0, 'Hel'),
      l.text('turn-1', 'm1', 2, 'xx'),
      l.text('turn-1', 'm1', -1, 'Hello world'),
    ])
    expect(feed.items['m:turn-1:m1']).toMatchObject({ text: 'Hello world', settled: true })
  })

  it('settles every text of a turn when the turn ends', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [l.text('turn-1', 'm1', 0, 'Hel'), l.end('turn-1')])
    expect(feed.items['m:turn-1:m1']).toMatchObject({ text: 'Hel', settled: true })
    expect(feed.facts.ended).toBe(1)
  })

  it('reads a tool still running when its turn ended as abandoned', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      l.tool('turn-1', 'done', 'pre'),
      l.tool('turn-1', 'done', 'post'),
      l.tool('turn-1', 'open', 'pre'),
      l.end('turn-1', 'interrupted'),
    ])
    expect((feed.items['t:done'] as ToolItem).status).toBe('ok')
    expect((feed.items['t:open'] as ToolItem).status).toBe('abandoned')
    expect(feed.facts.running).toBeNull()
    expect(feed.order.at(-1)).toBe('k:5')
    expect(feed.items['k:5']).toMatchObject({ kind: 'marker', marker: 'turn_interrupted' })
  })

  it('applies a row once when it arrives in replay and live', () => {
    const l = ledger()
    const rows = [l.prompt('r1', 'hi'), l.text('turn-1', 'm1', 0, 'Hel')]
    const once = applyRows(emptyFeed(), rows)
    const twice = applyRows(once, rows)
    expect(twice).toBe(once)
    expect(twice.order).toEqual(['p:r1', 'm:turn-1:m1'])
  })

  it('keeps the identity of untouched items, order and facts', () => {
    const l = ledger()
    const before = applyRows(emptyFeed(), [
      l.prompt('r1', 'hi'),
      l.start('turn-1', 'r1'),
      l.text('turn-1', 'm1', 0, 'a'),
    ])
    const after = applyRow(before, l.text('turn-1', 'm1', 1, 'b'))
    expect(after.items['p:r1']).toBe(before.items['p:r1'])
    expect(after.items['m:turn-1:m1']).not.toBe(before.items['m:turn-1:m1'])
    expect(after.order).toBe(before.order)
    expect(after.facts).toBe(before.facts)
    expect(after.scoped).toBe(before.scoped)
  })

  it('keeps a subagent scope out of the order', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.row('subagent', { phase: 'start', type: 'explore' }, { turnId: 'turn-1', scope: 'sub-1' }),
      l.row(
        'tool_call',
        { toolId: 'inner', phase: 'pre', name: 'Grep' },
        { turnId: 'turn-1', scope: 'sub-1' },
      ),
    ])
    expect(feed.order).toEqual([])
    expect(feed.scoped['sub-1']).toEqual(['t:inner'])
    expect(Object.keys(feed.facts.liveSubagents)).toEqual(['sub-1'])
  })

  it('opens an ask as a pending choice and resolves it', () => {
    const l = ledger()
    let feed = applyRow(
      emptyFeed(),
      l.row(
        'ask',
        {
          askId: 'q1',
          event: 'permission',
          kind: 'tool_permission',
          toolName: 'Bash',
          answerable: true,
          options: [{ id: 'y', kind: 'allow' }],
        },
        { turnId: 'turn-1' },
      ),
    )
    expect(feed.facts.openAsks).toEqual(['a:q1'])
    expect((feed.items['a:q1'] as AskItem).choice).toMatchObject({
      id: 'q1',
      pending: true,
      answerable: true,
    })
    feed = applyRow(
      feed,
      l.row('ask_resolved', { askId: 'q1', resolution: 'answered', optionIds: ['y'] }),
    )
    expect(feed.facts.openAsks).toEqual([])
    expect((feed.items['a:q1'] as AskItem).choice).toMatchObject({
      pending: false,
      answeredOptionIds: ['y'],
    })
  })

  it('draws a pill marker for a switch and tracks compaction as a fact', () => {
    const l = ledger()
    let feed = applyRow(emptyFeed(), l.row('marker', { kind: 'compaction_started' }))
    expect(feed.facts.compacting).toBe(true)
    expect(feed.order).toEqual([])
    feed = applyRows(feed, [
      l.row('marker', { kind: 'compaction_ended' }),
      l.row('marker', { kind: 'model_changed', detail: 'big' }),
    ])
    expect(feed.facts.compacting).toBe(false)
    expect(feed.order).toEqual(['k:2', 'k:3'])
  })

  it('shows a message that joins its turn reply as that reply', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.text('turn-1', 'final-turn-1', -1, 'Hello'),
      l.row(
        'assistant_text',
        { messageId: 'm2', index: -1, text: 'Hello there', reply: 'final-turn-1' },
        { turnId: 'turn-1' },
      ),
    ])
    expect(feed.order).toEqual(['m:turn-1:final-turn-1'])
    expect(feed.items['m:turn-1:final-turn-1']).toMatchObject({ text: 'Hello there' })
  })
})
