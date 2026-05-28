import type { WorkspaceStore } from './workspace-store'

let _activeWorkspaceStore: WorkspaceStore | null = null

export function setActiveWorkspaceStoreRef(store: WorkspaceStore | null): void {
  _activeWorkspaceStore = store
}

export function getActiveWorkspaceStoreRef(): WorkspaceStore | null {
  return _activeWorkspaceStore
}
