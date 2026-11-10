import { useChatIsThread } from '@/features/panes/hooks/use-chat-is-thread'
import { usePaneById } from '@/features/workspace/stores/hooks/use-pane-store'
import { useIsHomeWorkspace } from '@/lib/workspace-scope-url'

/** Whether a pane has a branch to review: one with no workspace has nothing to open it on, and a
 *  project home (repo-less, even before its first chat), a thread or a home chat owns none. */
export function usePaneCanReviewBranch(paneId: string, wsId: string | null): boolean {
  const isThread = useChatIsThread(usePaneById(paneId)?.chatId ?? null)
  const isHome = useIsHomeWorkspace(wsId ?? '')
  return !!wsId && !isThread && !isHome
}
