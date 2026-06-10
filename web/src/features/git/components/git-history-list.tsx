import { ScrollArea } from '@/components/ui/scroll-area'
import { useGitStore } from '@/features/git/stores/git-store'
import { dataOf } from '@/lib/loadable'
import { formatRelativeTime } from '@/utils/date'

// commitDateLabel renders the backend's ISO commit date as a relative time
// ("2 hours ago"); an unparseable date falls back to the raw string.
export function commitDateLabel(
  isoDate: string,
): string {
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
  const gitData = useGitStore(s => s.gitData)
  const commits = useGitStore(s => s.commits)
  const isLoadingMore = useGitStore(s => s.isLoadingMoreCommits)

  const isLoading = gitData.status === 'idle' || (gitData.status === 'loading' && !dataOf(gitData))

  if (isLoading) {
    return <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">Loading…</div>
  }

  if (commits.length === 0) {
    return <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">No commits</div>
  }

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    const el = e.target as HTMLElement
    if (!nearListEnd(el.scrollTop, el.clientHeight, el.scrollHeight)) return
    const { currentRepoPath, hasMoreCommits, isLoadingMoreCommits, actions } = useGitStore.getState()
    if (currentRepoPath && hasMoreCommits && !isLoadingMoreCommits) {
      void actions.loadMoreCommits(currentRepoPath)
    }
  }

  return (
    <ScrollArea className="flex-1" onScrollCapture={handleScroll}>
      <div className="py-1">
        {commits.map(commit => (
          <div key={commit.hash} className="flex items-start gap-2 mx-1.5 my-0.5 px-2 py-1.5 hover:bg-accent rounded-md cursor-pointer">
            <span className="mt-0.5 shrink-0 font-mono text-[11px] text-muted-foreground">{commit.hash.slice(0, 7)}</span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-[13px]">{commit.message}</p>
              <p className="text-[11px] text-muted-foreground">{commit.author} · {commitDateLabel(commit.date)}</p>
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
