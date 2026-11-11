import { getBaseName } from '@/utils/path-helpers'

/** What the Properties dialog shows for one file. */
export interface FileProperties {
  fileName: string
  path: string
  type: string
}

export function fileProperties(path: string): FileProperties {
  const fileName = getBaseName(path, '')
  const extension = fileName.includes('.') ? fileName.split('.').pop() : undefined
  return { fileName, path, type: extension || 'No extension' }
}
