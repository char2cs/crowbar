import { create } from 'zustand'
import { createLoadableSlice, type LoadableSlice } from '@/lib/store/loadable-slice'
import { apiFetch } from '@/lib/api'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'

export interface BranchReviewData {
  diff: MultiFileDiff
  chats: BranchReviewChat[]
}

async function fetchBranchReviewData(wsId: string): Promise<BranchReviewData> {
  const [diff, chats] = await Promise.all([
    apiFetch<MultiFileDiff>(`/api/v0/branch-review/${wsId}/diff`),
    apiFetch<BranchReviewChat[]>(`/api/v0/branch-review/${wsId}/chats`),
  ])
  return { diff, chats }
}

export const useBranchReviewDataStore = create<LoadableSlice<BranchReviewData>>()((set, get) =>
  createLoadableSlice<BranchReviewData>({
    store: 'branch-review-data',
    fetcher: fetchBranchReviewData,
  })(set, get),
)
