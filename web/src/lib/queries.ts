import { queryOptions } from '@tanstack/react-query'
import { fetchWorkspace, fetchConversation, apiFetch } from './api'
import type { GitStatus, Commit, Branch } from '@/lib/mock/git-data'
import type { FileNode } from '@/lib/mock/files'

export const workspaceQueryOptions = (wsId: string) =>
  queryOptions({
    queryKey: ['workspace', wsId],
    queryFn: () => fetchWorkspace(wsId),
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

export const fileContentQueryOptions = (path: string) =>
  queryOptions({
    queryKey: ['file-content', path] as const,
    queryFn: () => apiFetch<string>(`/api/v0/fs/file?path=${encodeURIComponent(path)}`),
    staleTime: 5 * 60 * 1000,
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

export const workspacesQueryOptions = () =>
  queryOptions({
    queryKey: ['workspaces'] as const,
    queryFn: () => apiFetch<import('@/lib/store/sidebar').Repo[]>('/api/v0/workspaces'),
  })

export const projectsQueryOptions = () =>
  queryOptions({
    queryKey: ['projects'] as const,
    queryFn: () => apiFetch<import('@/lib/types').Project[]>('/api/v0/projects'),
  })
