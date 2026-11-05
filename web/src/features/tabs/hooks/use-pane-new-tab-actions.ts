import { useMemo } from 'react'
import {
  ensurePaneChatThenOpen,
  openBranchReviewForWorkspace,
  openNewTabPage,
} from '@/features/panes/utils/pane-command-actions'
import { openUntitledDocument } from '@/features/editor/documents/open-document'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import { browserHomepage } from '@/features/browser/lib/homepage'
import { useSettingsStore } from '@/features/settings/store'

/** The one set of actions a New Tab page and an empty editor area offer, so they can't differ.
 *  All but the review need the pane's workspace (`ensurePaneChatThenOpen`); with `replaceTabId`
 *  the choice takes that New Tab page's place. */
export function usePaneNewTabActions(
  wsId: string | null | undefined,
  paneId: string,
  replaceTabId?: string,
) {
  const { openContent } = useBufferActions()
  return useMemo(() => {
    const into = (target: string) =>
      replaceTabId ? { paneId: target, replaceTabId } : { paneId: target }
    return {
      onNewTab: () => openNewTabPage(wsId, paneId),
      onNewFile: () => {
        if (!wsId) return
        ensurePaneChatThenOpen(wsId, paneId, (target, tabWsId) => {
          void openUntitledDocument(tabWsId, target, replaceTabId)
        })
      },
      onNewTerminal: () => {
        if (!wsId) return
        ensurePaneChatThenOpen(wsId, paneId, (target, tabWsId) => {
          openContent({ type: 'terminal', workspaceId: tabWsId }, into(target))
        })
      },
      onNewBrowser: () => {
        if (!wsId) return
        ensurePaneChatThenOpen(wsId, paneId, (target, tabWsId) => {
          openContent(
            {
              type: 'browser',
              url: browserHomepage(useSettingsStore.getState().settings),
              workspaceId: tabWsId,
            },
            into(target),
          )
        })
      },
      // The header's Branch Review shortcut opens it the same way.
      onReviewBranch: () => {
        openBranchReviewForWorkspace(wsId, paneId, replaceTabId)
      },
    }
  }, [wsId, paneId, replaceTabId, openContent])
}
