import { useEffect } from 'react'
import { holdScope } from '@/core/stream/stream'
import { useResolvedHost } from '@/lib/host-resolve'
import { isHomeWorkspace } from '@/lib/workspace-scope-url'
import { useOwningChatId } from '@/lib/workspace-scope'
import { useWorkspaceFileTree } from './use-workspace-file-tree'
import { useWorkspaceGit } from './use-workspace-git'

export function useWorkspaceEffects(wsId: string, active: boolean) {
  const owningChatId = useOwningChatId(wsId)
  const homeWorkspace = isHomeWorkspace(wsId)
  const chatScopeReady = homeWorkspace || owningChatId !== null

  const hostId = useResolvedHost({ wsId })
  useEffect(() => (hostId ? holdScope(hostId, `ws:${wsId}`) : undefined), [wsId, hostId])
  useWorkspaceFileTree(wsId, active, chatScopeReady)
  useWorkspaceGit(wsId, active, owningChatId, homeWorkspace)
}
