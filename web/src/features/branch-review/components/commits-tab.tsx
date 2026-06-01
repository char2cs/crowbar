import { getMockCommitHistory } from '@/lib/mock/git-data'
import { formatRelativeDate } from '@/utils/date'

interface CommitsTabProps {
  repoPath: string
}

export function CommitsTab({ repoPath }: CommitsTabProps) {
  const commits = getMockCommitHistory(repoPath)

  return (
    <div className="flex flex-col p-5">
      <p className="mb-3 text-xs font-medium uppercase tracking-wide text-muted-foreground">Commit history</p>
      <div className="flex flex-col">
        {commits.map(commit => (
          <div
            key={commit.hash}
            className="flex items-center gap-3 rounded-lg px-2 py-2 transition-colors hover:bg-accent"
          >
            <span className="w-12 shrink-0 font-mono text-[10px] text-muted-foreground/60">
              {commit.hash.slice(0, 7)}
            </span>
            <span className="flex-1 truncate text-sm text-foreground">{commit.message}</span>
            <span className="shrink-0 text-xs text-muted-foreground/50">
              {formatRelativeDate(commit.date)}
            </span>
          </div>
        ))}
      </div>
    </div>
  )
}
