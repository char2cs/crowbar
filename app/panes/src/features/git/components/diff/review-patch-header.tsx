import { ChatCircle } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'

export type PatchState = 'truncated' | 'loading' | 'failed'

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
