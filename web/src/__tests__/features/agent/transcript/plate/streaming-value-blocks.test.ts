import { describe, expect, it } from 'vitest'
import {
  stableBlockCount,
  trailingTextDivergence,
} from '@/features/agent/transcript/plate/streaming-value-blocks'

const p = (text: string, extra: Record<string, unknown> = {}) => ({
  type: 'p',
  id: 'irrelevant',
  children: [{ text, ...extra }],
})

describe('stableBlockCount', () => {
  it('is the full length when every block matches', () => {
    const a = [p('one'), p('two')]
    const b = [p('one'), p('two')]
    expect(stableBlockCount(a, b)).toBe(2)
  })

  it('stops at the first block that differs', () => {
    const a = [p('one'), p('two')]
    const b = [p('one'), p('two, growing')]
    expect(stableBlockCount(a, b)).toBe(1)
  })

  it('ignores id — a fresh reparse mints a new one for every block', () => {
    const a = [{ type: 'p', id: 'a1', children: [{ text: 'same' }] }]
    const b = [{ type: 'p', id: 'a2', children: [{ text: 'same' }] }]
    expect(stableBlockCount(a, b)).toBe(1)
  })

  it('is 0 when nothing in common, and never exceeds the shorter array', () => {
    expect(stableBlockCount([p('a')], [p('b'), p('c')])).toBe(0)
    expect(stableBlockCount([p('a'), p('b')], [p('a')])).toBe(1)
  })
})

describe('trailingTextDivergence', () => {
  it('is a pure append (keep = the whole of prev) for a plain growing paragraph', () => {
    const prevText = p('Building a').children![0]!
    expect(trailingTextDivergence(prevText, p('Building a CLI').children![0]!)).toEqual({
      keep: (prevText.text as string).length,
      replacement: ' CLI',
    })
  })

  it('is a pure append walking down through unchanged wrapper elements', () => {
    const prev = { type: 'p', children: [{ text: 'partial' }] }
    const next = { type: 'p', children: [{ text: 'partial text' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 7, replacement: ' text' })
  })

  // closeAssistantTurn can reconcile a streamed tail against the hook's own text, a change no
  // append describes; keeping the shared prefix avoids re-fading text that did not change.
  it('keeps the shared prefix and reports only the diverging tail when reconciliation changes wording partway through', () => {
    const prev = { type: 'p', children: [{ text: 'The cat sat on the mat' }] }
    const next = { type: 'p', children: [{ text: 'The cat sat on the rug' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({
      keep: 'The cat sat on the '.length,
      replacement: 'rug',
    })
  })

  it('reports keep: 0 when nothing at all is shared, rather than refusing structurally', () => {
    const prev = { type: 'p', children: [{ text: 'hello' }] }
    const next = { type: 'p', children: [{ text: 'goodbye' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 0, replacement: 'goodbye' })
  })

  it('reports an empty replacement when reconciliation only shortens the tail', () => {
    const prev = { type: 'p', children: [{ text: 'hello world' }] }
    const next = { type: 'p', children: [{ text: 'hello' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 5, replacement: '' })
  })

  it('returns null when a mark changed rather than the text growing or reconciling', () => {
    const prev = { type: 'p', children: [{ text: 'run make', code: true }] }
    const next = { type: 'p', children: [{ text: 'run make' }] }
    expect(trailingTextDivergence(prev, next)).toBeNull()
  })

  it('returns null when a non-trailing leaf changed', () => {
    const prev = { type: 'p', children: [{ text: 'a' }, { text: 'b' }] }
    const next = { type: 'p', children: [{ text: 'A' }, { text: 'b' }] }
    expect(trailingTextDivergence(prev, next)).toBeNull()
  })

  it('recurses into the last child when only the final inline run changed', () => {
    const prev = {
      type: 'p',
      children: [{ text: 'see ' }, { text: 'bold', bold: true }],
    }
    const next = {
      type: 'p',
      children: [{ text: 'see ' }, { text: 'bolder', bold: true }],
    }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 4, replacement: 'er' })
  })

  it('returns null when the child count changed (a new inline run started)', () => {
    const prev = { type: 'p', children: [{ text: 'see ' }] }
    const next = { type: 'p', children: [{ text: 'see ' }, { text: 'bold', bold: true }] }
    expect(trailingTextDivergence(prev, next)).toBeNull()
  })

  it('returns null when the block type changed', () => {
    const prev = { type: 'p', children: [{ text: 'x' }] }
    const next = { type: 'h1', children: [{ text: 'x' }] }
    expect(trailingTextDivergence(prev, next)).toBeNull()
  })

  it('keeps everything and replaces nothing for two identical blocks', () => {
    expect(trailingTextDivergence(p('same'), p('same'))).toEqual({ keep: 4, replacement: '' })
  })

  it('ignores id when comparing element props', () => {
    const prev = { type: 'p', id: 'a1', children: [{ text: 'grow' }] }
    const next = { type: 'p', id: 'a2', children: [{ text: 'growing' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 4, replacement: 'ing' })
  })
})
