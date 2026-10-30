import { create } from 'zustand'

/** A create the user is still naming: the sidebar draws an input at its slot. */
export interface CreateDraft {
  kind: 'branch' | 'chat'
  projectId: string
  repoId?: string
  /** The workspace the create forks from or runs in. */
  workspaceId: string
  /** The SidebarRow.parentId the created row lands under. */
  parentId: string
  order: number
  ownsWorktree: boolean
}

interface CreateDraftState {
  draft: CreateDraft | null
  /** Opens the naming input; one is open at a time, so it replaces any other. */
  start(d: CreateDraft): void
  cancel(): void
}

export const useCreateDraftStore = create<CreateDraftState>()((set) => ({
  draft: null,
  start: (draft) => set({ draft }),
  cancel: () => set({ draft: null }),
}))
