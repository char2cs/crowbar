import { useEffect } from 'react'
import { listChatFolders } from '@/features/agent/api/agent-api'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'

/**
 * Seed this workspace's chat FOLDERS into its store.
 *
 * Its own read rather than a field on the chat seed: folders are a second
 * aggregate with their own route, and the chat list is refetched on frames
 * (every turn a title lands, every reconnect) that say nothing about folders.
 *
 * One GET per mounted workspace, and nothing after it — no timer, no poll. The
 * arrangement changes only when this client changes it, and every one of those
 * writes returns the rows it moved (chat-tree-commit.ts), so there is nothing
 * for a poll to discover. A folder created in another window arrives on the next
 * mount, which is the same deal the sidebar's own folders make.
 */
export function useAgentChatFolders(wsId: string): void {
  useEffect(() => {
    let cancelled = false
    listChatFolders(wsId)
      .then((folders) => {
        // A workspace switch unmounted this before the answer came back; writing
        // it now would put one workspace's folders in another's store.
        if (cancelled) return
        getOrCreateWorkspaceStore(wsId).getState().seedAgentChatFolders(folders)
      })
      .catch(() => {
        // Non-fatal: with no folders the tree is every chat at the root, which
        // is exactly what the panel drew before folders existed.
      })
    return () => {
      cancelled = true
    }
  }, [wsId])
}
