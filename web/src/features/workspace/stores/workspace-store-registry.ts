import { createWorkspaceStore, type WorkspaceStore } from './workspace-store'
import { loadFromLocalStorage } from './workspace-persistence'
import { bindActiveWorkspaceId } from '@/lib/workspace-scope'

const registry = new Map<string, WorkspaceStore>()

let _activeWorkspaceId: string | null = null

// §3/§7: workspace-scoped API/WS URLs are now hierarchical
// (/v0/projects/:p/repos/:r/workspaces/:w/...). The owning project+repo of the
// active workspace are threaded from the TanStack route and recorded so the many
// wsId-keyed callers (files, git, terminal, editor) can resolve the full scope
// without every signature growing two params. The scope MAP itself lives in the
// dependency-free `@/lib/workspace-scope` module so those lightweight builders
// don't import this registry and the whole workspace-store graph behind it.
// We re-export the setter/getter here for callers that already depend on it.
export { setWorkspaceScope, getWorkspaceScope, type WorkspaceScope } from '@/lib/workspace-scope'

/**
 * THE active workspace (C4) — one id, written by exactly one owner:
 * `WorkspaceHost`, in the same layout effect that mounts the stores, so every
 * reader (scoped URLs, the active store, jump navigation) agrees within a
 * commit. The scope module reads it through the binding below rather than
 * keeping a copy of its own.
 */
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

bindActiveWorkspaceId(() => _activeWorkspaceId)

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

/**
 * A registered workspace store, or `undefined` if none exists — never
 * creates one. A buffer can outlive its workspace's eviction, and minting a
 * store for its id would register one `WorkspaceHost` never mounted and will
 * never destroy.
 */
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

/**
 * Mint (or return) `wsId`'s store. Only `WorkspaceHost` calls this in the app
 * — from its reconcile effect, never from render (C6): the registry's keys are
 * exactly the host's mounted set (C5). Everything else reads with
 * {@link getWorkspaceStore} and copes with a workspace that is not mounted.
 */
export function getOrCreateWorkspaceStore(wsId: string): WorkspaceStore {
  if (!registry.has(wsId)) {
    // Task 26: pane/buffer layout no longer lives on this per-workspace
    // snapshot (it's window-level now — see window-pane-store.ts), so the
    // only fields left to restore here are recentFiles/terminalLayout.
    const snapshot = loadFromLocalStorage(wsId) ?? undefined
    const store = createWorkspaceStore(wsId, snapshot)
    registry.set(wsId, store)
    notifyRegistryListeners('registered')
  }
  return registry.get(wsId)!
}

/**
 * Notified whenever a workspace store is registered or destroyed, for code that
 * watches a store it must not mint. A `'registered'` store is empty and minting
 * can happen during render, so watchers re-bind on it without pushing; only
 * `'destroyed'` (always from an effect) changes an answer.
 */
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
