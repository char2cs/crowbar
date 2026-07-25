/**
 * Pure drag geometry for the (virtualized) agent-chats sidebar list.
 *
 * Once the list is windowed, off-screen rows are not painted, so the drag can no
 * longer find its drop target by DOM hit-testing. These functions resolve the
 * target — and the auto-scroll needed to reach far rows — from scroll geometry
 * alone, which makes them unit-testable without a layout engine.
 */
import { describe, expect, it } from 'vitest'
import {
  AGENT_CHAT_ROW_HEIGHT,
  autoScrollDelta,
  resolveDropSlotIndex,
} from '@/features/agent/components/agent-chat-drop-geometry'

describe('AGENT_CHAT_ROW_HEIGHT', () => {
  it('is the ROW_BASE pitch: h-9 (36) + my-0.5 (4)', () => {
    expect(AGENT_CHAT_ROW_HEIGHT).toBe(40)
  })
})

// A SLOT, not a row. The drop lands BETWEEN two rows — slot i is the gap above
// row i — which is what the insertion line draws and what `reorderIds` inserts
// at. Resolving a ROW instead was the defect: "insert before the row under the
// pointer" has no way to express "after the last row", so the bottom of the list
// was unreachable, and dropping onto the row directly beneath the dragged one
// produced the list it already was.
describe('resolveDropSlotIndex', () => {
  const base = { containerTop: 100, scrollTop: 0, rowHeight: 40, count: 5 }

  it('resolves the top half of a row to the slot ABOVE it', () => {
    expect(resolveDropSlotIndex({ ...base, pointerY: 100 })).toBe(0)
    expect(resolveDropSlotIndex({ ...base, pointerY: 119 })).toBe(0)
  })

  it('crosses to the next slot at the row MIDPOINT, not the row edge', () => {
    expect(resolveDropSlotIndex({ ...base, pointerY: 120 })).toBe(1)
    expect(resolveDropSlotIndex({ ...base, pointerY: 159 })).toBe(1)
  })

  it('reaches the LAST slot from the bottom half of the last row', () => {
    // The slot the old row-based resolver could not express at all: past the
    // last row's midpoint is "after everything".
    expect(resolveDropSlotIndex({ ...base, pointerY: 281 })).toBe(5)
  })

  it('clamps below the list to the last slot', () => {
    expect(resolveDropSlotIndex({ ...base, pointerY: 900 })).toBe(5)
  })

  it('clamps above the first row to the first slot', () => {
    expect(resolveDropSlotIndex({ ...base, pointerY: -400 })).toBe(0)
  })

  it('accounts for the container scroll offset', () => {
    // Scrolled down 80px → the pointer at the container top sits on row 2's top
    // edge, i.e. the slot above it.
    expect(resolveDropSlotIndex({ ...base, scrollTop: 80, pointerY: 100 })).toBe(2)
  })

  it('is slot 0 for an empty list', () => {
    expect(resolveDropSlotIndex({ ...base, count: 0, pointerY: 900 })).toBe(0)
  })
})

describe('autoScrollDelta', () => {
  const base = { containerTop: 100, containerHeight: 400, edge: 40, step: 12 }

  it('scrolls up inside the top edge zone', () => {
    expect(autoScrollDelta({ ...base, pointerY: 120 })).toBe(-12)
  })

  it('keeps scrolling up when the pointer is above the container', () => {
    expect(autoScrollDelta({ ...base, pointerY: 90 })).toBe(-12)
  })

  it('scrolls down inside the bottom edge zone', () => {
    expect(autoScrollDelta({ ...base, pointerY: 480 })).toBe(12)
  })

  it('does not scroll in the middle', () => {
    expect(autoScrollDelta({ ...base, pointerY: 300 })).toBe(0)
  })

  it('does not scroll exactly at the edge-zone boundary', () => {
    expect(autoScrollDelta({ ...base, pointerY: 140 })).toBe(0) // distTop === edge
    expect(autoScrollDelta({ ...base, pointerY: 460 })).toBe(0) // distBottom === edge
  })
})
