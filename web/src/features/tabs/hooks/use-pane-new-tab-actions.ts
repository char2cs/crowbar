import { useMemo } from 'react'
import {
  ensurePaneChatThenOpen,
  openBranchReviewForWorkspace,
  openNewTabPage,
} from '@/features/panes/utils/pane-command-actions'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import { browserHomepage } from '@/features/browser/lib/homepage'
import { useSettingsStore } from '@/features/settings/store'

/**
 * What a New Tab page and an empty editor area offer: a blank file, a terminal
 * or a browser in this pane, plus the branch review — and the `+` that opens a
 * New Tab page. One implementation so the callers can never differ. All but
 * the review go through `ensurePaneChatThenOpen`, so they do nothing without
 * the pane's workspace. On a New Tab page (`replaceTabId`) the choice takes
 * that tab's place.
 */
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
          openContent(
            {
              type: 'editor',
              path: 'untitled:Untitled',
              name: 'Untitled',
              content: '',
              isVirtual: true,
              workspaceId: tabWsId,
            },
            into(target),
          )
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
