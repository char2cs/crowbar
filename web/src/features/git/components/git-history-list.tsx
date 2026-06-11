import { ScrollArea } from '@/components/ui/scroll-area'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useGitStore } from '@/features/git/stores/git-store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { dataOf } from '@/lib/loadable'
import { formatRelativeTime } from '@/utils/date'
import { useGitDiffHandlers } from '../hooks/use-git-diff-handlers'

// commitDateLabel renders the backend's ISO commit date as a relative time
// ("2 hours ago"); an unparseable date falls back to the raw string.
export function commitDateLabel(isoDate: string): string {
  const ms = Date.parse(isoDate)
  if (Number.isNaN(ms)) return isoDate
  return formatRelativeTime(ms / 1000)
}

// nearListEnd reports whether the viewport has scrolled past 80% of the list —
// the threshold at which the next commit page is requested.
export function nearListEnd(
  scrollTop: number,
  clientHeight: number,
  scrollHeight: number,
): boolean {
  if (scrollHeight <= 0) return false
  return (scrollTop + clientHeight) / scrollHeight >= 0.8
}

export function GitHistoryList() {
  const gitData = useGitStore((s) => s.gitData)
  const commits = useGitStore((s) => s.commits)
  const isLoadingMore = useGitStore((s) => s.isLoadingMoreCommits)

  const isLoading = gitData.status === 'idle' || (gitData.status === 'loading' && !dataOf(gitData))

  const wsId = getActiveWorkspaceId() ?? ''
  // Reuse the same diff-tab plumbing the Changes panel uses — a commit row
  // click opens the commit's multi-file diff tab.
  const { handleViewCommitDiff } = useGitDiffHandlers({
    activeRepoPath: wsId || null,
    visibleGitFiles: [],
    onFileSelect: (path, isDir) => {
      if (isDir) return
      const rel = wsId && path.startsWith(`${wsId}/`) ? path.slice(wsId.length + 1) : path
      void useFileSystemStore.getState().handleFileOpen?.(rel, false)
    },
  })

  if (isLoading) {
    return (
      <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">
        Loading…
      </div>
    )
  }

  if (commits.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">
        No commits
      </div>
    )
  }

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const el = e.target as HTMLElement
    if (!nearListEnd(el.scrollTop, el.clientHeight, el.scrollHeight)) return
    const { currentRepoPath, hasMoreCommits, isLoadingMoreCommits, actions } =
      useGitStore.getState()
    if (currentRepoPath && hasMoreCommits && !isLoadingMoreCommits) {
      void actions.loadMoreCommits(currentRepoPath)
    }
  }

  return (
    <ScrollArea className="flex-1" onScrollCapture={handleScroll}>
      <div className="py-1">
        {commits.map((commit) => (
          <div
            key={commit.hash}
            role="button"
            tabIndex={0}
            aria-label={`View diff for commit ${commit.hash.slice(0, 7)}`}
            className="flex items-start gap-2 mx-1.5 my-0.5 px-2 py-1.5 hover:bg-accent rounded-md cursor-pointer"
            onClick={() => void handleViewCommitDiff(commit.hash)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                void handleViewCommitDiff(commit.hash)
              }
            }}
          >
            <span className="mt-0.5 shrink-0 font-mono text-[11px] text-muted-foreground">
              {commit.hash.slice(0, 7)}
            </span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-[13px]">{commit.message}</p>
              <p className="text-[11px] text-muted-foreground">
                {commit.author} · {commitDateLabel(commit.date)}
              </p>
            </div>
          </div>
        ))}
        {isLoadingMore && (
          <div className="py-2 text-center text-[12px] text-muted-foreground">Loading more…</div>
        )}
      </div>
    </ScrollArea>
  )
}
