import { ArrowRight } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'

interface BranchReviewHeaderProps {
  title: string
  branch?: string
  baseBranch?: string
  /** Branch totals; absent until the files summary has loaded. */
  stats?: { files: number; additions: number; deletions: number }
}

function BranchPill({ name, keep }: { name: string; keep?: boolean }) {
  return (
    <span
      className={cn(
        'ui-text-sm min-w-0 truncate rounded-md bg-muted px-2 py-0.5 font-mono text-foreground',
        keep && 'max-w-[50%] shrink-0',
      )}
      title={name}
    >
      {name}
    </span>
  )
}

export function BranchReviewHeader({ title, branch, baseBranch, stats }: BranchReviewHeaderProps) {
  return (
    <div className="flex shrink-0 flex-col gap-2 pt-6 pb-4">
      <h1 className="min-w-0 truncate text-2xl font-semibold tracking-tight" title={title}>
        {title}
      </h1>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-muted-foreground">
        {/* One unbreakable unit: only the pills truncate, the counts wrap below it. */}
        {branch && (
          <div className="flex min-w-0 max-w-full items-center gap-2">
            <BranchPill name={branch} />
            {baseBranch && (
              <>
                <ArrowRight className="size-3.5 shrink-0" aria-hidden />
                <BranchPill name={baseBranch} keep />
              </>
            )}
          </div>
        )}
        {stats && (
          <span className="ui-text-sm flex shrink-0 items-center gap-2 font-mono">
            <span>
              {stats.files} file{stats.files === 1 ? '' : 's'}
            </span>
            <span className="text-git-added">+{stats.additions}</span>
            <span className="text-git-deleted">-{stats.deletions}</span>
          </span>
        )}
      </div>
    </div>
  )
}
