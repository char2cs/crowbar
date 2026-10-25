import type { StateCreator } from 'zustand'
import {
  loadBranchReviewViewed,
  saveBranchReviewViewed,
} from '@/lib/persistence/branch-review-viewed'
import type { WorkspaceState } from '../workspace-store.types'

/** One file to mark: its path and the signature it has right now. */
export interface ViewedEntry {
  path: string
  signature: string
}

export type MergeStrategy = 'merge' | 'squash' | 'rebase'

export interface ReviewMessage {
  id: string
  author: string | null
  isAgent: boolean
  body: string
  createdAt: string
  /** The agent provider that wrote this message, resolved against the workspace's
   *  provider list. Absent on every human message and on every agent message
   *  written before attribution existed — which is most of them, forever. */
  providerId?: string
  /** The chat the message came out of. Lives on the message permanently, but the
   *  chat itself is deletable, so an id here is not a promise the chat still
   *  exists. */
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

export interface BranchReviewState {
  description: string
  mergeStrategy: MergeStrategy
  diffStatus: 'idle' | 'loading' | 'loaded' | 'error'
  threads: ReviewThread[]
  conversations: ReviewConversation[]
  /** The changed file the review surface has been asked to scroll to, by PATH.
   *
   *  A path, not an index or a composite key: the surface is fed by the files
   *  summary and addresses everything by path, and the index-based key this
   *  replaced was resolved against a whole-diff cache that no longer exists —
   *  so every click silently resolved to nothing. */
  revealFilePath: string | null
  /** Bumped on every request so clicking the SAME file twice reveals it again
   *  (the path alone would compare equal and the effect would not re-run). */
  revealFileNonce: number
}

export interface BranchReviewSlice {
  branchReview: BranchReviewState
  /** Reviewed files: path -> the signature the file had when it was marked.
   *  Empty until `viewedLoaded`. */
  viewedFiles: Record<string, string>
  viewedLoaded: boolean
  /** Read the persisted viewed map once; a no-op after the first call. Run it
   *  only where the review is shown, never from an always-mounted path. */
  loadBranchReviewViewed: () => Promise<void>
  /** Mark or unmark a batch in one state update and one IDB write. */
  setFilesViewed: (entries: readonly ViewedEntry[], viewed: boolean) => Promise<void>
  setBranchReviewDescription: (description: string) => void
  setBranchReviewMergeStrategy: (strategy: MergeStrategy) => void
  setBranchReviewDiffStatus: (status: BranchReviewState['diffStatus']) => void
  /** Ask the review surface to scroll to a changed file. */
  revealBranchReviewFile: (path: string) => void
  addReviewThread: (thread: ReviewThread) => void
  removeReviewThread: (threadId: string) => void
  addReviewMessage: (threadId: string, message: ReviewMessage) => void
  /** @deprecated Use setReviewThreadResolved(id, true) instead. Kept for backward compat. */
  resolveReviewThread: (threadId: string) => void
  /** Two-way: pass false to reopen. */
  setReviewThreadResolved: (threadId: string, isResolved: boolean) => void
  /** Insert if new id; merge (replace) if id already exists. */
  upsertReviewThread: (thread: ReviewThread) => void
  setBranchReviewConversations: (conversations: ReviewConversation[]) => void
  addReviewConversation: (conversation: ReviewConversation) => void
}

export const INITIAL_BRANCH_REVIEW_STATE: BranchReviewState = {
  description: '',
  mergeStrategy: 'merge',
  diffStatus: 'idle',
  threads: [],
  conversations: [],
  revealFilePath: null,
  revealFileNonce: 0,
}

export const createBranchReviewSlice: StateCreator<
  WorkspaceState,
  [['zustand/immer', never]],
  [],
  BranchReviewSlice
> = (set, get) => ({
  branchReview: { ...INITIAL_BRANCH_REVIEW_STATE },
  viewedFiles: {},
  viewedLoaded: false,

  loadBranchReviewViewed: async () => {
    if (get().viewedLoaded) return
    const persisted = await loadBranchReviewViewed(get().workspaceId)
    // Two concurrent loads must not let the slower one overwrite a toggle
    // applied after the faster one.
    set((s) => {
      if (s.viewedLoaded) return
      s.viewedFiles = persisted ?? {}
      s.viewedLoaded = true
    })
  },

  setFilesViewed: async (entries, viewed) => {
    // Applying before the persisted map is read would write it back without
    // everything not yet loaded.
    await get().loadBranchReviewViewed()
    set((s) => {
      const next = { ...s.viewedFiles }
      for (const { path, signature } of entries) {
        if (viewed) next[path] = signature
        else delete next[path]
      }
      s.viewedFiles = next
    })
    await saveBranchReviewViewed(get().workspaceId, get().viewedFiles)
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

  setReviewThreadResolved: (threadId, isResolved) =>
    set((s) => {
      const t = s.branchReview.threads.find((t) => t.id === threadId)
      if (t) t.isResolved = isResolved
    }),

  upsertReviewThread: (thread) =>
    set((s) => {
      const idx = s.branchReview.threads.findIndex((t) => t.id === thread.id)
      if (idx === -1) {
        s.branchReview.threads.push(thread)
      } else {
        s.branchReview.threads[idx] = thread
      }
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
