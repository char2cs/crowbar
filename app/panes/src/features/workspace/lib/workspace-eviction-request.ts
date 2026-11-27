type EvictionListener = (wsId: string) => void

const listeners = new Set<EvictionListener>()

export function requestWorkspaceEviction(wsId: string): void {
  for (const listener of listeners) listener(wsId)
}

export function subscribeWorkspaceEviction(listener: EvictionListener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
