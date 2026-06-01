import { queryOptions } from '@tanstack/react-query'
import { apiFetch } from '@/lib/api'
import type { MarkdownTurn } from '@/features/markdown-chat/types'

export const markdownChatQueryOptions = (wsId: string, stepId: string) =>
  queryOptions({
    queryKey: ['markdown-chat', wsId, stepId] as const,
    queryFn: () => apiFetch<MarkdownTurn[]>(`/api/v0/markdown-chat/${wsId}/${stepId}`),
  })
