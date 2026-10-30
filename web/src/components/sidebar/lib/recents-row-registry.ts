import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// The live row of every chat a view tab shows, so a drag that starts anywhere (a tab, a
// pane's handle) can resolve the tab it is dropped beside.
const rows = new Map<string, SidebarRow>()

export function registerRecentsRow(row: SidebarRow): void {
  rows.set(row.id, row)
}

export function recentsRowOf(chatId: string): SidebarRow | undefined {
  return rows.get(chatId)
}
