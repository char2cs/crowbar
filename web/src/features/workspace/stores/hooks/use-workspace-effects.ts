import { useSyncExternalStore } from 'react'
import { isHomeWorkspace } from '@/lib/workspace-scope-url'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import { useWorkspaceThreadsStream } from './use-workspace-threads-stream'
import { useWorkspaceFileTree } from './use-workspace-file-tree'
import { useWorkspaceGit } from './use-workspace-git'

/**
 * `getOwningChatId(wsId)` as React state: the sidebar records it asynchronously,
 * and the chat-scoped routes throw without it, so effects must re-run once it lands.
 */
function useOwningChatId(wsId: string): string | null {
  return useSyncExternalStore(
    (onChange) => subscribeToWorkspaceScope(wsId, onChange),
    () => getOwningChatId(wsId),
  )
}

/**
 * A workspace's data feeds: its file tree, git status and review threads, each
 * written to the workspace's own store. Runs for every retained workspace so a
 * switch only changes which store the UI reads; `active` only gates the first
 * tree/git load.
 */
export function useWorkspaceEffects(wsId: string, active: boolean) {
  const owningChatId = useOwningChatId(wsId)
  const homeWorkspace = isHomeWorkspace(wsId)
  // A non-home workspace's routes need the owning chat id; until the sidebar
  // has recorded one there is nothing a fetch or subscription could address.
  const chatScopeReady = homeWorkspace || owningChatId !== null

  useWorkspaceThreadsStream(wsId)
  useWorkspaceFileTree(wsId, active, chatScopeReady)
  useWorkspaceGit(wsId, active, owningChatId, homeWorkspace)
}
