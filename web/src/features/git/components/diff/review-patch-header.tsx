import { ChatCircle } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'

/** Why a file's header is showing something other than its diff. */
export type PatchState = 'truncated' | 'loading' | 'failed'

/**
 * How many review threads a file carries, on its header.
 *
 * The threads store is always loaded; the diff is not. A thread anchored in a
 * file the window has left as a placeholder has no line to attach to and would
 * otherwise be invisible — so the header says it is there, and clicking loads
 * the file and goes to it.
 */
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

/**
 * What a file's header says when its body is not the whole story.
 *
 * The case this exists for: the fixture's monster file is a SINGLE 420k-line
 * hunk, so under the server's default cap nothing fits and the response is a
 * 142-byte header-only patch. Rendered naively that reads as "this file has no
 * changes" — the one thing it must never say.
 */
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
