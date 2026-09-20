import { afterEach, describe, expect, it, vi } from 'vitest'

// currentPlatform is computed ONCE, at module load, from navigator.userAgent —
// so each case needs a fresh module evaluation with the UA already stubbed,
// not a mutation after import.
async function loadWithUserAgent(userAgent: string) {
  vi.stubGlobal('navigator', { userAgent })
  vi.resetModules()
  return import('@/utils/platform')
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.resetModules()
})

describe('detectPlatform (webview branch)', () => {
  it('detects Windows from the user agent', async () => {
    const { currentPlatform, IS_WINDOWS, IS_MAC, IS_LINUX } = await loadWithUserAgent(
      'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko)',
    )
    expect(currentPlatform).toBe('windows')
    expect(IS_WINDOWS).toBe(true)
    expect(IS_MAC).toBe(false)
    expect(IS_LINUX).toBe(false)
  })

  it('detects Linux from the user agent', async () => {
    const { currentPlatform, IS_LINUX, IS_MAC, IS_WINDOWS } = await loadWithUserAgent(
      'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15 (KHTML, like Gecko)',
    )
    expect(currentPlatform).toBe('linux')
    expect(IS_LINUX).toBe(true)
    expect(IS_MAC).toBe(false)
    expect(IS_WINDOWS).toBe(false)
  })

  it('falls back to macOS for a Macintosh user agent', async () => {
    const { currentPlatform, IS_MAC } = await loadWithUserAgent(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)',
    )
    expect(currentPlatform).toBe('macos')
    expect(IS_MAC).toBe(true)
  })

  it('falls back to macOS for an unrecognized user agent', async () => {
    const { currentPlatform } = await loadWithUserAgent('SomeUnknownAgent/1.0')
    expect(currentPlatform).toBe('macos')
  })
})

describe('normalizeKey', () => {
  it('leaves cmd untouched on macOS', async () => {
    const { normalizeKey } = await loadWithUserAgent(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)',
    )
    expect(normalizeKey('cmd+s')).toBe('cmd+s')
  })

  it('converts cmd to ctrl on Linux', async () => {
    const { normalizeKey } = await loadWithUserAgent(
      'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15 (KHTML, like Gecko)',
    )
    expect(normalizeKey('cmd+s')).toBe('ctrl+s')
  })
})
