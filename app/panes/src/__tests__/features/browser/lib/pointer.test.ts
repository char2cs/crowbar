import { describe, expect, it } from 'vitest'
import { formatPointer, type PickResult } from '@/features/browser/lib/pointer'

const item = (over: Partial<PickResult['items'][number]> = {}) => ({
  s: 'section.summary > h2',
  tag: 'h2',
  role: 'heading',
  name: 'Order summary',
  text: 'Order summary',
  r: [640, 300, 420, 30],
  h: 'Checkout',
  i: 0,
  ...over,
})

const base: PickResult = {
  kind: 'region',
  title: 'Checkout',
  url: 'https://stripe.com/checkout',
  items: [item(), item({ tag: 'tr', role: '', name: '', text: 'Total $31' })],
  more: 2,
  box: [640, 300, 420, 260],
}

function dataOf(text: string) {
  const last = text.split('\n').at(-1)!
  expect(last.startsWith('⌖data ')).toBe(true)
  return JSON.parse(last.slice('⌖data '.length))
}

describe('formatPointer', () => {
  it('writes a header, item lines and a final data line for a region', () => {
    const lines = formatPointer(base, 'b-7k2').split('\n')
    expect(lines[0]).toBe(
      '⌖ region "Order summary" · https://stripe.com/checkout · tab b-7k2 · 4 items',
    )
    expect(lines[1]).toBe(' 1 heading "Order summary"')
    expect(lines[2]).toBe(' 2 tr "Total $31"')
    expect(dataOf(lines.join('\n'))).toEqual({
      v: 1,
      tab: 'b-7k2',
      url: 'https://stripe.com/checkout',
      title: 'Checkout',
      kind: 'region',
      items: base.items,
      more: 2,
      box: [640, 300, 420, 260],
    })
  })

  it('gives a single element only a header and the data line, without an item count', () => {
    const text = formatPointer({ ...base, kind: 'element', items: [item()], more: 0 }, 'b-1')
    expect(text.split('\n')).toHaveLength(2)
    expect(text).toMatch(/^⌖ heading "Order summary" · https:\/\/stripe.com\/checkout · tab b-1\n/)
  })

  it('carries the quote of a text pick', () => {
    const text = formatPointer({ ...base, kind: 'text', quote: 'pay "now"', more: 0 }, 'b-1')
    expect(text.split('\n')[0]).toContain('⌖ text "pay \\"now\\""')
    expect(dataOf(text).quote).toBe('pay "now"')
  })
})
