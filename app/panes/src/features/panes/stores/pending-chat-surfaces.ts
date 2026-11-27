import { createStore } from 'zustand'

interface PendingChatSurfacesState {
  pending: ReadonlySet<string>
  begin: (paneId: string) => void
  end: (paneId: string) => void
}

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
