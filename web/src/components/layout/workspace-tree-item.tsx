import { useRef, useState } from 'react'
import { cn } from '@/lib/utils'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { ROW_BASE } from './workspace-row-base'
import { useWorkspaceTreeContext } from './workspace-tree-context'
import type { WorkspaceTreeNode } from './workspace-tree'

interface WorkspaceTreeItemProps {
  node: WorkspaceTreeNode
  depth: number
  repoId: string
  activeWorkspaceId: string
  onWorkspaceClick: (wsId: string) => void
}

export function WorkspaceTreeItem({
  node, depth, repoId, activeWorkspaceId, onWorkspaceClick,
}: WorkspaceTreeItemProps) {
  const { workspace, children } = node
  const isActive = workspace.id === activeWorkspaceId
  const isLocked = workspace.status === 'locked'
  const hasChildren = children.length > 0
  const [expanded, setExpanded] = useState(true)
  const [isDragOver, setIsDragOver] = useState(false)
  const dragCounter = useRef(0)

  const {
    creatingChildOf, startCreating, confirmCreate, cancelCreate,
    renamingId, startRenaming, confirmRename, cancelRename,
    draggingWs, startDragging, endDragging, dropOnWorkspace,
  } = useWorkspaceTreeContext()

  const isCreatingChild = creatingChildOf?.parentId === workspace.id
  const isRenaming = renamingId === workspace.id
  const isDraggingThis = draggingWs?.id === workspace.id
  const showChildrenSection = (hasChildren && expanded) || isCreatingChild

  const variant = isActive
    ? 'border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/90'
    : isLocked
      ? 'border-transparent text-foreground/30 hover:bg-accent'
      : 'border-transparent text-foreground hover:bg-accent'

  function handleDragStart(e: React.DragEvent) {
    e.dataTransfer.effectAllowed = 'move'
    // Slight delay so the drag image captures the pre-fade state
    requestAnimationFrame(() => startDragging(workspace.id, repoId))
  }

  function handleDragEnter(e: React.DragEvent) {
    e.preventDefault()
    dragCounter.current++
    setIsDragOver(true)
  }

  function handleDragLeave() {
    dragCounter.current--
    if (dragCounter.current === 0) setIsDragOver(false)
  }

  function handleDragOver(e: React.DragEvent) {
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault()
    dragCounter.current = 0
    setIsDragOver(false)
    dropOnWorkspace(workspace.id, repoId)
  }

  return (
    <div>
      <div style={{ paddingLeft: (depth + 1) * 14 }}>
        <div
          role="button"
          tabIndex={0}
          draggable={!isRenaming}
          className={cn(
            ROW_BASE,
            variant,
            isDraggingThis && 'opacity-40',
            isDragOver && !isDraggingThis && 'ring-1 ring-ring',
          )}
          onClick={() => !isRenaming && onWorkspaceClick(workspace.id)}
          onKeyDown={(e) => {
            if (!isRenaming && (e.key === 'Enter' || e.key === ' ')) {
              e.preventDefault()
              onWorkspaceClick(workspace.id)
            }
          }}
          onDragStart={handleDragStart}
          onDragEnd={endDragging}
          onDragEnter={handleDragEnter}
          onDragLeave={handleDragLeave}
          onDragOver={handleDragOver}
          onDrop={handleDrop}
        >
          <WorkspaceBranchIcon status={workspace.status ?? 'new'} />

          {isRenaming ? (
            <WorkspaceInlineInput
              defaultValue={workspace.branch}
              onConfirm={confirmRename}
              onCancel={cancelRename}
            />
          ) : (
            <span
              className="min-w-0 flex-1 truncate font-mono text-left"
              onDoubleClick={(e) => { e.stopPropagation(); startRenaming(workspace.id) }}
            >
              {workspace.branch}
            </span>
          )}

          {isActive && !isRenaming && (workspace.added !== undefined || workspace.deleted !== undefined) && (
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

          {hasChildren ? (
            <button
              type="button"
              className="shrink-0 rounded-md p-1 text-foreground/30 hover:text-foreground/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              onClick={(e) => { e.stopPropagation(); setExpanded(v => !v) }}
              aria-label={expanded ? 'Collapse' : 'Expand'}
            >
              <svg
                aria-hidden="true"
                className={cn('size-3 transition-transform', expanded && 'rotate-90')}
                viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"
              >
                <path d="M6 3l5 5-5 5" />
              </svg>
            </button>
          ) : !isCreatingChild ? (
            <button
              type="button"
              className="shrink-0 rounded-md p-1 text-foreground/30 hover:text-foreground/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              onClick={(e) => {
                e.stopPropagation()
                setExpanded(true)
                startCreating(repoId, workspace.id)
              }}
              aria-label="Add child workspace"
            >
              <svg aria-hidden="true" className="size-3" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                <path d="M8 3v10M3 8h10" />
              </svg>
            </button>
          ) : null}
        </div>
      </div>

      {showChildrenSection && (
        <div>
          {hasChildren && expanded && children.map(child => (
            <WorkspaceTreeItem
              key={child.workspace.id}
              node={child}
              depth={depth + 1}
              repoId={repoId}
              activeWorkspaceId={activeWorkspaceId}
              onWorkspaceClick={onWorkspaceClick}
            />
          ))}

          <div style={{ paddingLeft: (depth + 2) * 14 }}>
            {isCreatingChild ? (
              <div className={cn(ROW_BASE, 'border-transparent text-foreground')}>
                <svg aria-hidden="true" className="size-4 shrink-0 text-foreground/30" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                  <path d="M8 3v10M3 8h10" />
                </svg>
                <WorkspaceInlineInput
                  onConfirm={confirmCreate}
                  onCancel={cancelCreate}
                />
              </div>
            ) : (
              <div
                role="button"
                tabIndex={0}
                className={cn(ROW_BASE, 'border-transparent text-muted-foreground/40 hover:bg-accent hover:text-muted-foreground/60')}
                onClick={() => startCreating(repoId, workspace.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    startCreating(repoId, workspace.id)
                  }
                }}
              >
                <svg aria-hidden="true" className="size-4 shrink-0" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                  <path d="M8 3v10M3 8h10" />
                </svg>
                <span className="font-mono text-left text-[13px]">New</span>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
