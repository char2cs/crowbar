import { createStore } from 'zustand'

interface PendingChatSurfacesState {
  /** Panes whose chat surface has been placed but has not committed yet. */
  pending: ReadonlySet<string>
  begin: (paneId: string) => void
  end: (paneId: string) => void
}

/** Chat panes still waiting for their surface (chunk plus first render). Parked views hold back
 *  until it empties, so their mounting never takes the main thread from the awaited chat. */
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
