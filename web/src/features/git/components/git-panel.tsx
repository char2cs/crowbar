import { useRouterState } from '@tanstack/react-router'
import { GitPullRequest } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { openBranchReviewForActiveWorkspace } from '@/features/panes/utils/pane-command-actions'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useGitStore } from '@/features/git/stores/git-store'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { useReviewDiff } from '@/features/git/hooks/use-review-diff'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { ChangedFilesTree } from './changed-files-tree'
import { BranchSection } from './branch-section'
import { GitHistoryList } from './git-history-list'

export function GitPanel() {
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

  // Review diff: branch-vs-parent blended files (for the changed-files tree).
  const { files } = useReviewDiff(wsId)

  // Open the unified branch-review tab and scroll to the clicked file.
  // fileKey must match the scheme used by ReviewDiffView:
  //   multiDiff.fileKeys?.[index] ?? `${diff.file_path}:${index}`
  // We look up the file index from the cached diffCache in the workspace store.
  const handleFileOpen = (filePath: string) => {
    openBranchReviewForActiveWorkspace()
    if (!wsId) return
    const store = getOrCreateWorkspaceStore(wsId)
    const diffCache = store.getState().branchReview.diffCache
    if (!diffCache) return
    const index = diffCache.files.findIndex((f) => f.file_path === filePath)
    if (index === -1) return
    const fileKey = diffCache.fileKeys?.[index] ?? `${filePath}:${index}`
    store.getState().setBranchReviewActiveFile(fileKey)
  }

  const repoPath = wsId ?? undefined

  return (
    <Tabs defaultValue="changes" className="flex flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 items-center gap-1 px-2 py-1.5">
        <TabsList variant="default" className="min-w-0 flex-1">
          <TabsTab value="changes" className="flex-1 justify-center">
            Changes
          </TabsTab>
          <TabsTab value="history" className="flex-1 justify-center">
            History
          </TabsTab>
        </TabsList>
        <Button
          variant="ghost"
          size="icon-sm"
          className="shrink-0"
          title="Open review"
          aria-label="Open review"
          onClick={() => openBranchReviewForActiveWorkspace()}
        >
          <GitPullRequest />
        </Button>
      </div>

      <TabsPanel value="changes" className="flex flex-1 flex-col overflow-hidden">
        {/* Scrollable file tree */}
        <ScrollArea className="flex-1">
          <ChangedFilesTree files={files} repoPath={repoPath} onFileOpen={handleFileOpen} />
        </ScrollArea>

        {/* Bottom pinned region: one parent-anchored section whose primary action
            reflects the branch state (commit / merge / sync). It dispatches
            git-status-changed after any mutation so the diff + status refresh. */}
        {wsId ? (
          <div className="shrink-0 border-t border-border">
            <BranchSection
              wsId={wsId}
              branch={activeWs?.branch ?? gitStatus?.branch ?? ''}
              parentBranch={activeWs?.parentBranch}
              canMergeLocally={activeWs?.canMergeLocally ?? false}
              wouldConflict={activeWs?.mergeConflicts ?? false}
              status={activeWs?.status ?? 'new'}
              ahead={gitStatus?.ahead ?? 0}
              behind={gitStatus?.behind ?? 0}
              // Working-tree status (incl. untracked) — drives the commit dialog and
              // the "N uncommitted changes" line. The changed-files tree above uses
              // the branch-vs-parent review diff, so the two can differ by untracked files.
              files={gitStatus?.files ?? []}
            />
          </div>
        ) : null}
      </TabsPanel>

      <TabsPanel value="history" className="flex flex-1 flex-col overflow-hidden">
        <GitHistoryList />
      </TabsPanel>
    </Tabs>
  )
}
