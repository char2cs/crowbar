import { GitBranch, GitFork, GitMerge, GitPullRequest, Lock, Warning } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import type { WorkspaceStatus } from '@/lib/store/sidebar'

interface WorkspaceBranchIconProps {
  status: WorkspaceStatus
  /** True while an agent/long-running op is in flight — renders the spinner. */
  working?: boolean
  /** True for a placeholder (locked + no localPath) — renders the warning glyph
   *  ahead of the locked→Lock case (spec §3.3). */
  isPlaceholder?: boolean
}

export function WorkspaceBranchIcon({ status, working, isPlaceholder }: WorkspaceBranchIconProps) {
  // `working` is the §5 in-flight flag that replaced the old 'agent-running'
  // status overlay; it shows the spinner regardless of the underlying status.
  if (working) return <WorkspaceAgentSpinner />

  // A placeholder is a locked row, but it needs the user's attention rather than
  // the "protected, immutable" lock: render the warning glyph ahead of the switch.
  if (isPlaceholder) {
    return (
      <Warning
        role="img"
        aria-label="Branch needs provisioning"
        className="size-4 shrink-0 text-amber-500"
        weight="fill"
      />
    )
  }

  switch (status) {
    case 'locked':
      return <Lock aria-hidden="true" className="size-4 shrink-0 text-foreground" weight="fill" />
    case 'new':
      return (
        <GitBranch aria-hidden="true" className="size-4 shrink-0 text-foreground" weight="fill" />
      )
    case 'pr-conflicts':
      return <Warning aria-hidden="true" className="size-4 shrink-0 text-amber-500" weight="fill" />
    case 'deleted':
      return <GitFork aria-hidden="true" className="size-4 shrink-0 text-red-500" weight="fill" />
    case 'pr-open':
      return (
        <GitPullRequest
          aria-hidden="true"
          className="size-4 shrink-0 text-green-500"
          weight="fill"
        />
      )
    case 'pr-closed':
      return <GitFork aria-hidden="true" className="size-4 shrink-0 text-red-500" weight="fill" />
    case 'pr-merged':
      return (
        <GitMerge aria-hidden="true" className="size-4 shrink-0 text-violet-500" weight="fill" />
      )
    default: {
      const _exhaustive: never = status
      return _exhaustive
    }
  }
}

export function WorkspaceAgentSpinner() {
  // Theme-token colored (text-primary), never a provider/hardcoded color; the
  // <FlickerSpinner> random-picks a flicker spinner and animates it.
  return (
    <span className="flex size-4 shrink-0 items-center justify-center text-primary">
      <FlickerSpinner className="size-3.5" />
    </span>
  )
}
