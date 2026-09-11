import { useMemo } from 'react'
import { useStore } from 'zustand'
import { BOTTOM_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { usePaneActions } from '@/features/workspace/stores/hooks/use-pane-store'
import { panesInView, viewIdOf } from '@/features/panes/lib/pane-views'
import { useAnyChatWorking } from '@/features/panes/hooks/use-any-chat-working'
import type { PaneGroup } from '@/features/panes/types/pane'

interface PaneViewCloseControl {
  isBottomPane: boolean
  /** False while any chat in the VIEW (every pane sharing `pane`'s viewId,
   *  not just this one) is working — same rule Recents' own × applies. */
  canClose: boolean
  onCloseView: () => void
}

/**
 * Whether — and how — a pane's own row can close the whole VIEW it belongs
 * to (spec §5.4). Extracted from `tab-bar.tsx` so `ChatOnlyPaneHeader` (the
 * chat-only redesign's stand-in for TabBar when a pane holds a chat and no
 * editor tabs) can offer the identical close control without re-deriving
 * "is the view working" on its own.
 */
export function usePaneViewCloseControl(pane: PaneGroup | null): PaneViewCloseControl {
  const { closeView } = usePaneActions()
  const isBottomPane = pane?.id === BOTTOM_PANE_ID
  // Selected as a stable joined-id STRING, not the array `panesInView` returns —
  // that call mints a fresh array every read, which would re-render on every
  // unrelated pane-store write.
  const viewChatIdsKey = useStore(windowPaneStore, (s) => {
    if (!pane) return ''
    return panesInView(s.panes, pane.id)
      .map((p) => p.chatId)
      .filter(Boolean)
      .join(',')
  })
  const viewChatIds = useMemo(
    () => (viewChatIdsKey ? viewChatIdsKey.split(',') : []),
    [viewChatIdsKey],
  )
  const isViewWorking = useAnyChatWorking(viewChatIds)

  return {
    isBottomPane,
    canClose: !isViewWorking,
    onCloseView: () => {
      if (pane) closeView(viewIdOf(pane))
    },
  }
}
