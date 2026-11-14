import { afterEach, describe, expect, it, vi } from 'vitest'
import { readLocal, removeLocal, writeLocal } from '@/lib/local-storage'

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
})

describe('local storage', () => {
  it('reads absent and writes false when storage throws', () => {
    const blocked = () => {
      throw new DOMException('blocked', 'SecurityError')
    }
    vi.stubGlobal('localStorage', { getItem: blocked, setItem: blocked, removeItem: blocked })

    expect(readLocal('k')).toBeNull()
    expect(writeLocal('k', 'v')).toBe(false)
    expect(() => removeLocal('k')).not.toThrow()
  })

  it('round-trips a value when storage works', () => {
    expect(writeLocal('k', 'v')).toBe(true)
    expect(readLocal('k')).toBe('v')
    removeLocal('k')
    expect(readLocal('k')).toBeNull()
  })
})
