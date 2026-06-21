import { useState } from 'react'
import { GitPullRequest } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsList, TabsTab, TabsPanel } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { openBranchReviewForActiveWorkspace } from '@/features/panes/utils/pane-command-actions'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useGitStore } from '@/features/git/stores/git-store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { useGitDiffHandlers } from '@/features/git/hooks/use-git-diff-handlers'
import { useReviewDiff } from '@/features/git/hooks/use-review-diff'
import { ChangedFilesTree } from './changed-files-tree'
import GitCommitPanel from './git-commit-panel'
import { MergeSection } from './merge-section'
import { GitHistoryList } from './git-history-list'

export function GitPanel() {
  // Resolve the active workspace id from the registry (same pattern as the
  // old GitChangesPanel). We capture it in state so it's stable across renders.
  const [wsId] = useState<string | null>(() => getActiveWorkspaceId())

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

  // Diff interaction handlers (same pattern as old GitChangesPanel).
  const { handleViewFileDiff } = useGitDiffHandlers({
    activeRepoPath: wsId ?? null,
    // Cast to satisfy the GitFile[] type — we only need the path for diff lookup.
    visibleGitFiles: gitStatus?.files ?? [],
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
        <div className="shrink-0 border-t border-border">
          <GitCommitPanel
            stagedFilesCount={staged.length}
            repoPath={repoPath}
            ahead={gitStatus?.ahead ?? 0}
            behind={gitStatus?.behind ?? 0}
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
