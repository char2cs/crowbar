import { describe, expect, it } from 'vitest'
import { pointInRect } from '@/features/file-system/lib/tauri-file-drop'

function rect(left: number, top: number, right: number, bottom: number): DOMRect {
  return { left, top, right, bottom, width: right - left, height: bottom - top, x: left, y: top, toJSON: () => ({}) }
}

describe('pointInRect', () => {
  it('is true for a point strictly inside', () => {
    expect(pointInRect({ x: 50, y: 50 }, rect(0, 0, 100, 100))).toBe(true)
  })

  it('is true on the boundary (inclusive)', () => {
    expect(pointInRect({ x: 100, y: 100 }, rect(0, 0, 100, 100))).toBe(true)
    expect(pointInRect({ x: 0, y: 0 }, rect(0, 0, 100, 100))).toBe(true)
  })

  it('is false outside the rect', () => {
    expect(pointInRect({ x: 101, y: 50 }, rect(0, 0, 100, 100))).toBe(false)
    expect(pointInRect({ x: 50, y: -1 }, rect(0, 0, 100, 100))).toBe(false)
  })
})
