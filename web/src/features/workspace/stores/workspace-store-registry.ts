import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'

const registry = new Map<string, WorkspaceStore>()

let _activeWorkspaceId: string | null = null

// Scoped URLs are hierarchical (/v0/projects/:p/repos/:r/workspaces/:w/...); the wsId -> scope map
// lives in dependency-free `@/lib/workspace-scope` so URL builders don't import this store graph.
export { setWorkspaceScope, getWorkspaceScope, type WorkspaceScope } from '@/lib/workspace-scope'

// Written only by `WorkspaceHost`, in the layout effect that mounts the stores, so every reader
// agrees within a commit. Scoped-URL builders never read it: they name their workspace.
export function setActiveWorkspaceId(wsId: string | null): void {
  if (_activeWorkspaceId === wsId) return
  _activeWorkspaceId = wsId
  for (const listener of [...activeWorkspaceListeners]) listener()
}

const activeWorkspaceListeners = new Set<() => void>()

/** Notified when the active workspace id changes. */
export function subscribeActiveWorkspaceId(listener: () => void): () => void {
  activeWorkspaceListeners.add(listener)
  return () => activeWorkspaceListeners.delete(listener)
}

/** Test-only: make `store` (registered under its own workspace id) the
 *  active one, or clear the active id — what the host does for a real mount. */
export function setActiveWorkspaceStoreForTests(store: WorkspaceStore | null): void {
  if (!store) {
    _activeWorkspaceId = null
    return
  }
  const wsId = store.getState().workspaceId
  registry.set(wsId, store)
  _activeWorkspaceId = wsId
}

export function getActiveWorkspaceStore(): WorkspaceStore | null {
  if (!_activeWorkspaceId) return null
  return registry.get(_activeWorkspaceId) ?? null
}

export function getActiveWorkspaceId(): string | null {
  return _activeWorkspaceId
}

// Never mints: a buffer can outlive its workspace's eviction, and minting for it would register a
// store `WorkspaceHost` never mounted and will never destroy.
export function getWorkspaceStore(wsId: string): WorkspaceStore | undefined {
  return registry.get(wsId)
}

let detached: WorkspaceStore | undefined

/** An empty store that belongs to no workspace and is never registered — the
 *  read-only stand-in a render path gets for a workspace that is not mounted. */
export function detachedWorkspaceStore(): WorkspaceStore {
  detached ??= createWorkspaceStore('')
  return detached
}

// Only `WorkspaceHost`'s reconcile effect calls this, never render, so the registry's keys are
// exactly its mounted set. Everything else reads with getWorkspaceStore.
export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    // Pane/buffer layout is window-level (window-pane-store.ts); this snapshot restores only
    // recentFiles and terminalLayout.
    const snapshot = loadFromLocalStorage(wsId) ?? undefined
    const store = createWorkspaceStore(wsId, snapshot)
    registry.set(wsId, store)
    notifyRegistryListeners('registered')
  }
  return registry.get(wsId)!
}

// For code watching a store it must not mint. A 'registered' store is empty, so watchers re-bind
// without pushing; only 'destroyed' (always from an effect) changes an answer.
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
  // After the delete, so a watcher re-binding on this signal sees the store
  // already gone rather than re-attaching to the one being torn down.
  notifyRegistryListeners('destroyed')
}

export function getAllActiveWorkspaceIds(): string[] {
  return Array.from(registry.keys())
}
