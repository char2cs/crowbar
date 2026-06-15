import { useState } from 'react'
import { GitBranch, GitFork, GitMerge, GitPullRequest, Lock } from '@phosphor-icons/react'
import { Spinner, spinnerNames } from '@agilek/cli-loaders'
import type { WorkspaceStatus } from '@/lib/store/sidebar'

interface WorkspaceBranchIconProps {
  status: WorkspaceStatus
}

export function WorkspaceBranchIcon({ status }: WorkspaceBranchIconProps) {
  if (status === 'agent-running') return <WorkspaceAgentSpinner />

  switch (status) {
    case 'locked':
      return (
        <Lock aria-hidden="true" className="size-4 shrink-0 text-foreground" weight="fill" />
      )
    case 'new':
      return (
        <GitBranch aria-hidden="true" className="size-4 shrink-0 text-foreground" weight="fill" />
      )
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
