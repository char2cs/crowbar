import { useMemo } from 'react'
import { GitBranch } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useShownSidebarTab } from '@/components/layout/sidebar-panel-tabs'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { openBranchReviewForWorkspace } from '@/features/panes/utils/pane-command-actions'
import { useReviewViewedById } from '@/features/git/lib/review-viewed'
import { openThreadCountsByPath } from '@/features/git/lib/review-thread-counts'
import { useSidebarStore } from '@/lib/store/sidebar'
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

  // Active workspace metadata from sidebar store (branch, parentBranch).
  const activeWs = useSidebarStore((s) => {
    if (!wsId) return null
    for (const repo of s.repos) {
      const ws = repo.workspaces.find((w) => w.id === wsId)
      if (ws) return ws
    }
    return null
  })

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
  const threads = useWorkspaceStoreById(wsId ?? '', (s) => s.branchReview.threads)
  const openThreadCounts = useMemo(() => openThreadCountsByPath(threads), [threads])

  const changedLabel = `${files.length} file${files.length !== 1 ? 's' : ''}`

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {wsId && branch && (
        <div className="mx-1.5 my-0.5 rounded-lg border border-background bg-background text-foreground shadow-xs shadow-black/10 inset-shadow-[0_1px_var(--elevated-highlight)]">
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

      {wsId && activeWs && <ReviewMergeBox wsId={wsId} />}

      {/* No workspace to show git for at all (project home, or the route/pane
          hasn't resolved one yet) — never fall through to the sections below,
          which would otherwise render `useSidebarChangedFiles`'s stale-status
          fallback: a workspace-scoped git store with no wsId to key its
          "which workspace is this?" question against just keeps whatever
          OTHER workspace's status/branch/changed-files it last fetched,
          cross-chat and cross-project. Git has no meaning without a repo,
          so it shows nothing instead of stale data. */}
      {!wsId && (
        <div className="flex flex-1 items-center justify-center px-4 text-center text-[13px] text-muted-foreground">
          No repository open
        </div>
      )}

      {wsId && (
        <>
          {/* Spec 6.3 #3 — "Changed — n files", then the changed list. */}
          <div className="ui-text-xs shrink-0 px-3.5 pt-2 pb-1 text-muted-foreground">
            Changed — {changedLabel}
          </div>
          <ScrollArea className="flex-1">
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
