import { useState } from 'react'
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
        <svg className="size-2.5 shrink-0 text-foreground/30" viewBox="0 0 16 16" fill="currentColor">
          <path d="M8 1a3 3 0 0 0-3 3v2H4a1 1 0 0 0-1 1v7h10V7a1 1 0 0 0-1-1h-1V4a3 3 0 0 0-3-3zm0 1.5A1.5 1.5 0 0 1 9.5 4v2h-3V4A1.5 1.5 0 0 1 8 2.5z" />
        </svg>
      )
    case 'new':
      return (
        <svg className="size-2.5 shrink-0 text-foreground/30" viewBox="0 0 16 16" fill="currentColor">
          <path d="M11.75 2.5a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0zm.75 2.25a2.25 2.25 0 1 1 0-4.5 2.25 2.25 0 0 1 0 4.5zM4.25 5.5A2.25 2.25 0 1 0 4.25 1a2.25 2.25 0 0 0 0 4.5zM4 12.75a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0zM3.25 15a2.25 2.25 0 1 0 0-4.5 2.25 2.25 0 0 0 0 4.5zm.75-10.372v5.244a2.25 2.25 0 0 0 0 4.256V15h1.5v-.872a2.25 2.25 0 0 0 0-4.256V4.628a2.25 2.25 0 0 0 0-4.256V.5H4v.372a2.25 2.25 0 0 0 0 4.256z" />
        </svg>
      )
    case 'pr-open':
      return (
        <svg className="size-2.5 shrink-0 text-green-500" viewBox="0 0 16 16" fill="currentColor">
          <path d="M5 3.25a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0zm0 2.122a2.25 2.25 0 1 0-1.5 0v.878A2.25 2.25 0 0 0 5.75 8.5h1.5v1.128a2.251 2.251 0 1 0 1.5 0V8.5h1.5a2.25 2.25 0 0 0 2.25-2.25v-.878a2.25 2.25 0 1 0-1.5 0v.878a.75.75 0 0 1-.75.75h-4.5A.75.75 0 0 1 5 6.25v-.878zm3.75 7.378a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0zm3-8.75a.75.75 0 1 1-1.5 0 .75.75 0 0 1 1.5 0z" />
        </svg>
      )
    case 'pr-closed':
      return (
        <svg className="size-2.5 shrink-0 text-red-500" viewBox="0 0 16 16" fill="currentColor">
          <path d="M3.72 3.72a.75.75 0 0 1 1.06 0L8 6.94l3.22-3.22a.749.749 0 0 1 1.275.326.749.749 0 0 1-.215.734L9.06 8l3.22 3.22a.749.749 0 0 1-.326 1.275.749.749 0 0 1-.734-.215L8 9.06l-3.22 3.22a.751.751 0 0 1-1.042-.018.751.751 0 0 1-.018-1.042L6.94 8 3.72 4.78a.75.75 0 0 1 0-1.06z" />
        </svg>
      )
    case 'pr-merged':
      return (
        <svg className="size-2.5 shrink-0 text-purple-500" viewBox="0 0 16 16" fill="currentColor">
          <path d="M5.45 5.154A4.25 4.25 0 0 0 9.25 7.5h1.378a2.251 2.251 0 1 1 0 1.5H9.25A5.734 5.734 0 0 1 5 7.123v3.505a2.25 2.25 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.95-.218zM4.25 13.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5zm8.5-4.5a.75.75 0 1 0 0-1.5.75.75 0 0 0 0 1.5zM5 3.25a.75.75 0 1 0 0 .005V3.25z" />
        </svg>
      )
  }
}

export function WorkspaceAgentSpinner() {
  const [name] = useState(
    () => spinnerNames[Math.floor(Math.random() * spinnerNames.length)]
  )
  return (
    <span className="size-2.5 shrink-0 text-violet-400 leading-none">
      <Spinner name={name} color="currentColor" size="0.625rem" />
    </span>
  )
}
