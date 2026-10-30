import { useEffect } from 'react'
import { useWorkspaceScopeReady } from '@/lib/workspace-scope'
import { attachScope } from '@/features/agent/stores/chat-scopes'
import { useChatStore } from '@/features/agent/stores/chat-store'

/** Holds `wsId`'s chat lifecycle scope open while mounted. Waits for the
 *  workspace's project/repo scope: the endpoint is built from it. */
export function useChatScope(wsId: string): void {
  const ready = useWorkspaceScopeReady(wsId)
  useEffect(() => (ready ? attachScope(wsId) : undefined), [wsId, ready])
}

/** Whether `wsId`'s chat list has landed: "not in the list" is final only then. */
export function useChatListReady(wsId: string): boolean {
  return useChatStore((s) => s.scopes[wsId] === 'ready')
}
