type Platform = 'macos' | 'windows' | 'linux' | 'ios'

function detectPlatform(): Platform {
  if (typeof window === 'undefined') {
    if (typeof process !== 'undefined' && process.platform) {
      if (process.platform === 'darwin') return 'macos'
      if (process.platform === 'win32') return 'windows'
      if (process.platform === 'linux') return 'linux'
    }
    return 'macos'
  }

  const ua = typeof navigator !== 'undefined' ? navigator.userAgent : ''
  if (/iPhone|iPad|iPod/i.test(ua)) return 'ios'
  if (/Macintosh/i.test(ua) && (navigator.maxTouchPoints ?? 0) > 1) return 'ios'
  if (/Windows/i.test(ua)) return 'windows'
  if (/Linux/i.test(ua)) return 'linux'
  return 'macos'
}

export const currentPlatform: Platform = detectPlatform()

export const IS_MAC: boolean = currentPlatform === 'macos'
export const IS_WINDOWS: boolean = currentPlatform === 'windows'
export const IS_MOBILE: boolean = currentPlatform === 'ios'
export const APPLE_KEYBOARD: boolean = IS_MAC || IS_MOBILE

export function normalizeKey(key: string): string {
  if (APPLE_KEYBOARD) return key
  return key.replace(/\bcmd\b/gi, 'ctrl')
}
