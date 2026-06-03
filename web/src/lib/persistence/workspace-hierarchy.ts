import { getDB } from './idb'
import type { WorkspaceHierarchy } from './schemas'

export async function saveWorkspaceHierarchy(
  repoId: string,
  entries: Array<{ wsId: string; parentId?: string }>,
): Promise<void> {
  const db = await getDB()
  await db.put('workspace-hierarchy', { repoId, entries, updatedAt: Date.now() })
}

export async function loadWorkspaceHierarchy(repoId: string): Promise<WorkspaceHierarchy | null> {
  const db = await getDB()
  return (await db.get('workspace-hierarchy', repoId)) ?? null
}

export async function loadAllWorkspaceHierarchies(): Promise<WorkspaceHierarchy[]> {
  const db = await getDB()
  return db.getAll('workspace-hierarchy')
}
