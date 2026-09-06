type EvictionListener = (wsId: string) => void

const listeners = new Set<EvictionListener>()

/**
 * "Drop `wsId` NOW, whatever the keep-alive window says."
 *
 * `WorkspaceHost`'s retention policy (`planRetention`) answers one question —
 * "is this workspace worth keeping warm for a fast switch back?" — and
 * everything it protects, it protects on the theory that the user might
 * return to it in a moment. A workspace whose last VIEW was just closed is
 * the opposite case: the user ended it, its chats' vendor CLIs have been
 * stopped, and nothing is coming back to it. Aging it out over the next
 * several minutes would keep a whole live workspace (its chats stream, LSP,
 * terminal transports, file watcher, Monaco registry) resident for a surface
 * the user closed.
 *
 * This is deliberately a REQUEST, not a `destroyWorkspaceStore` call. Only
 * `WorkspaceHost` may destroy a store it mounted, and only once React has
 * torn the subtree down — destroying one out from under a mounted
 * `WorkspaceView` is precisely the "no Monaco pane or terminal slot is ever
 * live over a destroyed store" rule that host's own deferred-destroy effect
 * exists to keep. So the close path asks, and the host does it through the
 * same unmount-then-destroy path an ordinary eviction takes.
 *
 * A no-op when no host is listening (nothing mounted the workspace, so there
 * is no store to evict).
 */
export function requestWorkspaceEviction(wsId: string): void {
  for (const listener of listeners) listener(wsId)
}

export function subscribeWorkspaceEviction(listener: EvictionListener): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}
