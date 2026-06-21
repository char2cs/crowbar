import type { StateCreator } from 'zustand'
import type { WorkspaceState } from '../workspace-store.types'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'

export type MergeStrategy = 'merge' | 'squash' | 'rebase'

export interface ReviewMessage {
  id: string
  author: string | null
  isAgent: boolean
  body: string
  createdAt: string
}

export interface ReviewThread {
  id: string
  filePath: string
  lineNumber: number
  side: 'left' | 'right'
  messages: ReviewMessage[]
  isResolved: boolean
}

export interface ReviewConversation {
  id: string
  title: string
  age: string
  isActive: boolean
}

export interface BranchReviewState {
  description: string
  mergeStrategy: MergeStrategy
  activeSubtab: 'about' | 'git' | 'diff'
  diffCache: MultiFileDiff | null
  diffStatus: 'idle' | 'loading' | 'loaded' | 'error'
  threads: ReviewThread[]
  conversations: ReviewConversation[]
  activeFileKey: string | null
  activeFileNonce: number
}

export interface BranchReviewSlice {
  branchReview: BranchReviewState
  setBranchReviewDescription: (description: string) => void
  setBranchReviewMergeStrategy: (strategy: MergeStrategy) => void
  setBranchReviewSubtab: (tab: BranchReviewState['activeSubtab']) => void
  setBranchReviewDiff: (diff: MultiFileDiff) => void
  setBranchReviewDiffStatus: (status: BranchReviewState['diffStatus']) => void
  setBranchReviewActiveFile: (key: string | null) => void
  addReviewThread: (thread: ReviewThread) => void
  removeReviewThread: (threadId: string) => void
  addReviewMessage: (threadId: string, message: ReviewMessage) => void
  resolveReviewThread: (threadId: string) => void
  setBranchReviewConversations: (conversations: ReviewConversation[]) => void
  addReviewConversation: (conversation: ReviewConversation) => void
}

export const INITIAL_BRANCH_REVIEW_STATE: BranchReviewState = {
  description: '',
  mergeStrategy: 'merge',
  activeSubtab: 'about',
  diffCache: null,
  diffStatus: 'idle',
  threads: [],
  conversations: [],
  activeFileKey: null,
  activeFileNonce: 0,
}

export interface OptimisticOp {
  apply: () => void
  rollback: () => void
  commit: () => Promise<void>
}

/**
 * Optimistic mutation: apply the local change immediately, then attempt the
 * server commit. If the commit throws, roll the local change back. Returns
 * whether the commit succeeded so the caller can surface a toast on failure
 * (stores never import components).
 */
export async function addThreadOptimistic({
  apply,
  rollback,
  commit,
}: OptimisticOp): Promise<boolean> {
  apply()
  try {
    await commit()
    return true
  } catch {
    rollback()
    return false
  }
}

export const createBranchReviewSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BranchReviewSlice
> = (set) => ({
  branchReview: { ...INITIAL_BRANCH_REVIEW_STATE },

  setBranchReviewDescription: (description) =>
    set((s) => {
      s.branchReview.description = description
    }),

  setBranchReviewMergeStrategy: (strategy) =>
    set((s) => {
      s.branchReview.mergeStrategy = strategy
    }),

  setBranchReviewSubtab: (tab) =>
    set((s) => {
      s.branchReview.activeSubtab = tab
    }),

  setBranchReviewDiff: (diff) =>
    set((s) => {
      s.branchReview.diffCache = diff
      s.branchReview.diffStatus = 'loaded'
    }),

  setBranchReviewDiffStatus: (status) =>
    set((s) => {
      s.branchReview.diffStatus = status
    }),

  setBranchReviewActiveFile: (key) =>
    set((s) => {
      s.branchReview.activeFileKey = key
      s.branchReview.activeFileNonce += 1
      s.branchReview.activeSubtab = 'diff'
    }),

  addReviewThread: (thread) =>
    set((s) => {
      s.branchReview.threads.push(thread)
    }),

  removeReviewThread: (threadId) =>
    set((s) => {
      s.branchReview.threads = s.branchReview.threads.filter((t) => t.id !== threadId)
    }),

  addReviewMessage: (threadId, message) =>
    set((s) => {
      const t = s.branchReview.threads.find((t) => t.id === threadId)
      if (t) t.messages.push(message)
    }),

  resolveReviewThread: (threadId) =>
    set((s) => {
      const t = s.branchReview.threads.find((t) => t.id === threadId)
      if (t) t.isResolved = true
    }),

  setBranchReviewConversations: (conversations) =>
    set((s) => {
      s.branchReview.conversations = conversations
    }),

  addReviewConversation: (conversation) =>
    set((s) => {
      s.branchReview.conversations.unshift(conversation)
    }),
})
