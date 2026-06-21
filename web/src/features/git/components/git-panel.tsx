import { useRouterState } from '@tanstack/react-router'
import { GitPullRequest } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { openBranchReviewForActiveWorkspace } from '@/features/panes/utils/pane-command-actions'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useGitStore } from '@/features/git/stores/git-store'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { useGitDiffHandlers } from '@/features/git/hooks/use-git-diff-handlers'
import { useReviewDiff } from '@/features/git/hooks/use-review-diff'
import type { GitFile } from '@/features/git/types/git-types'
import { ChangedFilesTree } from './changed-files-tree'
import GitCommitPanel from './git-commit-panel'
import { MergeSection } from './merge-section'
import { GitHistoryList } from './git-history-list'

export function GitPanel() {
  // I1 fix: derive wsId reactively from the route so a workspace switch without
  // remount keeps all API calls and store lookups pointed at the active workspace.
  // Pattern mirrors sidebar-carousel.tsx and context-pill.tsx.
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const wsId = parseWorkspaceScopeFromPath(pathname)?.wsId ?? null

  // Narrow selectors: pull only the fields we need from each store.
  const gitStatus = useGitStore((s) => s.gitStatus)
  const staged = gitStatus?.files.filter((f) => f.staged) ?? []

  // Active workspace metadata from sidebar store (canMergeLocally, parentBranch, status).
  const activeWs = useSidebarStore((s) => {
    if (!wsId) return null
    for (const repo of s.repos) {
      const ws = repo.workspaces.find((w) => w.id === wsId)
      if (ws) return ws
    }
    return null
  })

  // Review diff: branch-vs-parent blended files + uncommitted count.
  const { files, uncommittedCount } = useReviewDiff(wsId)

  // I2 fix: build a merged visibleGitFiles that includes committed (branch-only)
  // files from the review diff alongside the working-tree files from gitStatus.
  // Without this, clicking a committed file in the blended tree calls getFileDiff
  // which returns empty (no working-tree changes) and falls back to opening the
  // file as a plain editor buffer instead of a diff.
  //
  // Strategy: adapt each review GitDiff into the GitFile shape expected by
  // useGitDiffHandlers, then union with gitStatus.files (gitStatus entries take
  // priority so staged/unstaged state is accurate for working-tree files).
  const reviewAsGitFiles: GitFile[] = files.map((diff) => ({
    path: diff.file_path,
    status: diff.is_new
      ? 'added'
      : diff.is_deleted
        ? 'deleted'
        : diff.is_renamed
          ? 'renamed'
          : 'modified',
    staged: false,
  }))
  const workingTreePaths = new Set((gitStatus?.files ?? []).map((f) => f.path))
  const blendedGitFiles: GitFile[] = [
    ...(gitStatus?.files ?? []),
    ...reviewAsGitFiles.filter((f) => !workingTreePaths.has(f.path)),
  ]

  // Diff interaction handlers (same pattern as old GitChangesPanel).
  const { handleViewFileDiff } = useGitDiffHandlers({
    activeRepoPath: wsId ?? null,
    visibleGitFiles: blendedGitFiles,
  })

  // Handler adapts the (filePath: string) signature that ChangedFilesTree expects.
  const handleFileOpen = (filePath: string) => {
    void handleViewFileDiff(filePath, false)
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

        {/* Pinned commit bar */}
        {/* I3 fix: dispatch git-status-changed on commit/push/pull so useReviewDiff
            re-fetches the branch diff immediately after the operation completes.
            GitCommitPanel calls onCommitSuccess after a successful commit or
            remote action but does NOT dispatch the event itself. */}
        <div className="shrink-0 border-t border-border">
          <GitCommitPanel
            stagedFilesCount={staged.length}
            repoPath={repoPath}
            ahead={gitStatus?.ahead ?? 0}
            behind={gitStatus?.behind ?? 0}
            onCommitSuccess={() => window.dispatchEvent(new Event('git-status-changed'))}
          />
        </div>

        {/* Merge section — only rendered when the ws has merge eligibility data */}
        {wsId && activeWs?.parentBranch && (
          <div className="shrink-0 border-t border-border p-3">
            <MergeSection
              wsId={wsId}
              parentBranch={activeWs.parentBranch}
              canMergeLocally={activeWs.canMergeLocally ?? false}
              hasUncommitted={uncommittedCount > 0}
              status={activeWs.status ?? 'new'}
            />
          </div>
        )}
      </TabsPanel>

      <TabsPanel value="history" className="flex flex-1 flex-col overflow-hidden">
        <GitHistoryList />
      </TabsPanel>
    </Tabs>
  )
}
