import { describe, expect, it } from 'vitest'
import { nearsLoadedTop, openRows } from '@/features/agent/lib/feed-window'

const GUESSED_ROW_PX = 40
const MIN_WINDOW_ROWS = 12
const MAX_WINDOW_ROWS = 1000

describe('feed window policy', () => {
  it('opens on about three screens of rows, whatever the screen', () => {
    const phone = openRows(844)
    const desktop = openRows(1000)
    expect(phone).toBe(Math.ceil((3 * 844) / GUESSED_ROW_PX))
    expect(desktop).toBeGreaterThan(phone)
  })

  it('never asks for fewer rows than a turn needs nor more than the cap', () => {
    expect(openRows(0)).toBe(MIN_WINDOW_ROWS)
    expect(openRows(1e9)).toBe(MAX_WINDOW_ROWS)
  })

  it('wants older rows once the view is within two screens of the first loaded row', () => {
    expect(nearsLoadedTop(800, 400)).toBe(true)
    expect(nearsLoadedTop(801, 400)).toBe(false)
  })
})
