import { useMemo } from 'react'
import { GitBranch } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { SELECTED_PILL } from '@/components/ui/selected-pill'
import { cn } from '@/lib/utils'
import { useShownSidebarTab } from '@/components/layout/sidebar-panel-tabs'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { openBranchReviewForWorkspace } from '@/features/panes/utils/pane-command-actions'
import { selectWorkspace, useSelect } from '@/core/entities/select'
import { useReviewViewedById } from '@/features/git/lib/review-viewed'
import { useReviewThreads } from '@/features/git/hooks/use-review-threads'
import { openThreadCountsByPath } from '@/features/git/lib/review-thread-counts'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import {
  hasRepoWorkspace,
  useFocusedWorkspaceContextStore,
} from '@/features/window/stores/focused-workspace-context-store'
import { useSidebarChangedFiles } from '@/features/git/hooks/use-sidebar-changed-files'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { ChangedFilesTree } from './changed-files-tree'
import { ReviewMergeBox } from './review-merge-box'

export function GitPanel() {
  // Scoped to the focused workspace context, never the route: a split can focus
  // a branch chat while the route names project home. Null for non-repo scopes.
  const wsId = useFocusedWorkspaceContextStore((s) => (hasRepoWorkspace(s) ? s.workspaceId : null))

  // Narrow selectors: pull only the fields we need from each store.
  const gitStatus = useWorkspaceStoreById(wsId ?? '', (s) => s.gitStatus)
  const gitError = useWorkspaceStoreById(wsId ?? '', (s) => s.gitStatusError)

  // Active workspace metadata from the entity store (branch, parentBranch).
  const activeWs = useSelect((s) => (wsId ? (selectWorkspace(s, wsId) ?? null) : null))

  // Changed-files tree source: the full branch-vs-parent diff only while the
  // Branch Review pane is open, otherwise the cheap working-tree status (P2b).
  const { files } = useSidebarChangedFiles(wsId)

  // By PATH, because the review surface addresses files by path.
  const handleFileOpen = (filePath: string) => {
    if (!wsId) return
    const id = openBranchReviewForWorkspace(wsId)
    if (id) windowPaneStore.getState().bufferActions.setBranchReviewView(id, 'files')
    getWorkspaceStore(wsId)?.getState().revealBranchReviewFile(filePath)
  }

  const repoPath = wsId ?? undefined
  const branch = activeWs?.branch ?? gitStatus?.branch ?? ''
  const parentBranch = activeWs?.parentBranch

  // The panel is mounted on every sidebar mode; the viewed map is read only
  // once the tree is on screen with files to mark.
  const shown = useShownSidebarTab()
  const { viewed, setFilesViewed } = useReviewViewedById(
    wsId ?? '',
    shown === 'git' && files.length > 0,
  )
  const threads = useReviewThreads(wsId ?? '')
  const openThreadCounts = useMemo(() => openThreadCountsByPath(threads), [threads])

  const changedLabel = `${files.length} file${files.length !== 1 ? 's' : ''}`

  // Scrolls as a whole: the merge box grows with its reason and must not be
  // clipped by the carousel card's height.
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      {wsId && branch && (
        <div className={cn('mx-1.5 my-0.5 text-foreground', SELECTED_PILL)}>
          <div className="flex select-none items-center gap-2 h-9 px-2 text-[13px] font-medium">
            <GitBranch className="size-3.5 shrink-0 text-muted-foreground" />
            <span className="min-w-0 flex-1 truncate font-mono">{branch}</span>
            {parentBranch && (
              <>
                <span className="shrink-0 text-muted-foreground">→</span>
                <span className="shrink-0 font-mono text-muted-foreground">{parentBranch}</span>
              </>
            )}
          </div>
        </div>
      )}

      {wsId && gitError && (
        <div role="alert" className="mx-3 my-2 flex min-w-0 flex-col gap-1">
          <p className="ui-text-sm font-medium">Git can't read this workspace</p>
          <p className="ui-text-xs break-words text-muted-foreground">{gitError}</p>
        </div>
      )}

      {wsId && activeWs && !gitError && <ReviewMergeBox wsId={wsId} />}

      {/* No workspace resolved: show nothing, since the git store would otherwise keep
          another workspace's status and changed files. */}
      {!wsId && (
        <div className="flex flex-1 items-center justify-center px-4 text-center text-[13px] text-muted-foreground">
          No repository open
        </div>
      )}

      {wsId && !gitError && (
        <>
          {/* Spec 6.3 #3 — "Changed — n files", then the changed list. */}
          <div className="ui-text-xs shrink-0 px-3.5 pt-2 pb-1 text-muted-foreground">
            Changed — {changedLabel}
          </div>
          <ScrollArea className="min-h-24 flex-1">
            <ChangedFilesTree
              files={files}
              repoPath={repoPath}
              onFileOpen={handleFileOpen}
              viewed={viewed}
              onSetViewed={setFilesViewed}
              openThreadCounts={openThreadCounts}
            />
          </ScrollArea>
        </>
      )}
    </div>
  )
}
