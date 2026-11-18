import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { PAGE_BYTES } from '@/features/agent/lib/feed-window'
import { apiFetch } from '@/lib/api/fetch'
import { chatBaseOn } from '@/lib/workspace-scope-url'

/** Whole turns older than a row the client holds, as previews: `from` is the seq the first one
 *  follows, and `more` says rows before it exist. */
export interface RowsPage {
  rows: LedgerRow[]
  from: number
  more: boolean
}

/** The newest whole turns before row `before`, within the page's wire size. */
export function readOlderRows(hostId: string, chatId: string, before: number): Promise<RowsPage> {
  return apiFetch<RowsPage>(
    `${chatBaseOn(hostId, chatId)}/rows?before=${before}&bytes=${PAGE_BYTES}`,
  )
}

/** Row `seq` whole, as a replay's preview of it left fields out. */
export function readRow(hostId: string, chatId: string, seq: number): Promise<LedgerRow> {
  return apiFetch<LedgerRow>(`${chatBaseOn(hostId, chatId)}/rows/${seq}`)
}
