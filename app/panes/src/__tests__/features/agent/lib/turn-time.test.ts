import { describe, expect, it, vi } from 'vitest'
import { turnTimestampLabel, turnTimeTitle } from '@/features/agent/lib/turn-time'

function expectedAbsolute(at: string): string {
  const then = new Date(Date.parse(at))
  const sameYear = then.getFullYear() === new Date().getFullYear()
  const datePart = then.toLocaleDateString(undefined, {
    month: 'short',
    day: 'numeric',
    year: sameYear ? undefined : 'numeric',
  })
  const timePart = then.toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' })
  return `${datePart}, ${timePart}`
}

describe('turnTimestampLabel', () => {
  it('is just the absolute date and time', () => {
    const at = '2026-08-30T14:59:00Z'
    expect(turnTimestampLabel(at)).toBe(expectedAbsolute(at))
  })

  it('returns empty for a timestamp it cannot parse', () => {
    expect(turnTimestampLabel('not a date')).toBe('')
  })
})

describe('turnTimeTitle', () => {
  it('always carries the full date, regardless of which shorthand is showing', () => {
    const title = turnTimeTitle('2026-08-30T13:00:00Z')
    expect(title).toMatch(/2026/)
  })

  it('returns empty for a timestamp it cannot parse', () => {
    expect(turnTimeTitle('not a date')).toBe('')
  })
})

describe('turn time formatting cost', () => {
  it('formats rows without resolving the locale on each call', () => {
    const spies = (['toLocaleString', 'toLocaleDateString', 'toLocaleTimeString'] as const).map(
      (method) => vi.spyOn(Date.prototype, method),
    )
    try {
      for (let i = 0; i < 20; i++) {
        const at = `2026-08-30T13:${String(i).padStart(2, '0')}:00Z`
        turnTimeTitle(at)
        turnTimestampLabel(at)
      }
      for (const spy of spies) expect(spy).not.toHaveBeenCalled()
    } finally {
      for (const spy of spies) spy.mockRestore()
    }
  })
})
