import { describe, it, expect, vi, afterEach } from 'vitest'
import { bestEffort } from '@/lib/best-effort'

/** Settle the microtask queue so a floating rejection would have been reported. */
const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

afterEach(() => {
  vi.restoreAllMocks()
})

describe('bestEffort', () => {
  it('leaves no unhandled rejection behind when the work fails', async () => {
    const unhandled = vi.fn()
    process.on('unhandledRejection', unhandled)
    try {
      bestEffort(Promise.reject(new Error('module missing')), 'thing')
      await settle()
      expect(unhandled).not.toHaveBeenCalled()
    } finally {
      process.off('unhandledRejection', unhandled)
    }
  })

  it('names the failed work in the dev warning so it stays diagnosable', async () => {
    vi.stubEnv('DEV', true)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const boom = new Error('module missing')

    bestEffort(Promise.reject(boom), 'clear blame for closed buffer')
    await settle()

    expect(warn).toHaveBeenCalledWith('best-effort clear blame for closed buffer failed:', boom)
    vi.unstubAllEnvs()
  })

  it('stays quiet outside dev so production consoles are not noisy', async () => {
    vi.stubEnv('DEV', false)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})

    bestEffort(Promise.reject(new Error('module missing')), 'thing')
    await settle()

    expect(warn).not.toHaveBeenCalled()
    vi.unstubAllEnvs()
  })

  it('does nothing at all when the work succeeds', async () => {
    vi.stubEnv('DEV', true)
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})

    bestEffort(Promise.resolve('fine'), 'thing')
    await settle()

    expect(warn).not.toHaveBeenCalled()
    vi.unstubAllEnvs()
  })
})
