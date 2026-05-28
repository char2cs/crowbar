import type { WorkspaceStore } from './workspace-store'

let _activeWorkspaceStore: WorkspaceStore | null = null

const _workspaceStoreListeners = new Set<(store: WorkspaceStore | null) => void>()

export function setActiveWorkspaceStoreRef(store: WorkspaceStore | null): void {
  _activeWorkspaceStore = store
  for (const listener of _workspaceStoreListeners) {
    listener(store)
  }
}

/**
 * Returns the active workspace store for imperative (non-React) access.
 * Returns null during workspace transitions — always null-check the result.
 */
export function getActiveWorkspaceStoreRef(): WorkspaceStore | null {
  return _activeWorkspaceStore
}

/**
 * Register a listener that fires whenever the active workspace store changes
 * (including when it is set to null during workspace teardown).
 * Returns an unsubscribe function.
 *
 * Use this to wire up store subscriptions that need to follow the active workspace.
 */
export function onActiveWorkspaceStoreChange(
  listener: (store: WorkspaceStore | null) => void,
): () => void {
  _workspaceStoreListeners.add(listener)
  // Fire immediately with the current store so late registrants don't miss it.
  listener(_activeWorkspaceStore)
  return () => { _workspaceStoreListeners.delete(listener) }
}
