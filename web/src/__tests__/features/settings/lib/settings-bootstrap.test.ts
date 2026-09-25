import { beforeEach, describe, expect, it } from 'vitest'
import { initializeSettingsState } from '@/features/settings/lib/settings-bootstrap'

describe('initializeSettingsState', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  // A profile from the base build saved the static cut this build no longer ships.
  it('rewrites a saved static Geist Mono to the variable cut, once, in storage', async () => {
    localStorage.setItem('crowbar:settings:fontFamily', JSON.stringify('Geist Mono'))
    localStorage.setItem('crowbar:settings:terminalFontFamily', JSON.stringify('Geist Mono'))

    const settings = await initializeSettingsState(() => {})

    expect(settings.fontFamily).toBe('Geist Mono Variable')
    expect(settings.terminalFontFamily).toBe('Geist Mono Variable')
    expect(JSON.parse(localStorage.getItem('crowbar:settings:fontFamily')!)).toBe(
      'Geist Mono Variable',
    )
    expect(JSON.parse(localStorage.getItem('crowbar:settings:terminalFontFamily')!)).toBe(
      'Geist Mono Variable',
    )
  })
})
