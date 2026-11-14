import { ChatCircle } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'

/** Why a file's header is showing something other than its diff. */
export type PatchState = 'truncated' | 'loading' | 'failed'

/** A file's thread count on its header: a thread in a placeholder file has no line to hang on,
 *  so the header shows it, and clicking loads the file and goes there. */
export function FileThreadCount({
  path,
  count,
  onReveal,
}: {
  path: string
  count: number
  onReveal: (path: string) => void
}) {
  if (count <= 0) return null
  return (
    <Button
      variant="ghost"
      size="xs"
      aria-label={count === 1 ? '1 comment' : `${count} comments`}
      onClick={() => onReveal(path)}
    >
      <ChatCircle />
      {count}
    </Button>
  )
}

/** The per-file "Viewed" checkbox: a viewed file collapses to its header. */
export function ViewedToggle({
  path,
  viewed,
  onToggle,
}: {
  path: string
  viewed: boolean
  onToggle: (path: string, viewed: boolean) => void
}) {
  return (
    <label className="flex cursor-pointer items-center gap-1.5 ui-text-xs text-muted-foreground">
      <Checkbox checked={viewed} onChange={(next: boolean) => onToggle(path, next)} />
      Viewed
    </label>
  )
}

/** What the header says when the body isn't the whole story: a single hunk over the cap arrives
 *  as a header-only patch, which must never read as "no changes". */
export function PatchStateNotice({
  path,
  state,
  onExpand,
  onRetry,
}: {
  path: string
  state: PatchState | undefined
  onExpand: (path: string) => void
  onRetry: (path: string) => void
}) {
  if (state == null) return null
  if (state === 'loading') {
    return <span className="ui-text-xs text-muted-foreground">Loading…</span>
  }
  if (state === 'failed') {
    return (
      <span className="flex items-center gap-2 ui-text-xs text-muted-foreground">
        Could not load this diff
        <Button variant="ghost" compact onClick={() => onRetry(path)}>
          Retry
        </Button>
      </span>
    )
  }
  return (
    <span className="flex items-center gap-2 ui-text-xs text-muted-foreground">
      Diff truncated
      <Button variant="ghost" compact onClick={() => onExpand(path)}>
        Show all
      </Button>
    </span>
  )
}
