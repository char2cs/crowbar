import { ScrollArea } from '@/components/ui/scroll-area'
import { useGitStore } from '@/features/git/stores/git-store'
import { dataOf } from '@/lib/loadable'

export function GitHistoryList() {
  const gitData = useGitStore(s => s.gitData)
  const commits = useGitStore(s => s.commits)

  const isLoading = gitData.status === 'idle' || (gitData.status === 'loading' && !dataOf(gitData))

  if (isLoading) {
    return <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">Loading…</div>
  }

  if (commits.length === 0) {
    return <div className="flex flex-1 items-center justify-center text-[13px] text-muted-foreground">No commits</div>
  }

  return (
    <ScrollArea className="flex-1">
      <div className="py-1">
        {commits.map(commit => (
          <div key={commit.hash} className="flex items-start gap-2 mx-1.5 my-0.5 px-2 py-1.5 hover:bg-accent rounded-md cursor-pointer">
            <span className="mt-0.5 shrink-0 font-mono text-[11px] text-muted-foreground">{commit.hash.slice(0, 7)}</span>
            <div className="min-w-0 flex-1">
              <p className="truncate text-[13px]">{commit.message}</p>
              <p className="text-[11px] text-muted-foreground">{commit.author} · {commit.date}</p>
            </div>
          </div>
        ))}
      </div>
    </ScrollArea>
  )
}
