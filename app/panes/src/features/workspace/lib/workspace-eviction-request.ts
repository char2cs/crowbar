type EvictionListener = (wsId: string) => void

const listeners = new Set<EvictionListener>()

// Asks `WorkspaceHost` to drop `wsId` now (its last view was closed). A request, not a destroy:
// only the host may destroy a store, after React unmounts its subtree. No-op with no host.
export function requestWorkspaceEviction(wsId: string): void {
  for (const listener of listeners) listener(wsId)
}

export function subscribeWorkspaceEviction(listener: EvictionListener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
