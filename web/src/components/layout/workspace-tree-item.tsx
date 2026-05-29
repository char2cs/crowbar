import { useState } from 'react'
import { cn } from '@/lib/utils'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import type { WorkspaceTreeNode } from './workspace-tree'

interface WorkspaceTreeItemProps {
  node: WorkspaceTreeNode
  depth: number
  activeWorkspaceId: string
  onWorkspaceClick: (wsId: string) => void
}

const NAV_BASE =
  'relative flex min-w-0 flex-1 shrink-0 cursor-pointer items-center gap-1.5 rounded-lg border ' +
  'px-2 py-1 text-xs font-medium outline-none transition-shadow ' +
  'focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background'

export function WorkspaceTreeItem({
  node, depth, activeWorkspaceId, onWorkspaceClick,
}: WorkspaceTreeItemProps) {
  const { workspace, children } = node
  const isActive = workspace.id === activeWorkspaceId
  const isLocked = workspace.status === 'locked'
  const hasChildren = children.length > 0
  const [expanded, setExpanded] = useState(true)

  const variant = isActive
    ? 'border-primary bg-primary text-primary-foreground shadow-xs hover:bg-primary/90'
    : isLocked
      ? 'border-transparent text-foreground/30 hover:bg-accent'
      : 'border-transparent text-foreground hover:bg-accent'

  return (
    <div>
      {/* Row: nav button + optional chevron as siblings — no nested buttons */}
      <div className="mb-px flex items-center" style={{ paddingLeft: depth * 14 }}>
        <button
          type="button"
          className={cn(NAV_BASE, variant)}
          onClick={() => onWorkspaceClick(workspace.id)}
        >
          <WorkspaceBranchIcon status={workspace.status ?? 'new'} />

          <span className="min-w-0 flex-1 truncate font-mono text-left">
            {workspace.branch}
          </span>

          {isActive && (workspace.added !== undefined || workspace.deleted !== undefined) && (
            <span className="flex shrink-0 gap-1 font-mono">
              {workspace.added !== undefined && workspace.added > 0 && (
                <span className="text-green-300">
                  +{workspace.added > 999 ? `${Math.round(workspace.added / 1000)}k` : workspace.added}
                </span>
              )}
              {workspace.deleted !== undefined && workspace.deleted > 0 && (
                <span className="text-red-300">
                  -{workspace.deleted > 999 ? `${Math.round(workspace.deleted / 1000)}k` : workspace.deleted}
                </span>
              )}
            </span>
          )}
        </button>

        {hasChildren && (
          <button
            type="button"
            className="ml-0.5 shrink-0 rounded p-1 opacity-30 hover:opacity-80 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            onClick={() => setExpanded(v => !v)}
            aria-label={expanded ? 'Collapse' : 'Expand'}
          >
            <svg
              aria-hidden="true"
              className={cn('size-2.5 transition-transform', expanded && 'rotate-90')}
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="2"
              strokeLinecap="round"
            >
              <path d="M6 3l5 5-5 5" />
            </svg>
          </button>
        )}
      </div>

      {hasChildren && expanded && (
        <div>
          {children.map(child => (
            <WorkspaceTreeItem
              key={child.workspace.id}
              node={child}
              depth={depth + 1}
              activeWorkspaceId={activeWorkspaceId}
              onWorkspaceClick={onWorkspaceClick}
            />
          ))}
        </div>
      )}
    </div>
  )
}
