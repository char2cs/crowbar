import { describe, it, expect } from 'vitest'

describe('test environment', () => {
  it('has window', () => {
    console.log('typeof window:', typeof window)
    expect(typeof window).not.toBe('undefined')
  })
})
