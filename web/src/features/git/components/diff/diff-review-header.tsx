import { ArrowRight } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { formatRelativeDate } from '@/utils/date'

export interface DiffReviewHeaderProps {
  /** Primary title: a commit message (commit diff) or a branch name (branch review). */
  title?: string
  /** Optional secondary body under the title (e.g. a commit's extended description). */
  description?: string
  /** Commit identity meta (commit diffs). */
  author?: string
  date?: string
  hash?: string
  /** Branch-review meta: the base branch this branch compares into (e.g. "develop"). */
  baseBranch?: string
}

/**
 * The shared title + meta block for a multi-file review surface, used by BOTH
 * the commit/stash/tag diff and the branch review so the two read as siblings.
 * The structure is identical; only the source of each row swaps:
 *   - commit  → title = commit message, meta = author · date · ⟨hash⟩
 *   - branch  → title = branch name,    meta = → baseBranch
 * It sits directly under the diff toolbar (the breadcrumb row).
 */
export function DiffReviewHeader({
  title,
  description,
  author,
  date,
  hash,
  baseBranch,
}: DiffReviewHeaderProps) {
  const hasMeta = Boolean(baseBranch || author || date || hash)
  if (!title && !description && !hasMeta) return null

  return (
    <div className="bg-background px-2 py-2">
      <div className="px-1 py-1.5">
        {title ? <div className="ui-text-sm font-medium text-foreground">{title}</div> : null}
        {description ? (
          <div className="ui-text-sm mt-2 whitespace-pre-wrap text-muted-foreground">
            {description}
          </div>
        ) : null}
        {hasMeta ? (
          <div className="ui-text-sm mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-muted-foreground">
            {baseBranch ? (
              <span className="flex items-center gap-1">
                <ArrowRight className="size-3.5" />
                <span className="font-mono">{baseBranch}</span>
              </span>
            ) : null}
            {author ? <span>{author}</span> : null}
            {date ? <span>{formatRelativeDate(date)}</span> : null}
            {hash ? (
              <Badge size="sm" variant="secondary">
                {hash}
              </Badge>
            ) : null}
          </div>
        ) : null}
      </div>
    </div>
  )
}
