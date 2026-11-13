import { describe, expect, it } from 'vitest'

import { formatElapsed } from '@/features/agent/activity/lib/shelf-fit'

describe('formatElapsed', () => {
  it('reads like a stopwatch, because subagents run past a minute', () => {
    expect(formatElapsed(0)).toBe('0:00')
    expect(formatElapsed(9)).toBe('0:09')
    expect(formatElapsed(75)).toBe('1:15')
    expect(formatElapsed(3599)).toBe('59:59')
  })

  it('never renders a negative clock from a skewed timestamp', () => {
    expect(formatElapsed(-5)).toBe('0:00')
  })

  it('rolls into h:mm:ss at the hour mark, because minutes alone stop reading as a clock', () => {
    expect(formatElapsed(3600)).toBe('1:00:00')
    expect(formatElapsed(3661)).toBe('1:01:01')
    expect(formatElapsed(12000)).toBe('3:20:00')
  })

  it('rolls into d hh:mm:ss once a subagent runs past a day', () => {
    expect(formatElapsed(86400)).toBe('1d 00:00:00')
    expect(formatElapsed(90061)).toBe('1d 01:01:01')
    expect(formatElapsed(200000)).toBe('2d 07:33:20')
  })
})
