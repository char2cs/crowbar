import { getDB } from './idb'
import type { SidebarUI } from './schemas'

/**
 * Every collapse set the sidebar persists, written as ONE record.
 *
 * An object rather than four positional string[] parameters: the lists are
 * mutually interchangeable to the type checker, so a transposed pair would fold
 * away the wrong tree with a green build.
 */
export type SidebarCollapse = Omit<SidebarUI, 'updatedAt'>

export async function saveSidebarUI(collapse: SidebarCollapse): Promise<void> {
  const db = await getDB()
  await db.put('sidebar-ui', { ...collapse, updatedAt: Date.now() }, 'global')
}

export async function loadSidebarUI(): Promise<SidebarUI | null> {
  const db = await getDB()
  return (await db.get('sidebar-ui', 'global')) ?? null
}
