import { useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { GitPullRequest, GitBranch, ClockCounterClockwise, CaretRight } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/utils/cn'
import { openBranchReviewForActiveWorkspace } from '@/features/panes/utils/pane-command-actions'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useGitStore } from '@/features/git/stores/git-store'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { useSidebarChangedFiles } from '@/features/git/hooks/use-sidebar-changed-files'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { ChangedFilesTree } from './changed-files-tree'
import { BranchSection } from './branch-section'
import { GitHistoryList } from './git-history-list'

export function GitPanel() {
  // History (the commit log) has no place in spec 6.3's flat commit-box →
  // review-row → changed-list shape — it isn't described there at all, and it
  // isn't redundant with anything else in the app (branch review shows the
  // current diff, not the log). It's real, load-bearing, virtualized/infinite-
  // scroll functionality, so it stays — folded below the three required
  // sections behind a disclosure rather than a competing top-level tab.
  const [historyOpen, setHistoryOpen] = useState(false)
  // I1 fix: derive wsId reactively from the route so a workspace switch without
  // remount keeps all API calls and store lookups pointed at the active workspace.
  // Pattern mirrors sidebar-carousel.tsx and context-pill.tsx.
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const wsId = parseWorkspaceScopeFromPath(pathname)?.wsId ?? null

  // Narrow selectors: pull only the fields we need from each store.
  const gitStatus = useGitStore((s) => s.gitStatus)

  // Active workspace metadata from sidebar store (canMergeLocally, parentBranch, status).
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

  // Open the unified branch-review tab and scroll to the clicked file.
  //
  // By PATH. This used to resolve the file's INDEX against a whole-diff cache
  // in the store and pass a composite `path:index` key — and when the review
  // surface stopped loading that cache (it reads the files summary and fetches
  // patches per file now), the lookup returned nothing and every click in this
  // tree silently opened the tab at the top. The surface addresses files by
  // path, so ask for one.
  const handleFileOpen = (filePath: string) => {
    openBranchReviewForActiveWorkspace()
    if (!wsId) return
    getOrCreateWorkspaceStore(wsId).getState().revealBranchReviewFile(filePath)
  }

  const repoPath = wsId ?? undefined
  const branch = activeWs?.branch ?? gitStatus?.branch ?? ''
  const parentBranch = activeWs?.parentBranch

  // Single source of truth for "how many files changed" — the review row's
  // count and the "Changed — n files" heading below both read this, not two
  // separately-computed definitions of "changed".
  const changedCount = files.length
  const changedLabel = `${changedCount} file${changedCount !== 1 ? 's' : ''}`

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
          <BranchSection
            wsId={wsId}
            parentBranch={parentBranch}
            canMergeLocally={activeWs?.canMergeLocally ?? false}
            status={activeWs?.status ?? 'new'}
            ahead={gitStatus?.ahead ?? 0}
            behind={gitStatus?.behind ?? 0}
            files={gitStatus?.files ?? []}
          />
        </div>
      )}

      {/* Spec 6.3 #2 — "Review this branch", carrying the changed-file count,
          opens the branch review in the editor view. */}
      <button
        type="button"
        className="mx-1.5 flex shrink-0 items-center gap-2 rounded-md px-2 py-1.5 text-[13px] text-foreground hover:bg-accent"
        onClick={() => openBranchReviewForActiveWorkspace()}
      >
        <GitPullRequest className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate text-left">Review this branch</span>
        <span className="shrink-0 text-muted-foreground">{changedLabel}</span>
      </button>

      {/* Spec 6.3 #3 — "Changed — n files", then the changed list. */}
      <div className="ui-text-xs shrink-0 px-3.5 pt-2 pb-1 text-muted-foreground">
        Changed — {changedLabel}
      </div>
      <ScrollArea className="flex-1">
        <ChangedFilesTree files={files} repoPath={repoPath} onFileOpen={handleFileOpen} />
      </ScrollArea>

      {/* History (the commit log) below the required three sections, folded —
          see the note above. */}
      <div className="shrink-0 border-t border-border">
        <button
          type="button"
          className="flex w-full items-center gap-2 px-2 py-1.5 text-[13px] text-muted-foreground hover:bg-accent"
          onClick={() => setHistoryOpen((open) => !open)}
          aria-expanded={historyOpen}
        >
          <CaretRight
            className={cn('size-3 shrink-0 transition-transform', historyOpen && 'rotate-90')}
          />
          <ClockCounterClockwise className="size-3.5 shrink-0" />
          <span className="flex-1 text-left">History</span>
        </button>
        {historyOpen && (
          <div className="flex h-64 flex-col overflow-hidden border-t border-border">
            <GitHistoryList />
          </div>
        )}
      </div>
    </div>
  )
}
