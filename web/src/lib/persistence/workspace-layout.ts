import { getDB } from './idb'
import type { WorkspaceLayout } from './schemas'

export async function saveWorkspaceLayout(layout: WorkspaceLayout): Promise<void> {
  const db = await getDB()
  await db.put('workspace-layout', { ...layout, updatedAt: Date.now() })
}

export async function loadWorkspaceLayout(workspaceId: string): Promise<WorkspaceLayout | null> {
  const db = await getDB()
  return (await db.get('workspace-layout', workspaceId)) ?? null
}
