import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const rows = new Map<string, { row: SidebarRow }>()

export function registerRecentsRow(row: SidebarRow): () => void {
  const entry = { row }
  rows.set(row.id, entry)
  return () => {
    if (rows.get(row.id) === entry) rows.delete(row.id)
  }
}

export function recentsRowOf(chatId: string): SidebarRow | undefined {
  return rows.get(chatId)?.row
}
