import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { ReviewThread, ReviewMessage, MergeStrategy } from '@/features/branch-review/types/review-types'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'

export interface BranchReviewState {
  description: string
  mergeStrategy: MergeStrategy
  activeSubtab: 'about' | 'commits' | 'diff'
  diffCache: MultiFileDiff | null
  diffStatus: 'idle' | 'loading' | 'loaded' | 'error'
  threads: ReviewThread[]
}

export interface BranchReviewSlice {
  branchReview: BranchReviewState
  setBranchReviewDescription: (description: string) => void
  setBranchReviewMergeStrategy: (strategy: MergeStrategy) => void
  setBranchReviewSubtab: (tab: BranchReviewState['activeSubtab']) => void
  setBranchReviewDiff: (diff: MultiFileDiff) => void
  setBranchReviewDiffStatus: (status: BranchReviewState['diffStatus']) => void
  addReviewThread: (thread: ReviewThread) => void
  addReviewMessage: (threadId: string, message: ReviewMessage) => void
  resolveReviewThread: (threadId: string) => void
}

export const INITIAL_BRANCH_REVIEW_STATE: BranchReviewState = {
  description: '',
  mergeStrategy: 'merge',
  activeSubtab: 'about',
  diffCache: null,
  diffStatus: 'idle',
  threads: [],
}

export const createBranchReviewSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BranchReviewSlice
> = (set) => ({
  branchReview: { ...INITIAL_BRANCH_REVIEW_STATE },

  setBranchReviewDescription: (description) =>
    set(s => { s.branchReview.description = description }),

  setBranchReviewMergeStrategy: (strategy) =>
    set(s => { s.branchReview.mergeStrategy = strategy }),

  setBranchReviewSubtab: (tab) =>
    set(s => { s.branchReview.activeSubtab = tab }),

  setBranchReviewDiff: (diff) =>
    set(s => { s.branchReview.diffCache = diff; s.branchReview.diffStatus = 'loaded' }),

  setBranchReviewDiffStatus: (status) =>
    set(s => { s.branchReview.diffStatus = status }),

  addReviewThread: (thread) =>
    set(s => { s.branchReview.threads.push(thread) }),

  addReviewMessage: (threadId, message) =>
    set(s => {
      const t = s.branchReview.threads.find(t => t.id === threadId)
      if (t) t.messages.push(message)
    }),

  resolveReviewThread: (threadId) =>
    set(s => {
      const t = s.branchReview.threads.find(t => t.id === threadId)
      if (t) t.isResolved = true
    }),
})
