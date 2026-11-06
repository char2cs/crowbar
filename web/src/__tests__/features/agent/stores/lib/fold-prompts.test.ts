import { describe, expect, it } from 'vitest'
import { emptyFeed } from '@/features/agent/stores/lib/fold-rows'
import { applyRows, ledger } from '@/__tests__/__fixtures__/ledger-rows'

// The pills a transcript draws between turns, folded from their marker rows.
describe('divider', () => {
  it('draws one switch divider for switches back and forth with nothing said between', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.row('marker', { kind: 'provider_switched', detail: 'codex' }),
      l.row('marker', { kind: 'model_changed', detail: 'big' }),
      l.row('marker', { kind: 'provider_switched', detail: 'claude' }),
      l.row('marker', { kind: 'provider_switched', detail: 'codex' }),
    ])
    expect(feed.order).toEqual(['k:2', 'k:4'])
    expect(feed.items['k:4']).toMatchObject({ marker: 'provider_switched', detail: 'codex' })
  })

  it('shows what the CLI warned its user about as a notice in the transcript', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.row('marker', { kind: 'warning', detail: 'Code Mode is unavailable' }),
    ])
    expect(feed.order).toEqual(['k:1'])
    expect(feed.items['k:1']).toMatchObject({
      kind: 'marker',
      marker: 'warning',
      detail: 'Code Mode is unavailable',
    })
  })

  it('draws a warning the chat already shows once, though each new connection repeats it', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.row('marker', { kind: 'warning', detail: 'no bubblewrap' }),
      l.start('turn-1'),
      l.end('turn-1'),
      l.row('marker', { kind: 'warning', detail: 'no bubblewrap' }),
      l.row('marker', { kind: 'warning', detail: 'resumed on another model' }),
    ])
    const warnings = feed.order.filter((id) => feed.items[id]?.kind === 'marker')
    expect(warnings.map((id) => (feed.items[id] as { detail?: string }).detail)).toEqual([
      'no bubblewrap',
      'resumed on another model',
    ])
  })
})
