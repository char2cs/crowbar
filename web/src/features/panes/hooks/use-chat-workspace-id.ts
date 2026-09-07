import { useCallback, useSyncExternalStore } from 'react'
import { subscribeWorkspaceStores } from '@/features/workspace/stores/workspace-store-registry'
import { resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'

/**
 * {@link resolveChatWorkspaceId} in the render path — the workspace `chatId`
 * belongs to, re-resolved whenever that could start (or stop) being knowable.
 *
 * A pane mounts before any store has been seeded with the chat it holds, so a
 * one-shot lookup at mount answers null and stays there for the session. The
 * subscription is registry-wide for the same reason the resolver's scan is:
 * WHICH store will turn out to hold the chat is precisely what the caller does
 * not know yet.
 *
 * Returns a plain string (or null), so a store write that leaves the answer
 * unchanged — every one of them, in the steady state — re-renders nothing.
 */
export function useChatWorkspaceId(chatId: string | null): string | null {
  const subscribe = useCallback(
    (onChange: () => void) => (chatId ? subscribeWorkspaceStores(onChange) : () => {}),
    [chatId],
  )
  const snapshot = useCallback(() => (chatId ? resolveChatWorkspaceId(chatId) : null), [chatId])
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}
