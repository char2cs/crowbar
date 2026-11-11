import { GitBranch, GitFork, GitMerge, GitPullRequest, Lock, Warning } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { cn } from '@/lib/utils'
import type { WorkspaceStatus } from '@/core/entities/repo-types'

interface WorkspaceBranchIconProps {
  status: WorkspaceStatus
  /** True while an agent/long-running op is in flight — renders the spinner. */
  working?: boolean
  /** True for a workspace Crowbar could not provision — renders the warning
   *  glyph ahead of the locked→Lock case (spec §3.3). */
  isPlaceholder?: boolean
  /** Why this row has no worktree, as the glyph's `<title>` so hover names the problem (an
   *  `aria-label` alone reaches only screen readers). */
  reason?: string
  /** Glyph box size, matching `RowGlyph` (`size-4`; `size-5` for the project-home row). */
  size?: string
  /** The row sits on the inverted TAB_ACTIVE ground (`activeGround`), so the `locked`/`new` glyphs
   *  invert their foreground; the fixed status colours read on either ground. */
  invertedGround?: boolean
}

export function WorkspaceBranchIcon({
  status,
  working,
  isPlaceholder,
  reason,
  size = 'size-4',
  invertedGround,
}: WorkspaceBranchIconProps) {
  // `working` is the §5 in-flight flag; it shows the spinner whatever the underlying status.
  if (working) return <WorkspaceAgentSpinner invertedGround={invertedGround} />

  // A placeholder is a locked row, but it needs the user's attention rather than
  // the "protected, immutable" lock: render the warning glyph ahead of the switch.
  if (isPlaceholder) {
    return (
      <Warning
        role="img"
        aria-label="Branch needs provisioning"
        alt={reason}
        className={cn(size, 'shrink-0 text-amber-500')}
        weight="fill"
      />
    )
  }

  switch (status) {
    case 'locked':
      return (
        <Lock
          aria-hidden="true"
          // A locked branch held by the repo's own checkout draws this same
          // Lock as a managed one — the reason is the only thing telling them
          // apart, so it goes on the glyph rather than nowhere.
          alt={reason}
          className={cn(
            size,
            'shrink-0 text-foreground',
            invertedGround && 'text-foreground-inverse',
          )}
          weight="fill"
        />
      )
    case 'new':
      return (
        <GitBranch
          aria-hidden="true"
          className={cn(
            size,
            'shrink-0 text-foreground',
            invertedGround && 'text-foreground-inverse',
          )}
          weight="fill"
        />
      )
    case 'pr-conflicts':
      return (
        <Warning aria-hidden="true" className={cn(size, 'shrink-0 text-amber-500')} weight="fill" />
      )
    case 'deleted':
      return (
        <GitFork aria-hidden="true" className={cn(size, 'shrink-0 text-red-500')} weight="fill" />
      )
    case 'pr-open':
      return (
        <GitPullRequest
          aria-hidden="true"
          className={cn(size, 'shrink-0 text-green-500')}
          weight="fill"
        />
      )
    case 'pr-closed':
      return (
        <GitFork aria-hidden="true" className={cn(size, 'shrink-0 text-red-500')} weight="fill" />
      )
    case 'pr-merged':
      return (
        <GitMerge
          aria-hidden="true"
          className={cn(size, 'shrink-0 text-violet-500')}
          weight="fill"
        />
      )
    default: {
      const _exhaustive: never = status
      return _exhaustive
    }
  }
}

export function WorkspaceAgentSpinner({ invertedGround }: { invertedGround?: boolean } = {}) {
  // Theme-token colored, never a provider/hardcoded color; the <FlickerSpinner>
  // random-picks a flicker spinner and animates it.
  return (
    <span
      className={cn(
        'flex size-4 shrink-0 items-center justify-center text-foreground',
        invertedGround && 'text-foreground-inverse',
      )}
    >
      <FlickerSpinner className="size-3.5" />
    </span>
  )
}
