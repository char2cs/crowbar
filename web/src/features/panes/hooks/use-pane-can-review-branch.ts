import { useChatIsThread } from '@/features/panes/hooks/use-chat-is-thread'
import { usePaneById } from '@/features/workspace/stores/hooks/use-pane-store'

/** Whether a pane's chat has a branch to review: a thread, or a home chat, owns none. */
export function usePaneCanReviewBranch(paneId: string): boolean {
  return !useChatIsThread(usePaneById(paneId)?.chatId ?? null)
}
