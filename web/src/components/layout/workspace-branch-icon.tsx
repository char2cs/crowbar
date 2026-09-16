import { GitBranch, GitFork, GitMerge, GitPullRequest, Lock, Warning } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { cn } from '@/lib/utils'
import type { WorkspaceStatus } from '@/lib/store/sidebar'

interface WorkspaceBranchIconProps {
  status: WorkspaceStatus
  /** True while an agent/long-running op is in flight — renders the spinner. */
  working?: boolean
  /** True for a placeholder (locked + no localPath) — renders the warning glyph
   *  ahead of the locked→Lock case (spec §3.3). */
  isPlaceholder?: boolean
  /** Glyph box size, matching `sidebar-row.tsx`'s own `RowGlyph` sizing
   *  (`size-4` ordinarily, `size-5` for the project-home row's large glyph).
   *  Every icon below used to hardcode `size-4`, which was exactly right for
   *  this component's one prior caller (workspace-switcher.tsx) but wrong the
   *  moment `RowGlyph` started delegating here too. */
  size?: string
  /** Forwarded from `sidebar-row.tsx`'s own `activeGround` (see that prop's
   *  doc) — this row's body sits directly on an inverted `ROW_ACTIVE` ground
   *  (Recents' solo-showing row, or every member of a showing SET). The
   *  `locked`/`new` cases below hardcode `text-foreground` — correct for the
   *  ordinary ambient sidebar background this component was originally built
   *  for (workspace-switcher.tsx), but the wrong theme's color once
   *  `RowGlyph` started delegating here too and the row can now sit on the
   *  OTHER theme's inverted surface (live-verified: the branch icon read as
   *  a barely-visible dark mark on the same dark ground its text was just
   *  fixed to invert against). The status-colored cases (amber/red/green/
   *  violet) are untouched — those are fixed brand colors, not an
   *  ambient-theme token, so they read fine on either ground. */
  invertedGround?: boolean
}

export function WorkspaceBranchIcon({
  status,
  working,
  isPlaceholder,
  size = 'size-4',
  invertedGround,
}: WorkspaceBranchIconProps) {
  // `working` is the §5 in-flight flag that replaced the old 'agent-running'
  // status overlay; it shows the spinner regardless of the underlying status.
  if (working) return <WorkspaceAgentSpinner invertedGround={invertedGround} />

  // A placeholder is a locked row, but it needs the user's attention rather than
  // the "protected, immutable" lock: render the warning glyph ahead of the switch.
  if (isPlaceholder) {
    return (
      <Warning
        role="img"
        aria-label="Branch needs provisioning"
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
