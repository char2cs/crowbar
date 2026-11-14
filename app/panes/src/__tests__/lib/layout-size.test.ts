import { describe, expect, it, vi } from 'vitest'
import { layoutHeight } from '@/lib/layout-size'

function box(rect: { width: number; height: number }, offsetWidth: number): HTMLElement {
  const el = document.createElement('div')
  vi.spyOn(el, 'getBoundingClientRect').mockReturnValue(rect as DOMRect)
  Object.defineProperty(el, 'offsetWidth', { value: offsetWidth })
  return el
}

describe('layoutHeight', () => {
  it('returns the CSS-pixel height of an element under CSS zoom', () => {
    expect(layoutHeight(box({ width: 240, height: 60 }, 200))).toBeCloseTo(50)
  })

  it('returns the rect height unchanged when unzoomed or unlaid-out', () => {
    expect(layoutHeight(box({ width: 200, height: 60 }, 200))).toBe(60)
    expect(layoutHeight(box({ width: 0, height: 0 }, 0))).toBe(0)
  })
})
