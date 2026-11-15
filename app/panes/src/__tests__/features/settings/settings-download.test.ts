import { afterEach, describe, expect, it, vi } from 'vitest'
import { defaultSettings } from '@/features/settings/config/default-settings'
import { downloadSettingsFile } from '@/features/settings/lib/settings-download'
import { parseSettingsImportJson } from '@/features/settings/lib/settings-import-export'

async function download(fontSize: number): Promise<{ filename: string; contents: string }> {
  const createObjectURL = vi.fn((_blob: Blob) => 'blob:settings')
  vi.stubGlobal('URL', Object.assign(URL, { createObjectURL, revokeObjectURL: vi.fn() }))
  let filename = ''
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (
    this: HTMLAnchorElement,
  ) {
    filename = this.download
  })

  downloadSettingsFile({ ...defaultSettings, fontSize })

  const blob = createObjectURL.mock.calls[0][0]
  return { filename, contents: await blob.text() }
}

describe('downloadSettingsFile', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it('downloads a pretty-printed export file with the expected filename', async () => {
    const { filename, contents } = await download(17)

    expect(filename).toBe('crowbar-settings.json')
    expect(contents).toContain('\n')
    expect(contents).toContain('  "format": "crowbar.settings"')
  })

  it('produces JSON that round-trips back through the import parser', async () => {
    const { contents } = await download(21)

    const parsed = JSON.parse(contents)
    expect(parsed.format).toBe('crowbar.settings')
    expect(parsed.version).toBe(1)
    expect(typeof parsed.exportedAt).toBe('string')

    const reimported = parseSettingsImportJson(contents)
    expect(reimported).not.toBeNull()
    expect(reimported?.fontSize).toBe(21)
  })
})
