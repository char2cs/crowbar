import { GitBranch, GitFork, GitMerge, GitPullRequest, Lock, Warning } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { cn } from '@/lib/utils'
import type { WorkspaceStatus } from '@/core/entities/repo-types'

interface WorkspaceBranchIconProps {
  status: WorkspaceStatus
  working?: boolean
  isPlaceholder?: boolean
  reason?: string
  size?: string
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
  if (working) return <WorkspaceAgentSpinner invertedGround={invertedGround} />

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

function WorkspaceAgentSpinner({ invertedGround }: { invertedGround?: boolean } = {}) {
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
