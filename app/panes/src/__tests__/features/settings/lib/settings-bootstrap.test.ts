import { beforeEach, describe, expect, it } from 'vitest'
import { initializeSettingsState } from '@/features/settings/lib/settings-bootstrap'

describe('initializeSettingsState', () => {
  beforeEach(() => {
    localStorage.clear()
  })

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

  it('writes back only what was stored, never a default', async () => {
    localStorage.setItem('crowbar:settings:fontFamily', JSON.stringify('Geist Mono'))

    await initializeSettingsState(() => {})

    const keys = Array.from({ length: localStorage.length }, (_, i) => localStorage.key(i) ?? '')
    const written = keys.filter((key) => key.startsWith('crowbar:settings:'))
    expect(written).toEqual(['crowbar:settings:fontFamily'])
  })
})
