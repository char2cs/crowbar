import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'

const registry = new Map<string, WorkspaceStore>()

export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    const snapshot = loadFromLocalStorage(wsId)
    registry.set(wsId, createWorkspaceStore(wsId, snapshot ?? undefined))
  }
  return registry.get(wsId)!
}

export function destroyWorkspaceStore(wsId: string): void {
  registry.delete(wsId)
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
