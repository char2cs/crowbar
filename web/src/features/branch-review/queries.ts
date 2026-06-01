import { queryOptions } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'

export const branchDiffQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['branch-diff', wsId] as const,
    queryFn: () => apiFetch<MultiFileDiff>(`/api/v0/branch-review/${wsId}/diff`),
  })

export const branchThreadsQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['branch-threads', wsId] as const,
    queryFn: () => apiFetch<ReviewThread[]>(`/api/v0/branch-review/${wsId}/threads`),
  })

export const branchDescriptionQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['branch-description', wsId] as const,
    queryFn: () => apiFetch<string>(`/api/v0/branch-review/${wsId}/description`),
  })

export const branchChatsQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['branch-chats', wsId] as const,
    queryFn: () => apiFetch<BranchReviewChat[]>(`/api/v0/branch-review/${wsId}/chats`),
  })
