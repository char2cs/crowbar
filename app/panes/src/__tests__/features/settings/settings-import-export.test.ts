import { describe, expect, it } from 'vitest'
import { defaultSettings } from '@/features/settings/config/default-settings'
import {
  createSettingsExportPayload,
  parseSettingsImportJson,
} from '@/features/settings/lib/settings-import-export'

describe('settings import/export', () => {
  it('creates a versioned settings export payload', () => {
    const payload = createSettingsExportPayload({
      ...defaultSettings,
      fontSize: 15,
    })

    expect(payload.format).toBe('crowbar.settings')
    expect(payload.version).toBe(1)
    expect(payload.settings.fontSize).toBe(15)
  })

  it('imports raw settings objects and ignores unknown keys', () => {
    const imported = parseSettingsImportJson(
      JSON.stringify({
        fontSize: 17,
        fileTreeDensity: 'unknown',
        unknownSetting: true,
      }),
    )

    expect(imported?.fontSize).toBe(17)
    // A known key with an out-of-range value is normalized, not dropped.
    expect(imported?.fileTreeDensity).toBe('default')
    expect('unknownSetting' in (imported as object)).toBe(false)
  })

  it('imports versioned settings payloads', () => {
    const imported = parseSettingsImportJson(
      JSON.stringify({
        format: 'crowbar.settings',
        version: 1,
        exportedAt: '2026-04-25T00:00:00.000Z',
        settings: {
          ...defaultSettings,
          wordWrap: true,
        },
      }),
    )

    expect(imported?.wordWrap).toBe(true)
  })
})
