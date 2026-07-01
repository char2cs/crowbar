import { createSettingsExportPayload } from '@/features/settings/lib/settings-import-export'
import type { Settings } from '@/features/settings/types/settings'

export const SETTINGS_EXPORT_FILENAME = 'crowbar-settings.json'

/**
 * Serialize the given settings into the versioned export envelope and
 * pretty-print it as JSON. Pure — safe to unit test without a DOM.
 */
export function buildSettingsExportFile(settings: Settings): {
  filename: string
  contents: string
} {
  const payload = createSettingsExportPayload(settings)
  return {
    filename: SETTINGS_EXPORT_FILENAME,
    contents: JSON.stringify(payload, null, 2),
  }
}

/**
 * Trigger a browser download of the given settings as a JSON file.
 * Creates a Blob, an object URL, clicks a temporary anchor, then revokes
 * the URL. No-op safe to call only in a DOM environment.
 */
export function downloadSettingsFile(settings: Settings): void {
  const { filename, contents } = buildSettingsExportFile(settings)
  const blob = new Blob([contents], { type: 'application/json' })
  const url = URL.createObjectURL(blob)

  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()

  URL.revokeObjectURL(url)
}
