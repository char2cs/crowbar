import { createStore } from 'zustand'

interface PendingChatSurfacesState {
  /** Panes whose chat surface has been placed but has not committed yet. */
  pending: ReadonlySet<string>
  begin: (paneId: string) => void
  end: (paneId: string) => void
}

/**
 * Which chat panes are still waiting for their surface (a lazy chunk plus its
 * first render). Parked views hold back until it is empty, so their mount work
 * cannot take the main thread from the chat the user is waiting on.
 */
export const pendingChatSurfaces = createStore<PendingChatSurfacesState>()((set) => ({
  pending: new Set(),
  begin: (paneId) =>
    set((state) =>
      state.pending.has(paneId) ? state : { pending: new Set(state.pending).add(paneId) },
    ),
  end: (paneId) =>
    set((state) => {
      if (!state.pending.has(paneId)) return state
      const pending = new Set(state.pending)
      pending.delete(paneId)
      return { pending }
    }),
}))
