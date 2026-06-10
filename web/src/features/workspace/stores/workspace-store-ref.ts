import type { WorkspaceStore } from './workspace-store'

let _activeWorkspaceStore: WorkspaceStore | null = null
const _listeners = new Set<(store: WorkspaceStore | null) => void>()

export function setActiveWorkspaceStoreRef(store: WorkspaceStore | null): void {
  _activeWorkspaceStore = store

  if (store !== null) {
    // Non-null: notify immediately so the new workspace is available synchronously.
    for (const fn of _listeners) fn(store)
  } else {
    // Null: defer one microtask so a same-tick workspace switch can set the new
    // store before subscribers are notified. If the ref is non-null by the time
    // the microtask fires, the null notification is suppressed entirely.
    queueMicrotask(() => {
      if (_activeWorkspaceStore === null) {
        for (const fn of _listeners) fn(null)
      }
    })
  }
}

/**
 * Returns the active workspace store for imperative (non-React) access.
 * Returns null when no workspace is active. Always null-check the result.
 */
export function getActiveWorkspaceStoreRef(): WorkspaceStore | null {
  return _activeWorkspaceStore
}

/**
 * Register a listener that fires whenever the active workspace store changes.
 * Fires immediately with the current store so late registrants don't miss it.
 * Returns an unsubscribe function.
 */
export function onActiveWorkspaceStoreChange(
  listener: (store: WorkspaceStore | null) => void,
): () => void {
  _listeners.add(listener)
  listener(_activeWorkspaceStore)
  return () => {
    _listeners.delete(listener)
  }
}
