import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'

const registry = new Map<string, WorkspaceStore>()

let _activeWorkspaceId: string | null = null

export function setActiveWorkspaceId(wsId: string | null): void {
  if (_activeWorkspaceId === wsId) return
  _activeWorkspaceId = wsId
  for (const listener of [...activeWorkspaceListeners]) listener()
}

const activeWorkspaceListeners = new Set<() => void>()

export function subscribeActiveWorkspaceId(listener: () => void): () => void {
  activeWorkspaceListeners.add(listener)
  return () => activeWorkspaceListeners.delete(listener)
}

export function getActiveWorkspaceId(): string | null {
  return _activeWorkspaceId
}

export function getWorkspaceStore(wsId: string): WorkspaceStore | undefined {
  return registry.get(wsId)
}

let detached: WorkspaceStore | undefined

export function detachedWorkspaceStore(): WorkspaceStore {
  detached ??= createWorkspaceStore('')
  return detached
}

export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    const snapshot = loadFromLocalStorage(wsId) ?? undefined
    const store = createWorkspaceStore(wsId, snapshot)
    registry.set(wsId, store)
    notifyRegistryListeners('registered')
  }
  return registry.get(wsId)!
}

export type WorkspaceRegistryChange = 'registered' | 'destroyed'

const registryListeners = new Set<(change: WorkspaceRegistryChange) => void>()

function notifyRegistryListeners(change: WorkspaceRegistryChange): void {
  for (const listener of registryListeners) listener(change)
}

export function subscribeWorkspaceRegistry(
  callback: (change: WorkspaceRegistryChange) => void,
): () => void {
  registryListeners.add(callback)
  return () => {
    registryListeners.delete(callback)
  }
}

export function destroyWorkspaceStore(wsId: string): void {
  registry.delete(wsId)
  notifyRegistryListeners('destroyed')
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
