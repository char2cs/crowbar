import { describe, it, expect } from 'vitest'
import { BASE_RECONNECT_DELAY_MS, nextReconnectDelay } from '@/lib/ws/reconnect-backoff'

describe('nextReconnectDelay', () => {
  it.each([
    [BASE_RECONNECT_DELAY_MS, 2000],
    [8000, 16_000],
    [20_000, 30_000],
    [30_000, 30_000],
  ])('after %i waits %i', (current, expected) => {
    expect(nextReconnectDelay(current)).toBe(expected)
  })
})
