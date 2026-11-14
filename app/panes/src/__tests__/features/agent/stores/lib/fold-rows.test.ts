import { describe, expect, it } from 'vitest'
import { applyRow, emptyFeed, FeedFold } from '@/features/agent/stores/lib/fold-rows'
import type { AskItem, TextItem, ToolItem } from '@/features/agent/stores/lib/transcript-items'
import { applyRows, ledger } from '@/__tests__/__fixtures__/ledger-rows'

describe('applyRow', () => {
  it('orders items by the seq of their first row and never re-sorts', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'hi'),
      l.start('turn-1'),
      l.text('turn-1', 'm1', 0, 'Hel'),
      l.tool('turn-1', 'tool-1', 'pre'),
      l.text('turn-1', 'm1', 1, 'lo', true),
      l.tool('turn-1', 'tool-1', 'post'),
    ])
    expect(feed.order).toEqual(['p:r1', 'm:turn-1:m1', 't:tool-1'])
    expect(feed.items.get('m:turn-1:m1')?.seq).toBe(3)
  })

  // A provider may report a one-paragraph message only when its response ended: after
  // the calls it made had started, and sometimes after they had ended.
  it('reads a message reported whole after its calls above them, a closing reply below', () => {
    const l = ledger()
    const tools = (id: string) => [l.tool('turn-1', id, 'pre'), l.tool('turn-1', id, 'post')]
    const whole = (id: string) => l.text('turn-1', id, 0, id, true)
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      ...tools('s0'),
      l.tool('turn-1', 's1', 'pre'),
      whole('A'),
      l.tool('turn-1', 's1', 'post'),
      ...tools('f'),
      whole('B'),
      ...tools('z'),
      whole('C'),
      l.end('turn-1'),
      l.start('turn-2'),
      l.tool('turn-2', 'g', 'pre'),
      l.tool('turn-2', 'g', 'post'),
      l.text('turn-2', 'D', 0, 'D', true),
      ...[l.tool('turn-2', 'h', 'pre'), l.tool('turn-2', 'h', 'post'), l.end('turn-2')],
    ])
    const turn1 = ['t:s0', 'm:turn-1:A', 't:s1', 'm:turn-1:B', 't:f', 't:z', 'm:turn-1:C']
    expect(feed.order).toEqual([...turn1, 'm:turn-2:D', 't:g', 't:h'])
    const m = ledger()
    // Only a one-chunk message may have waited for its response's end; text
    // streamed or completed after its calls ended was said after them.
    const streamed = applyRows(emptyFeed(), [
      ...[m.tool('turn-3', 'a', 'pre'), m.tool('turn-3', 'a', 'post')],
      m.text('turn-3', 'chunked', 0, 'first part'),
      ...[m.tool('turn-4', 'b', 'pre'), m.tool('turn-4', 'b', 'post')],
      m.text('turn-4', 'complete', -1, 'reported complete'),
    ])
    expect(streamed.order).toEqual(['t:a', 'm:turn-3:chunked', 't:b', 'm:turn-4:complete'])
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

  it('folds numbered chunks into one text and settles on the final chunk', () => {
    const l = ledger()
    let feed = applyRows(emptyFeed(), [
      l.text('turn-1', 'm1', 1, 'lo'),
      l.text('turn-1', 'm1', 0, 'Hel'),
    ])
    let item = feed.items.get('m:turn-1:m1') as TextItem
    expect(item).toMatchObject({ text: 'Hello', settled: false })
    feed = applyRow(feed, l.text('turn-1', 'm1', 2, '!', true))
    item = feed.items.get('m:turn-1:m1') as TextItem
    expect(item).toMatchObject({ text: 'Hello!', settled: true })
  })

  it('prefers the complete row over chunks once it lands', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.text('turn-1', 'm1', 0, 'Hel'),
      l.text('turn-1', 'm1', 2, 'xx'),
      l.text('turn-1', 'm1', -1, 'Hello world'),
    ])
    expect(feed.items.get('m:turn-1:m1')).toMatchObject({ text: 'Hello world', settled: true })
  })

  it('reads a message that lands while its calls run above them, as it was said', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', 'go'),
      l.start('turn-1', 'r1'),
      l.tool('turn-1', 'step-1', 'pre'),
      l.tool('turn-1', 'step-1', 'post'),
      l.tool('turn-1', 'step-2', 'pre'),
      l.tool('turn-1', 'step-3', 'pre'),
      l.text('turn-1', 'm1', 0, 'Step 1 done, so steps 2 and 3 now.', true),
      l.tool('turn-1', 'step-2', 'post'),
      l.tool('turn-1', 'step-3', 'post'),
      l.text('turn-1', 'm2', 0, 'Done.', true),
    ])
    const order = ['p:r1', 't:step-1', 'm:turn-1:m1', 't:step-2', 't:step-3', 'm:turn-1:m2']
    expect(feed.order).toEqual(order)
  })

  it('settles every text of a turn when the turn ends', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [l.text('turn-1', 'm1', 0, 'Hel'), l.end('turn-1')])
    expect(feed.items.get('m:turn-1:m1')).toMatchObject({ text: 'Hel', settled: true })
  })

  it('reads a tool still running when its turn was interrupted as interrupted', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.start('turn-1'),
      l.tool('turn-1', 'done', 'pre'),
      l.tool('turn-1', 'done', 'post'),
      l.tool('turn-1', 'open', 'pre'),
      l.end('turn-1', 'interrupted'),
    ])
    expect((feed.items.get('t:done') as ToolItem).status).toBe('ok')
    expect((feed.items.get('t:open') as ToolItem).status).toBe('interrupted')
    expect(feed.facts.running).toBeNull()
    expect(feed.order.at(-1)).toBe('k:5')
    expect(feed.items.get('k:5')).toMatchObject({ kind: 'marker', marker: 'turn_interrupted' })
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
    expect(after.items.get('p:r1')).toBe(before.items.get('p:r1'))
    expect(after.items.get('m:turn-1:m1')).not.toBe(before.items.get('m:turn-1:m1'))
    expect(after.order).toBe(before.order)
    expect(after.facts).toBe(before.facts)
    expect(after.scoped).toBe(before.scoped)
  })

  it('keeps a subagent scope out of the order, under its card', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.row('subagent', { phase: 'start', type: 'explore' }, { turnId: 'turn-1', scope: 'sub-1' }),
      { ...l.tool('turn-1', 'inner', 'pre', 'Grep'), scope: 'sub-1' },
    ])
    expect(feed.order).toEqual(['s:sub-1'])
    expect(feed.scoped['sub-1']).toEqual(['t:inner'])
    expect(feed.facts.liveSubagents).toEqual(['s:sub-1'])
  })

  it('opens an ask as a pending choice and resolves it', () => {
    const l = ledger()
    let feed = applyRow(emptyFeed(), l.ask('q1'))
    expect(feed.facts.openAsks).toEqual(['a:q1'])
    expect((feed.items.get('a:q1') as AskItem).choice).toMatchObject({
      id: 'q1',
      pending: true,
      answerable: true,
    })
    feed = applyRow(feed, l.answer('q1', 'y'))
    expect(feed.facts.openAsks).toEqual([])
    expect((feed.items.get('a:q1') as AskItem).choice).toMatchObject({
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

  it('draws /compact and the compaction it ran as one divider carrying the command', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r1', '/compact'),
      l.start('turn-1', 'r1'),
      l.end('turn-1'),
      l.row('marker', { kind: 'compaction_ended' }),
    ])
    expect(feed.order).toHaveLength(1)
    expect(feed.items.get(feed.order[0]!)).toMatchObject({
      kind: 'marker',
      marker: 'compaction_ended',
      command: '/compact',
    })
  })

  it("names the model each turn ran on: its CLI's launch model, then any change", () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.runner('r1', 'model-a'),
      l.start('turn-1', 'r1'),
      l.end('turn-1'),
      l.row('marker', { kind: 'model_changed', detail: 'model-b' }),
      l.start('turn-2'),
      l.end('turn-2'),
      l.runner('r2'),
      l.start('turn-3'),
    ])
    expect(feed.facts.models).toEqual({ 'turn-1': 'model-a', 'turn-2': 'model-b' })
  })

  // A launch records its prompt, then the CLI that carries it says what it switched to.
  it('reads a launch switch above the prompt that caused it', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r0', 'earlier'),
      l.start('turn-0', 'r0'),
      l.end('turn-0'),
      l.delivered('r1', 'on', 'restart'),
      l.row('marker', { kind: 'provider_switched', detail: 'p2' }),
      l.row('marker', { kind: 'model_changed', detail: 'big' }),
      l.start('turn-1', 'r1'),
      l.row('marker', { kind: 'compaction_ended' }),
    ])
    expect(feed.order).toEqual(['p:r0', 'k:5', 'p:r1', 'k:8'])
  })

  it('leaves a queued prompt above a selection change made while it waits', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.prompt('r0', 'earlier'),
      l.start('turn-0', 'r0'),
      l.delivered('r1', 'queued', 'api'),
      l.row('marker', { kind: 'model_changed', detail: 'big' }),
    ])
    expect(feed.order).toEqual(['p:r0', 'p:r1', 'k:4'])
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
    expect(feed.items.get('m:turn-1:final-turn-1')).toMatchObject({ text: 'Hello there' })
  })

  it('shows a joining message once, though its naming chunk came before its first', () => {
    const l = ledger()
    const joins = { messageId: 'm2', index: 1, final: true, text: ' there', reply: 'final-turn-1' }
    const feed = applyRows(emptyFeed(), [
      l.text('turn-1', 'final-turn-1', -1, 'Hello there'),
      l.end('turn-1'),
      l.row('assistant_text', joins, { turnId: 'turn-1' }),
      l.text('turn-1', 'm2', 0, 'Hello'),
    ])
    expect(feed.order.filter((id) => id.startsWith('m:'))).toEqual(['m:turn-1:final-turn-1'])
  })
})

