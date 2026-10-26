import { afterEach, describe, expect, it, vi } from 'vitest'

// currentPlatform is computed ONCE, at module load, from navigator.userAgent —
// so each case needs a fresh module evaluation with the UA already stubbed,
// not a mutation after import.
async function loadWithUserAgent(userAgent: string, maxTouchPoints = 0) {
  vi.stubGlobal('navigator', { userAgent, maxTouchPoints })
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

describe('detectPlatform (iOS)', () => {
  it('detects an iPhone, which is not macOS', async () => {
    const { currentPlatform, IS_MOBILE, IS_MAC } = await loadWithUserAgent(
      'Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148',
    )
    expect(currentPlatform).toBe('ios')
    expect(IS_MOBILE).toBe(true)
    expect(IS_MAC).toBe(false)
  })

  it('detects iPadOS, which sends a Macintosh user agent but has touch', async () => {
    const { IS_MOBILE, IS_MAC } = await loadWithUserAgent(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)',
      5,
    )
    expect(IS_MOBILE).toBe(true)
    expect(IS_MAC).toBe(false)
  })

  it('a Mac is not mobile', async () => {
    const { IS_MOBILE } = await loadWithUserAgent(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)',
    )
    expect(IS_MOBILE).toBe(false)
  })
})

describe('normalizeKey', () => {
  it('leaves cmd untouched on macOS', async () => {
    const { normalizeKey } = await loadWithUserAgent(
      'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)',
    )
    expect(normalizeKey('cmd+s')).toBe('cmd+s')
  })

  // An iPhone's hardware keyboard has Command, as a Mac's does.
  it('leaves cmd untouched on iOS, which keys like a Mac', async () => {
    const { normalizeKey, APPLE_KEYBOARD } = await loadWithUserAgent(
      'Mozilla/5.0 (iPhone; CPU iPhone OS 26_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148',
    )
    expect(APPLE_KEYBOARD).toBe(true)
    expect(normalizeKey('cmd+s')).toBe('cmd+s')
  })

  it('converts cmd to ctrl on Linux', async () => {
    const { normalizeKey } = await loadWithUserAgent(
      'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15 (KHTML, like Gecko)',
    )
    expect(normalizeKey('cmd+s')).toBe('ctrl+s')
  })
})
