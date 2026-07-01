import { useEffect, useRef } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useGitStore } from '@/features/git/stores/git-store'
import { getActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
import { dataOf } from '@/lib/loadable'
import { formatRelativeTime } from '@/utils/date'
import type { GitCommit } from '../types/git-types'
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

  return (
    <GitHistoryListBody
      commits={commits}
      isLoadingMore={isLoadingMore}
      onViewCommitDiff={handleViewCommitDiff}
    />
  )
}

/**
 * The virtualized commit list. Split out from GitHistoryList so the virtualizer
 * hooks only run once we know there are commits to show (the early-return
 * loading/empty branches above keep the hook order stable here).
 */
function GitHistoryListBody({
  commits,
  isLoadingMore,
  onViewCommitDiff,
}: {
  commits: GitCommit[]
  isLoadingMore: boolean
  onViewCommitDiff: (hash: string) => void | Promise<void>
}) {
  // Plain scroll container (mirrors git-diff-editor-stack) so the virtualizer
  // can own the scroll element directly. Only the rows near the viewport mount —
  // the full history can grow unbounded via infinite scroll without piling
  // thousands of DOM nodes.
  const scrollRef = useRef<HTMLDivElement>(null)
  const rowVirtualizer = useVirtualizer({
    count: commits.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => 48,
    overscan: 12,
    measureElement: (el) => el.getBoundingClientRect().height,
  })

  // Drive the next-page fetch off the virtualizer range rather than a scroll
  // handler: once the rendered window reaches the last few commits, request more.
  const virtualItems = rowVirtualizer.getVirtualItems()
  const lastIndex = virtualItems.length > 0 ? virtualItems[virtualItems.length - 1].index : -1
  useEffect(() => {
    if (lastIndex < commits.length - 5) return
    const { currentRepoPath, hasMoreCommits, isLoadingMoreCommits, actions } =
      useGitStore.getState()
    if (currentRepoPath && hasMoreCommits && !isLoadingMoreCommits) {
      void actions.loadMoreCommits(currentRepoPath)
    }
  }, [lastIndex, commits.length])

  return (
    <div
      ref={scrollRef}
      className="min-h-0 flex-1 overflow-auto"
      style={{ overflowAnchor: 'none' }}
    >
      <div className="py-1">
        <div className="relative w-full" style={{ height: `${rowVirtualizer.getTotalSize()}px` }}>
          {virtualItems.map((virtualItem) => {
            const commit = commits[virtualItem.index]
            if (!commit) return null
            return (
              <div
                key={commit.hash}
                ref={rowVirtualizer.measureElement}
                data-index={virtualItem.index}
                style={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  transform: `translateY(${virtualItem.start}px)`,
                }}
              >
                <div
                  role="button"
                  tabIndex={0}
                  aria-label={`View diff for commit ${commit.hash.slice(0, 7)}`}
                  className="flex items-start gap-2 mx-1.5 my-0.5 px-2 py-1.5 hover:bg-accent rounded-md cursor-pointer"
                  onClick={() => void onViewCommitDiff(commit.hash)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      void onViewCommitDiff(commit.hash)
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
              </div>
            )
          })}
        </div>
        {isLoadingMore && (
          <div className="py-2 text-center text-[12px] text-muted-foreground">Loading more…</div>
        )}
      </div>
    </div>
  )
}
