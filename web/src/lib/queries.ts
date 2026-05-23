import { queryOptions } from '@tanstack/react-query'
import { fetchWorkspace, fetchFlows, fetchConversation } from './api'

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
