import type { WorkspaceStore } from './workspace-store'

let _activeWorkspaceStore: WorkspaceStore | null = null

export function setActiveWorkspaceStoreRef(store: WorkspaceStore | null): void {
  _activeWorkspaceStore = store
}

/**
 * Returns the active workspace store for imperative (non-React) access.
 * Returns null during workspace transitions — always null-check the result.
 */
export function getActiveWorkspaceStoreRef(): WorkspaceStore | null {
  return _activeWorkspaceStore
}
