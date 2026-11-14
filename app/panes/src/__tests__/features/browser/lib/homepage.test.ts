import { describe, expect, it } from 'vitest'
import { browserHomepage } from '@/features/browser/lib/homepage'
import { getDefaultSettingsSnapshot } from '@/features/settings/config/default-settings'

describe('browserHomepage', () => {
  it('is undefined by default so a new tab keeps the blank address bar', () => {
    expect(browserHomepage(getDefaultSettingsSnapshot())).toBeUndefined()
  })

  it('normalizes the configured page like the address bar would', () => {
    const settings = { ...getDefaultSettingsSnapshot(), browserHomepage: 'example.com' }
    expect(browserHomepage(settings)).toBe('https://example.com')
  })
})
