import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { PAGE_BYTES } from '@/features/agent/lib/feed-window'
import { apiFetch } from '@/lib/api/fetch'
import { chatBaseOn } from '@/lib/workspace-scope-url'

export interface RowsPage {
  rows: LedgerRow[]
  from: number
  more: boolean
}

export function readOlderRows(hostId: string, chatId: string, before: number): Promise<RowsPage> {
  return apiFetch<RowsPage>(
    `${chatBaseOn(hostId, chatId)}/rows?before=${before}&bytes=${PAGE_BYTES}`,
  )
}

export function readRow(hostId: string, chatId: string, seq: number): Promise<LedgerRow> {
  return apiFetch<LedgerRow>(`${chatBaseOn(hostId, chatId)}/rows/${seq}`)
}
