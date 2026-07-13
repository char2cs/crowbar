import { getDB } from './idb'
import type { SidebarUI } from './schemas'

export async function saveSidebarUI(
  collapsedRepos: string[],
  collapsedWorkspaces: string[],
): Promise<void> {
  const db = await getDB()
  await db.put(
    'sidebar-ui',
    { collapsedRepos, collapsedWorkspaces, updatedAt: Date.now() },
    'global',
  )
}

export async function loadSidebarUI(): Promise<SidebarUI | null> {
  const db = await getDB()
  return (await db.get('sidebar-ui', 'global')) ?? null
}
