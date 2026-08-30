import { describe, expect, it } from 'vitest'
import {
  turnLatencyLabel,
  turnTimeLabel,
  turnTimestampLabel,
  turnTimeTitle,
} from '@/features/agent/lib/turn-time'

const NOW = Date.parse('2026-08-30T15:00:00Z')

/** Mirrors `turnTimeLabel`'s own absolute formatting, so assertions hold in
 *  whatever timezone the test runner is actually in — never a hardcoded hour
 *  that would only be true in one specific one. */
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

describe('turnTimeLabel', () => {
  // Both halves visible together — nobody should have to hover to see when a
  // turn actually happened.
  it('pairs whole minutes with the absolute date and time for anything under an hour', () => {
    const at = '2026-08-30T14:59:00Z'
    expect(turnTimeLabel(at, NOW)).toBe(`1m · ${expectedAbsolute(at)}`)
  })

  it('rounds down rather than up, and floors at 0m for something just sent', () => {
    const justUnder = '2026-08-30T14:59:59Z'
    const exact = '2026-08-30T15:00:00Z'
    expect(turnTimeLabel(justUnder, NOW)).toBe(`0m · ${expectedAbsolute(justUnder)}`)
    expect(turnTimeLabel(exact, NOW)).toBe(`0m · ${expectedAbsolute(exact)}`)
  })

  // Past an hour it ROUNDS to hours rather than either keep growing as
  // minutes ("85m") or drop the elapsed half entirely.
  it('rounds to whole hours once an hour has actually passed, floored not nearest', () => {
    const oneHourTen = '2026-08-30T13:50:00Z' // 70 minutes ago — 1h, not 2h
    expect(turnTimeLabel(oneHourTen, NOW)).toBe(`1h · ${expectedAbsolute(oneHourTen)}`)
  })

  it('rounds to whole days once a day has actually passed', () => {
    const oneDayFive = '2026-08-29T10:00:00Z' // 29 hours ago — 1d, not 2d
    expect(turnTimeLabel(oneDayFive, NOW)).toBe(`1d · ${expectedAbsolute(oneDayFive)}`)
  })

  it('never reports a negative elapsed count for a clock skewed slightly ahead', () => {
    const at = '2026-08-30T15:00:05Z'
    expect(turnTimeLabel(at, NOW)).toBe(`0m · ${expectedAbsolute(at)}`)
  })

  it('returns empty for a timestamp it cannot parse', () => {
    expect(turnTimeLabel('not a date', NOW)).toBe('')
  })
})

describe('turnTimestampLabel', () => {
  // A user turn has nothing to time itself against — never an elapsed count,
  // only the absolute stamp `turnTimeLabel` pairs with one.
  it('is always just the absolute date and time, never an elapsed count', () => {
    const at = '2026-08-30T14:59:00Z'
    expect(turnTimestampLabel(at)).toBe(expectedAbsolute(at))
  })

  it('returns empty for a timestamp it cannot parse', () => {
    expect(turnTimestampLabel('not a date')).toBe('')
  })
})

describe('turnLatencyLabel', () => {
  // The useful number for a reply is how long the agent took to ANSWER, not
  // how long ago it answered — timed against the user's own turn, not now.
  it('times the reply against the user turn it answers, ignoring the current clock entirely', () => {
    const userAt = '2026-08-30T14:00:00Z'
    const assistantAt = '2026-08-30T14:03:00Z'
    expect(turnLatencyLabel(userAt, assistantAt)).toBe(`3m · ${expectedAbsolute(assistantAt)}`)
  })

  it('floors at 0m for a reply that lands within the same minute as the prompt', () => {
    const userAt = '2026-08-30T14:00:00Z'
    const assistantAt = '2026-08-30T14:00:45Z'
    expect(turnLatencyLabel(userAt, assistantAt)).toBe(`0m · ${expectedAbsolute(assistantAt)}`)
  })

  // REGRESSION: this used to keep growing as a raw minute count with no
  // ceiling ("85m") — now rounds to hours, then days, same as `turnTimeLabel`.
  it('rounds a long reply gap to hours, not a raw minute count', () => {
    const userAt = '2026-08-30T13:00:00Z'
    const assistantAt = '2026-08-30T14:15:00Z' // 75 minutes — 1h, not 75m
    expect(turnLatencyLabel(userAt, assistantAt)).toBe(`1h · ${expectedAbsolute(assistantAt)}`)
  })

  it('rounds a multi-day reply gap to days', () => {
    const userAt = '2026-08-27T14:00:00Z'
    const assistantAt = '2026-08-30T15:00:00Z' // 3 days later
    expect(turnLatencyLabel(userAt, assistantAt)).toBe(`3d · ${expectedAbsolute(assistantAt)}`)
  })

  it('returns empty when either side cannot be parsed', () => {
    expect(turnLatencyLabel('not a date', '2026-08-30T14:00:00Z')).toBe('')
    expect(turnLatencyLabel('2026-08-30T14:00:00Z', 'not a date')).toBe('')
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
