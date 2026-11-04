import { useEffect } from 'react'
import { holdScope } from '@/core/stream/stream'
import { useResolvedHost } from '@/lib/host-resolve'
import { isHomeWorkspace } from '@/lib/workspace-scope-url'
import { useOwningChatId } from '@/lib/workspace-scope'
import { useWorkspaceFileTree } from './use-workspace-file-tree'
import { useWorkspaceGit } from './use-workspace-git'

/**
 * A workspace's data feeds: its file tree and git status, written to the
 * workspace's own store, and its ws: scope (review threads into the entity
 * store). Runs for every retained workspace so a switch only
 * changes which store the UI reads; `active` only gates the first tree/git load.
 */
export function useWorkspaceEffects(wsId: string, active: boolean) {
  const owningChatId = useOwningChatId(wsId)
  const homeWorkspace = isHomeWorkspace(wsId)
  // A non-home workspace's routes need the owning chat id; until the sidebar
  // has recorded one there is nothing a fetch or subscription could address.
  const chatScopeReady = homeWorkspace || owningChatId !== null

  const hostId = useResolvedHost({ wsId })
  useEffect(() => (hostId ? holdScope(hostId, `ws:${wsId}`) : undefined), [wsId, hostId])
  useWorkspaceFileTree(wsId, active, chatScopeReady)
  useWorkspaceGit(wsId, active, owningChatId, homeWorkspace)
}
