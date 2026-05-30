import { queryOptions } from '@tanstack/react-query'
import { fetchWorkspace, fetchFlows, fetchConversation, apiFetch } from './api'
import type { GitStatus, Commit, Branch } from '@/lib/mock/git-data'
import type { FileNode } from '@/lib/mock/files'

export const workspaceQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['workspace', wsId],
    queryFn: () => fetchWorkspace(wsId),
  })

export const flowsQueryOptions = () =>
  queryOptions({
    queryKey: ['flows'],
    queryFn: fetchFlows,
  })

export const conversationQueryOptions = (wsId: string, step: string) =>
  queryOptions({
    queryKey: ['conversation', wsId, step],
    queryFn: () => fetchConversation(wsId, step),
  })

export const fileTreeQueryOptions = (rootPath: string) =>
  queryOptions({
    queryKey: ['file-tree', rootPath] as const,
    queryFn: () => apiFetch<FileNode>(`/api/v0/fs/tree?root=${encodeURIComponent(rootPath)}`),
  })

export const gitStatusQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-status', repoPath] as const,
    queryFn: () => apiFetch<GitStatus>(`/api/v0/git/status?repo=${encodeURIComponent(repoPath)}`),
  })

export const gitHistoryQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-history', repoPath] as const,
    queryFn: () => apiFetch<Commit[]>(`/api/v0/git/log?repo=${encodeURIComponent(repoPath)}`),
  })

export const gitBranchesQueryOptions = (repoPath: string) =>
  queryOptions({
    queryKey: ['git-branches', repoPath] as const,
    queryFn: () => apiFetch<Branch[]>(`/api/v0/git/branches?repo=${encodeURIComponent(repoPath)}`),
  })
