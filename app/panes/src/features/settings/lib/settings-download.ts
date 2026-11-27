import { createSettingsExportPayload } from '@/features/settings/lib/settings-import-export'
import type { Settings } from '@/features/settings/types/settings'

const SETTINGS_EXPORT_FILENAME = 'crowbar-settings.json'

function buildSettingsExportFile(settings: Settings): {
  filename: string
  contents: string
} {
  const payload = createSettingsExportPayload(settings)
  return {
    filename: SETTINGS_EXPORT_FILENAME,
    contents: JSON.stringify(payload, null, 2),
  }
}

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
