import { describe, expect, it } from 'vitest'
import { defaultSettings } from '@/features/settings/config/default-settings'
import {
  SETTINGS_EXPORT_FILENAME,
  buildSettingsExportFile,
} from '@/features/settings/lib/settings-download'
import { parseSettingsImportJson } from '@/features/settings/lib/settings-import-export'

describe('settings download', () => {
  it('builds a pretty-printed export file with the expected filename', () => {
    const { filename, contents } = buildSettingsExportFile({
      ...defaultSettings,
      fontSize: 17,
    })

    expect(filename).toBe(SETTINGS_EXPORT_FILENAME)
    expect(filename).toBe('crowbar-settings.json')
    // Pretty-printed: contains indentation / newlines.
    expect(contents).toContain('\n')
    expect(contents).toContain('  "format": "crowbar.settings"')
  })

  it('produces JSON that round-trips back through the import parser', () => {
    const { contents } = buildSettingsExportFile({
      ...defaultSettings,
      fontSize: 21,
    })

    const parsed = JSON.parse(contents)
    expect(parsed.format).toBe('crowbar.settings')
    expect(parsed.version).toBe(1)
    expect(typeof parsed.exportedAt).toBe('string')

    const reimported = parseSettingsImportJson(contents)
    expect(reimported).not.toBeNull()
    expect(reimported?.fontSize).toBe(21)
  })
})
