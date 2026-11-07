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
    expect(feed.order).toEqual(['k:1'])
    expect(feed.items['k:1']).toMatchObject({
      parts: [
        { marker: 'provider_switched', detail: 'codex' },
        { marker: 'model_changed', detail: 'big' },
      ],
    })
  })

  it('draws a switch of provider, model and effort as one divider above the prompt it carries', () => {
    const l = ledger()
    const feed = applyRows(emptyFeed(), [
      l.row('user_message', {
        requestId: 'r1',
        text: 'go',
        origin: 'crowbar',
        delivery: 'restart',
      }),
      l.row('marker', { kind: 'effort_changed', detail: 'low' }),
      l.row('marker', { kind: 'provider_switched', detail: 'claude' }),
      l.row('marker', { kind: 'model_changed', detail: 'sonnet' }),
      l.start('turn-1', 'r1'),
      l.end('turn-1'),
      l.row('marker', { kind: 'model_changed', detail: 'opus' }),
    ])
    const kinds = feed.order.map((id) => feed.items[id]?.kind)
    expect(kinds).toEqual(['marker', 'user', 'marker'])
    expect(feed.items[feed.order[0]!]).toMatchObject({
      parts: [
        { marker: 'provider_switched', detail: 'claude' },
        { marker: 'model_changed', detail: 'sonnet' },
        { marker: 'effort_changed', detail: 'low' },
      ],
    })
    expect(feed.items[feed.order[2]!]).toMatchObject({ marker: 'model_changed', detail: 'opus' })
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

  it('draws a repeated warning once though each repeat names a fresh request id', () => {
    const l = ledger()
    const fallback = (ray: string, req: string) =>
      l.row('marker', {
        kind: 'warning',
        detail: `Falling back to HTTPS. 401 Unauthorized, cf-ray: ${ray}-SCL, request id: ${req}`,
      })
    const feed = applyRows(emptyFeed(), [
      fallback('a5678e78ccede79c', 'req_29b2bd5a9f54455d'),
      fallback('a5678329cf05b4df', 'req_00ff11ee22dd33cc'),
    ])
    expect(feed.order.filter((id) => feed.items[id]?.kind === 'marker')).toHaveLength(1)
  })
})
