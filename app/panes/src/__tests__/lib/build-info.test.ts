import { afterEach, describe, it, expect, vi } from 'vitest'
import { getBuildInfo } from '@/lib/build-info'

interface BuildEnv {
  DEV: boolean
  VITE_BUILD_CHANNEL?: string
  VITE_BUILD_VERSION?: string
  VITE_BUILD_TIMESTAMP?: string
}

afterEach(() => vi.unstubAllEnvs())

/** What the build stamped into `import.meta.env`, read through the public entry. */
function resolveBuildInfo(env: BuildEnv) {
  vi.stubEnv('DEV', env.DEV)
  vi.stubEnv('VITE_BUILD_CHANNEL', env.VITE_BUILD_CHANNEL)
  vi.stubEnv('VITE_BUILD_VERSION', env.VITE_BUILD_VERSION)
  vi.stubEnv('VITE_BUILD_TIMESTAMP', env.VITE_BUILD_TIMESTAMP)
  return getBuildInfo()
}

describe('getBuildInfo', () => {
  it('is dev whenever DEV is true, regardless of any CI channel stamp', () => {
    expect(resolveBuildInfo({ DEV: true, VITE_BUILD_CHANNEL: 'release' })).toMatchObject({
      channel: 'dev',
    })
  })

  it('is nightly and carries no version, even if one is stamped', () => {
    const info = resolveBuildInfo({
      DEV: false,
      VITE_BUILD_CHANNEL: 'nightly',
      VITE_BUILD_VERSION: 'v1.2.3',
      VITE_BUILD_TIMESTAMP: '2026-09-12T03:14:00Z',
    })
    expect(info).toEqual({ channel: 'nightly', timestamp: '2026-09-12T03:14:00Z' })
  })

  it('is beta and carries both version and timestamp', () => {
    const info = resolveBuildInfo({
      DEV: false,
      VITE_BUILD_CHANNEL: 'beta',
      VITE_BUILD_VERSION: 'v0.9.2-beta.1',
      VITE_BUILD_TIMESTAMP: '2026-09-12T03:14:00Z',
    })
    expect(info).toEqual({
      channel: 'beta',
      version: 'v0.9.2-beta.1',
      timestamp: '2026-09-12T03:14:00Z',
    })
  })

  it('falls back to a quiet release when no CI channel is stamped', () => {
    const info = resolveBuildInfo({ DEV: false, VITE_BUILD_VERSION: 'v1.0.0' })
    expect(info).toEqual({ channel: 'release', version: 'v1.0.0' })
  })

  it('is release for an explicit release stamp', () => {
    const info = resolveBuildInfo({
      DEV: false,
      VITE_BUILD_CHANNEL: 'release',
      VITE_BUILD_VERSION: 'v1.0.0',
    })
    expect(info).toEqual({ channel: 'release', version: 'v1.0.0' })
  })
})
