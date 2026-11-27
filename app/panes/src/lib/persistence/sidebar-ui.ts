import { getDB } from './idb'
import type { SidebarUI } from './schemas'

export type SidebarCollapse = Omit<SidebarUI, 'updatedAt'>

export async function saveSidebarUI(collapse: SidebarCollapse): Promise<void> {
  const db = await getDB()
  await db.put('sidebar-ui', { ...collapse, updatedAt: Date.now() }, 'global')
}

export async function loadSidebarUI(): Promise<SidebarUI | null> {
  const db = await getDB()
  return (await db.get('sidebar-ui', 'global')) ?? null
}
