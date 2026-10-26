import { describe, expect, it } from 'vitest'
import { rowTouch } from '@/features/window/lib/row-touch'

describe('rowTouch', () => {
  it('arms on the hold and stays armed while the finger rests within 8px', () => {
    const armed = rowTouch('idle', { type: 'hold' })
    expect(armed).toBe('armed')
    expect(rowTouch(armed, { type: 'move', dx: 5, dy: 6 })).toBe('armed')
  })

  it('hands the touch to the drag once the finger moves past 8px, and keeps it there', () => {
    const drag = rowTouch('armed', { type: 'move', dx: 0, dy: 9 })
    expect(drag).toBe('drag')
    expect(rowTouch(drag, { type: 'move', dx: 0, dy: 0 })).toBe('drag')
    expect(rowTouch(drag, { type: 'hold' })).toBe('drag')
  })

  it('ignores movement before the hold', () => {
    expect(rowTouch('idle', { type: 'move', dx: 40, dy: 40 })).toBe('idle')
  })

  it('returns to idle on release from any state (menu shown, lift ended, or cancelled)', () => {
    for (const s of ['idle', 'armed', 'drag'] as const) {
      expect(rowTouch(s, { type: 'release' })).toBe('idle')
    }
  })
})
