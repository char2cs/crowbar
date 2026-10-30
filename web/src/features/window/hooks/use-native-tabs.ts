import { useEffect, useMemo } from 'react'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { hasUnsavedEdits, type PaneContent } from '@/features/panes/types/pane-content'
import { usePaneNewTabActions } from '@/features/tabs/hooks/use-pane-new-tab-actions'
import { usePaneCanReviewBranch } from '@/features/panes/hooks/use-pane-can-review-branch'
import { useBuffersByIds } from '@/features/workspace/stores/hooks/use-buffer-store'
import { onNativeChromeIntent, setNativeTabs, type NativeTabs } from '@/lib/native-chrome'

type NativeTab = NativeTabs['tabs'][number]

const TERMINAL_ACTION = { id: 'terminal', title: 'New Terminal', icon: 'TerminalWindow' }
const REVIEW_ACTION = { id: 'review', title: 'Review Branch', icon: 'GitPullRequest' }

function kindOf(buffer: PaneContent): Pick<NativeTab, 'kind' | 'path'> {
  switch (buffer.type) {
    case 'terminal':
      return { kind: 'terminal', path: null }
    case 'branchReview':
      return { kind: 'review', path: null }
    case 'commitDiff':
      return { kind: 'diff', path: null }
    case 'browser':
      return { kind: 'browser', path: null }
    case 'newTabPage':
      return { kind: 'newTab', path: null }
    case 'markdownPreview':
    case 'htmlPreview':
    case 'csvPreview':
      return { kind: 'preview', path: buffer.sourceFilePath }
    default:
      return { kind: 'file', path: buffer.path ?? null }
  }
}

function closeTab(paneId: string, bufferId: string) {
  const { buffers, paneActions, bufferActions } = windowPaneStore.getState()
  if (buffers.find((b) => b.id === bufferId)?.isUncloseable) return
  paneActions.removeEditorTabFromPane(paneId, bufferId)
  bufferActions.closeBuffer(bufferId)
}

/**
 * Feeds the IDE's native tab strip with `paneId`'s editor tabs and carries out what it asks:
 * show a tab, close one, or open what its + menu picked.
 */
export function useNativeTabs(paneId: string, wsId: string) {
  const tabIds = useStore(windowPaneStore, (s) => s.panes[paneId]?.editorTabIds)
  const activeId = useStore(windowPaneStore, (s) => s.panes[paneId]?.activeEditorTabId ?? null)
  const buffers = useBuffersByIds(tabIds ?? [])
  const { onNewTerminal, onReviewBranch } = usePaneNewTabActions(wsId, paneId)
  const canReview = usePaneCanReviewBranch(paneId)

  const snapshot = useMemo<NativeTabs>(
    () => ({
      tabs: buffers.map((buffer) => ({
        id: buffer.id,
        title: buffer.name,
        ...kindOf(buffer),
        active: buffer.id === activeId,
        dirty: hasUnsavedEdits(buffer),
        closable: !buffer.isUncloseable,
      })),
      newTabActions: canReview ? [TERMINAL_ACTION, REVIEW_ACTION] : [TERMINAL_ACTION],
    }),
    [buffers, activeId, canReview],
  )
  useEffect(() => {
    setNativeTabs(snapshot)
  }, [snapshot])
  useEffect(
    () => () => {
      setNativeTabs({ tabs: [], newTabActions: [] })
    },
    [],
  )

  useEffect(
    () =>
      onNativeChromeIntent((intent) => {
        if (intent.type === 'selectTab') {
          windowPaneStore.getState().paneActions.activateEditorTabInPane(paneId, intent.id)
        } else if (intent.type === 'closeTab') closeTab(paneId, intent.id)
        else if (intent.type === 'newTab' && intent.actionId === 'terminal') onNewTerminal()
        else if (intent.type === 'newTab' && intent.actionId === 'review' && canReview) {
          onReviewBranch()
        }
      }),
    [paneId, onNewTerminal, onReviewBranch, canReview],
  )
}
