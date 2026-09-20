type Platform = 'macos' | 'windows' | 'linux'

/**
 * Single source of truth for platform detection.
 * Falls back to a sensible default when evaluated outside a browser/webview
 * (for example during unit tests in node) so modules that transitively import
 * from platform can still load without a window reference.
 */
function detectPlatform(): Platform {
  if (typeof window === 'undefined') {
    if (typeof process !== 'undefined' && process.platform) {
      if (process.platform === 'darwin') return 'macos'
      if (process.platform === 'win32') return 'windows'
      if (process.platform === 'linux') return 'linux'
    }
    return 'macos'
  }

  // Mirrors appearance-bootstrap.ts's isWindowsPlatform() UA-sniffing approach —
  // Tauri v2 dropped OS detection from core (it's a separate plugin we don't
  // depend on), so the webview's own user agent is the only signal available.
  const ua = typeof navigator !== 'undefined' ? navigator.userAgent : ''
  if (/Windows/i.test(ua)) return 'windows'
  if (/Linux/i.test(ua)) return 'linux'
  return 'macos'
}

export const currentPlatform: Platform = detectPlatform()

export const IS_MAC: boolean = currentPlatform === 'macos'
export const IS_WINDOWS: boolean = currentPlatform === 'windows'
export const IS_LINUX: boolean = currentPlatform === 'linux'

/**
 * Normalize key combination for current platform.
 * Converts 'cmd' to 'ctrl' on Windows/Linux.
 */
export function normalizeKey(key: string): string {
  if (IS_MAC) return key
  return key.replace(/\bcmd\b/gi, 'ctrl')
}
