import type { StateCreator } from 'zustand'
import { viewedOwner } from '@/features/git/lib/viewed-owner'
import type { ViewedFile } from '@/sdk/viewed-file'
import type { WorkspaceState } from '../workspace-store.types'

export type MergeStrategy = 'merge' | 'squash' | 'rebase'

export interface ReviewMessage {
  id: string
  author: string | null
  isAgent: boolean
  body: string
  createdAt: string
  providerId?: string
  chatId?: string
}

export interface ReviewThread {
  id: string
  filePath: string
  lineNumber: number
  startLine: number
  endLine: number
  side: 'old' | 'new'
  messages: ReviewMessage[]
  isResolved: boolean
}

export interface ReviewConversation {
  id: string
  title: string
  age: string
  isActive: boolean
}

interface BranchReviewState {
  description: string
  mergeStrategy: MergeStrategy
  diffStatus: 'idle' | 'loading' | 'loaded' | 'error'
  conversations: ReviewConversation[]
  revealFilePath: string | null
  revealFileNonce: number
}

export interface BranchReviewSlice {
  branchReview: BranchReviewState
  viewedFiles: Record<string, string>
  viewedLoaded: boolean
  loadBranchReviewViewed: () => Promise<void>
  setFilesViewed: (entries: readonly ViewedFile[], viewed: boolean) => Promise<void>
  setBranchReviewDescription: (description: string) => void
  setBranchReviewMergeStrategy: (strategy: MergeStrategy) => void
  setBranchReviewDiffStatus: (status: BranchReviewState['diffStatus']) => void
  revealBranchReviewFile: (path: string) => void
  setBranchReviewConversations: (conversations: ReviewConversation[]) => void
  addReviewConversation: (conversation: ReviewConversation) => void
}

const INITIAL_BRANCH_REVIEW_STATE: BranchReviewState = {
  description: '',
  mergeStrategy: 'merge',
  diffStatus: 'idle',
  conversations: [],
  revealFilePath: null,
  revealFileNonce: 0,
}

export const createBranchReviewSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BranchReviewSlice
> = (set, get) => {
  let following: Promise<void> | null = null
  return {
    branchReview: { ...INITIAL_BRANCH_REVIEW_STATE },
    viewedFiles: {},
    viewedLoaded: false,

    loadBranchReviewViewed: () => {
      following ??= new Promise((resolve) => {
        viewedOwner().follow(get().workspaceId, (viewed) => {
          set((s) => {
            s.viewedFiles = viewed
            s.viewedLoaded = true
          })
          resolve()
        })
      })
      return following
    },

    setFilesViewed: async (entries, viewed) => {
      await get().loadBranchReviewViewed()
      await viewedOwner().set(get().workspaceId, { files: entries, viewed })
    },

    setBranchReviewDescription: (description) =>
      set((s) => {
        s.branchReview.description = description
      }),

    setBranchReviewMergeStrategy: (strategy) =>
      set((s) => {
        s.branchReview.mergeStrategy = strategy
      }),

    setBranchReviewDiffStatus: (status) =>
      set((s) => {
        s.branchReview.diffStatus = status
      }),

    revealBranchReviewFile: (path) =>
      set((s) => {
        s.branchReview.revealFilePath = path
        s.branchReview.revealFileNonce += 1
      }),

    setBranchReviewConversations: (conversations) =>
      set((s) => {
        s.branchReview.conversations = conversations
      }),

    addReviewConversation: (conversation) =>
      set((s) => {
        s.branchReview.conversations.unshift(conversation)
      }),
  }
}
