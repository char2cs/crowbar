import { create } from 'zustand'

/** The branch whose pull was refused because it can't fast-forward. */
export interface PullConflictTarget {
  wsId: string
  branch: string
}

interface PullConflictModalState {
  target: PullConflictTarget | null
  open: (target: PullConflictTarget) => void
  close: () => void
}

// Global UI store (mirrors detach-modal-store): the daemon refuses a
// non-fast-forwardable pull with HTTP 409 `not_fast_forwardable`; the Pull
// triggers read that code and open this single inform-only modal, rendered once
// at the shell level, instead of a generic error toast.
export const usePullConflictModalStore = create<PullConflictModalState>()((set) => ({
  target: null,
  open: (target) => set({ target }),
  close: () => set({ target: null }),
}))
