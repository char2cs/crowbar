import type { NativeAttachSheet } from '@/lib/native-chrome'

export const LIMIT_MB = 25

type Status = 'uploading' | 'done' | 'bad'

export interface Staged {
  id: number
  name: string
  bytes: number | null
  status: Status
  retry?: () => void
  loaded?: number
  markdown?: string
  problem?: string
}

function sizeLabel(bytes: number | null): string {
  if (bytes === null) return ''
  if (bytes < 1024 * 1024) return `${Math.max(1, Math.round(bytes / 1024))} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1).replace(/\.0$/, '')} MB`
}

export function rowNote(item: Staged): string {
  const size = sizeLabel(item.bytes)
  if (item.status === 'uploading' && item.bytes && item.loaded !== undefined) {
    return `${sizeLabel(item.loaded)} of ${size} · Uploading…`
  }
  const note =
    item.status === 'done' ? 'Uploaded' : item.status === 'uploading' ? 'Uploading…' : item.problem
  return [size, note].filter(Boolean).join(' · ')
}

export function percentOf(item: Staged): number | null {
  return item.bytes && item.loaded !== undefined
    ? Math.round((item.loaded / item.bytes) * 100)
    : null
}

export interface AttachSummary {
  ready: boolean
  attachLabel: string
  hint: string | null
}

export function summarize(items: Staged[]): AttachSummary {
  const done = items.filter((item) => item.status === 'done').length
  const worth = items.filter((item) => item.status !== 'bad').length
  const uploading = worth > done
  return {
    ready: done > 0 && !uploading,
    attachLabel: worth === 0 ? 'Attach files' : `Attach ${worth} ${worth === 1 ? 'file' : 'files'}`,
    hint: uploading ? `Uploading ${done} of ${worth}…` : null,
  }
}

export function attachSheetFiles(items: Staged[]): NativeAttachSheet['files'] {
  return items.map((item) => ({
    id: String(item.id),
    name: item.name,
    detail: rowNote(item),
    status: item.status,
    percent: item.status === 'uploading' ? percentOf(item) : null,
    canRetry: item.retry !== undefined,
  }))
}
