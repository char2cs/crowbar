import { afterEach, describe, expect, it, vi } from 'vitest'

afterEach(() => {
  vi.unstubAllEnvs()
  vi.resetModules()
})

const defaultsUnder = async (dev: boolean) => {
  vi.stubEnv('DEV', dev)
  vi.resetModules()
  return (await import('@/features/settings/config/default-settings')).defaultSettings
}

describe('default settings', () => {
  // The readout is a developer's diagnostic: a shipped build starts without it.
  it('shows the build readout only in a development build', async () => {
    expect((await defaultsUnder(false)).buildReadout).toBe(false)
    expect((await defaultsUnder(true)).buildReadout).toBe(true)
  })
})
