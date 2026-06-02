import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'

// Diff and chats are fetched through SEPARATE loadable stores so a failure in one
// endpoint can never blank the other. Both share the same `branch-review-data` IDB
// object store, keyed distinctly (`<wsId>:diff` vs `<wsId>:chats`).

export const useBranchReviewDiffStore = create<LoadableSlice<MultiFileDiff>>()((set, get) =>
  createLoadableSlice<MultiFileDiff>({
    store: 'branch-review-data',
    fetcher: (wsId: string) => apiFetch<MultiFileDiff>(`/api/v0/branch-review/${wsId}/diff`),
    cacheKey: (wsId: string) => `${wsId}:diff`,
  })(set, get),
)

export const useBranchReviewChatsStore = create<LoadableSlice<BranchReviewChat[]>>()((set, get) =>
  createLoadableSlice<BranchReviewChat[]>({
    store: 'branch-review-data',
    fetcher: (wsId: string) => apiFetch<BranchReviewChat[]>(`/api/v0/branch-review/${wsId}/chats`),
    cacheKey: (wsId: string) => `${wsId}:chats`,
  })(set, get),
)
