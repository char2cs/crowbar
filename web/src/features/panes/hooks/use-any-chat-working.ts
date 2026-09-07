import { useCallback, useSyncExternalStore } from 'react'
import {
  isChatWorking,
  subscribeWorkspaceStores,
} from '@/features/workspace/stores/workspace-store-registry'

/**
 * Whether ANY of `chatIds` is currently mid-turn — the cross-workspace form of a
 * single chat's `agentChats.working[chatId]` read, for a control that acts on a
 * whole VIEW rather than one chat in one already-known store (a view's panes can
 * each belong to a different workspace since cross-workspace splits shipped).
 *
 * `subscribeWorkspaceStores` re-fires on every registered store's own writes, not
 * just registry membership changes, so this stays live as any member's turn
 * starts or ends — the same "still running" signal Recents' own `canClose` reads
 * (`recents-entries.ts`'s `resolveState`), just answered for an arbitrary chat set
 * instead of one entry's own `chatIds`.
 *
 * `key` re-subscribes only when the SET actually changes content, not on every
 * render's fresh array identity — `panesInView` returns a new array each call.
 */
export function useAnyChatWorking(chatIds: readonly string[]): boolean {
  const key = chatIds.join(',')
  const subscribe = useCallback((onChange: () => void) => subscribeWorkspaceStores(onChange), [])
  // eslint-disable-next-line react-hooks/exhaustive-deps -- `key` is chatIds' content identity
  const snapshot = useCallback(() => chatIds.some((id) => isChatWorking(id)), [key])
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}
