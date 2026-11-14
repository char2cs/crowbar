import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// The live row of every chat a view tab shows, so a drag that starts anywhere (a tab, a
// pane's handle) can resolve the tab it is dropped beside. A row lives while its tab does.
const rows = new Map<string, { row: SidebarRow }>()

/** Returns the unregister to run when the tab unmounts; a row registered since is kept. */
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
