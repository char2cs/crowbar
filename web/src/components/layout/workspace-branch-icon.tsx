import { useState } from 'react'
import {
  GitBranch,
  GitFork,
  GitMerge,
  GitPullRequest,
  Lock,
  Warning,
} from '@phosphor-icons/react'
import { Spinner, spinnerNames } from '@agilek/cli-loaders'
import type { WorkspaceStatus } from '@/lib/store/sidebar'

interface WorkspaceBranchIconProps {
  status: WorkspaceStatus
  /** True while an agent/long-running op is in flight — renders the spinner. */
  working?: boolean
}

export function WorkspaceBranchIcon({ status, working }: WorkspaceBranchIconProps) {
  // `working` is the §5 in-flight flag that replaced the old 'agent-running'
  // status overlay; it shows the spinner regardless of the underlying status.
  if (working) return <WorkspaceAgentSpinner />

  switch (status) {
    case 'locked':
      return (
        <Lock aria-hidden="true" className="size-4 shrink-0 text-foreground" weight="fill" />
      )
    case 'new':
      return (
        <GitBranch aria-hidden="true" className="size-4 shrink-0 text-foreground" weight="fill" />
      )
    case 'pr-conflicts':
      return (
        <Warning aria-hidden="true" className="size-4 shrink-0 text-amber-500" weight="fill" />
      )
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
  const [name] = useState(() => spinnerNames[Math.floor(Math.random() * spinnerNames.length)])
  return (
    <span className="size-4 shrink-0 text-primary leading-none flex items-center justify-center">
      <Spinner name={name} color="currentColor" size="0.875rem" shape="square" />
    </span>
  )
}
