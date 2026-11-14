import { getDB } from './idb'
import type { SidebarUI } from './schemas'

// Every collapse set the sidebar persists, as one record: four positional string[] parameters
// would be interchangeable to the type checker, so a transposed pair would fold the wrong tree.
export type SidebarCollapse = Omit<SidebarUI, 'updatedAt'>

export async function saveSidebarUI(collapse: SidebarCollapse): Promise<void> {
  const db = await getDB()
  await db.put('sidebar-ui', { ...collapse, updatedAt: Date.now() }, 'global')
}

export async function loadSidebarUI(): Promise<SidebarUI | null> {
  const db = await getDB()
  return (await db.get('sidebar-ui', 'global')) ?? null
}
