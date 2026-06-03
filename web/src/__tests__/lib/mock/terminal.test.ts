// web/src/__tests__/lib/mock/terminal.test.ts
import { describe, it, expect } from 'vitest'
import { getMockTerminalSeed } from '@/lib/mock/terminal'

describe('getMockTerminalSeed', () => {
  it('returns a prompt string', () => {
    const seed = getMockTerminalSeed()
    expect(typeof seed.prompt).toBe('string')
    expect(seed.prompt.length).toBeGreaterThan(0)
  })
  it('returns prior output lines', () => {
    const seed = getMockTerminalSeed()
    expect(Array.isArray(seed.priorOutput)).toBe(true)
    expect(seed.priorOutput.length).toBeGreaterThan(0)
  })
})
