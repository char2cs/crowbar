import { getBaseName } from '@/utils/path-helpers'

/** What the Properties dialog shows for one file. */
export interface FileProperties {
  fileName: string
  path: string
  size: string
  type: string
}

function formatFileSize(sizeHeader: string | null): string {
  const bytes = Number(sizeHeader)
  if (!Number.isFinite(bytes) || bytes < 0) return 'Unknown'
  if (bytes < 1024) return `${bytes} bytes`

  const units = ['KB', 'MB', 'GB', 'TB']
  let value = bytes / 1024
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }

  return `${value.toFixed(value >= 10 ? 1 : 2)} ${units[unitIndex]}`
}

async function fileSize(path: string): Promise<string> {
  try {
    const stats = await fetch(`file://${path}`, { method: 'HEAD' })
    return formatFileSize(stats.headers.get('content-length'))
  } catch {
    return 'Unknown' // a webview that cannot stat the file still shows the rest
  }
}

export async function readFileProperties(path: string): Promise<FileProperties> {
  const fileName = getBaseName(path, '')
  const extension = fileName.includes('.') ? fileName.split('.').pop() : undefined
  return { fileName, path, size: await fileSize(path), type: extension || 'No extension' }
}