// A replay folds thousands of rows before anyone reads the feed; copying the
// item map, the order or a per-turn record per row made it quadratic.
describe('FeedFold', () => {
  it('copies the item map, the order and the turn records once per replay', () => {
    const l = ledger()
    const fold = new FeedFold(emptyFeed())
    const turn = (i: number) => [
      l.prompt(`r${i}`, `prompt ${i}`),
      l.start(`turn-${i}`, `r${i}`),
      l.tool(`turn-${i}`, `tool-${i}`, 'pre'),
      l.text(`turn-${i}`, `m${i}`, 0, `reply ${i}`, true),
      l.row('turn_ended', { status: 'done', effort: 'high' }, { turnId: `turn-${i}` }),
    ]
    for (const row of turn(0)) fold.apply(row)
    const first = fold.finish()
    // Read now: the fold's facts object is itself reused, so a later read would
    // see whatever record it holds by then.
    const { prompts, efforts, output } = first.facts
    for (let i = 1; i < 50; i++) for (const row of turn(i)) fold.apply(row)
    const last = fold.finish()
    expect(last.order).toHaveLength(150)
    expect(last.items).toBe(first.items)
    expect(last.order).toBe(first.order)
    expect(last.facts.prompts).toBe(prompts)
    expect(last.facts.efforts).toBe(efforts)
    expect(last.facts.output).toBe(output)
  })

  it('settles only the turn that ended', () => {
    const l = ledger()
    const fold = new FeedFold(emptyFeed())
    for (const row of [
      l.start('turn-a'),
      l.text('turn-a', 'ma', 0, 'from a'),
      l.tool('turn-a', 'tool-a', 'pre'),
      l.start('turn-b'),
      l.text('turn-b', 'mb', 0, 'from b'),
      l.tool('turn-b', 'tool-b', 'pre'),
      l.end('turn-a'),
    ])
      fold.apply(row)
    const { items } = fold.finish()
    expect((items.get('m:turn-a:ma') as TextItem).settled).toBe(true)
    expect((items.get('t:tool-a') as ToolItem).status).toBe('abandoned')
    expect((items.get('m:turn-b:mb') as TextItem).settled).toBe(false)
    expect((items.get('t:tool-b') as ToolItem).status).toBe('running')
  })
})
