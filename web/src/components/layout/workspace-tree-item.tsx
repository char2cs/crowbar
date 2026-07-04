import { cn } from '@/lib/utils'
import { formatChangeCount } from './format-change-count'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { PlaceholderRowActions } from './placeholder-row-actions'
import { isPlaceholderWorkspace } from '@/lib/workspace/placeholder'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { PendingCreateRow } from './pending-create-row'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE, ADD_GLYPH_PATH } from './workspace-row-base'
import { useWorkspaceTreeActions, useWorkspaceTreeDrag } from './workspace-tree-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { findWorkspaceForBranch } from '@/lib/workspace/branch-workspace'
import type { WorkspaceTreeNode } from './workspace-tree'

interface WorkspaceTreeItemProps {
  node: WorkspaceTreeNode
  depth: number
  repoId: string
  projectId: string
  activeWorkspaceId: string
  onWorkspaceClick: (wsId: string, projectId: string, repoId: string) => void
}

export function WorkspaceTreeItem({
  node,
  depth,
  repoId,
  projectId,
  activeWorkspaceId,
  onWorkspaceClick,
}: WorkspaceTreeItemProps) {
  const { workspace, children } = node
  const isActive = workspace.id === activeWorkspaceId
  const isLocked = workspace.status === 'locked'
  const isPlaceholder = isPlaceholderWorkspace(workspace)
  const hasChildren = children.length > 0
  const isCollapsed = useSidebarStore((s) => s.collapsedWorkspaces.has(workspace.id))
  const repo = useSidebarStore((s) => s.repos.find((r) => r.id === repoId))
  const expanded = !isCollapsed

  const {
    creatingChildOf,
    startCreating,
    confirmCreate,
    cancelCreate,
    renamingId,
    startRenaming,
    confirmRename,
    cancelRename,
    onPointerDownDrag,
    pendingCreates,
    clearPendingCreate,
  } = useWorkspaceTreeActions()
  const { draggingWs, hoverTargetId, movingWsId } = useWorkspaceTreeDrag()

  // A placeholder row keeps its reason + Retry/Detach… collapsed until the user
  // ENTERS the workspace: the details render as an attached part of the row
  // while it is the active workspace and disappear when the user moves away.
  const showPlaceholderDetails = isPlaceholder && isActive

  const isCreatingChild = creatingChildOf?.parentId === workspace.id
  const isRenaming = renamingId === workspace.id
  const isDraggingThis = draggingWs?.id === workspace.id
  const isMoving = movingWsId === workspace.id
  const isDropTarget = hoverTargetId === `ws:${workspace.id}` && !isDraggingThis
  // An in-flight create lives in pendingCreates AFTER the inline input is hidden
  // (confirmCreate clears creatingChildOf immediately). For a LEAF workspace
  // hasChildren and isCreatingChild are both false by then, so without this the
  // children section — and the optimistic spinner row inside it — would not
  // render and the create would show nothing.
  const hasPendingChild = Array.from(pendingCreates.values()).some(
    (p) => p.parentId === workspace.id,
  )
  const showChildrenSection = (hasChildren && expanded) || isCreatingChild || hasPendingChild

  const variant = isActive ? ROW_ACTIVE : ROW_INACTIVE

  return (
    <div>
      <div style={{ paddingLeft: (depth + 1) * 14 }}>
        <div
          role="button"
          tabIndex={0}
          data-ws-drop={!isRenaming ? workspace.id : undefined}
          aria-expanded={isPlaceholder ? showPlaceholderDetails : undefined}
          className={cn(
            ROW_BASE,
            variant,
            isDraggingThis && 'opacity-40',
            isMoving && 'opacity-50 pointer-events-none',
            isDropTarget && 'ring-1 ring-ring',
            showPlaceholderDetails && 'mb-0 rounded-b-none',
          )}
          onClick={() => !isRenaming && onWorkspaceClick(workspace.id, projectId, repoId)}
          onKeyDown={(e) => {
            if (!isRenaming && (e.key === 'Enter' || e.key === ' ')) {
              e.preventDefault()
              onWorkspaceClick(workspace.id, projectId, repoId)
            }
          }}
          onPointerDown={
            !isRenaming && !isLocked
              ? (e) => onPointerDownDrag(workspace.id, repoId, workspace.branch, e)
              : undefined
          }
        >
          <WorkspaceBranchIcon
            status={workspace.status ?? 'new'}
            working={workspace.working || isMoving}
            isPlaceholder={isPlaceholder}
          />

          {isRenaming ? (
            <WorkspaceInlineInput
              defaultValue={workspace.branch}
              onConfirm={confirmRename}
              onCancel={cancelRename}
            />
          ) : (
            <span
              className="min-w-0 flex-1 truncate font-mono text-left"
              onDoubleClick={(e) => {
                if (isLocked) return
                e.stopPropagation()
                startRenaming(workspace.id)
              }}
            >
              {workspace.branch}
            </span>
          )}

          {isActive &&
            !isRenaming &&
            !isLocked &&
            (workspace.added !== undefined || workspace.deleted !== undefined) && (
              <span className="flex shrink-0 gap-1 font-mono">
                {workspace.added !== undefined && workspace.added > 0 && (
                  <span className="text-green-300">+{formatChangeCount(workspace.added)}</span>
                )}
                {workspace.deleted !== undefined && workspace.deleted > 0 && (
                  <span className="text-red-300">-{formatChangeCount(workspace.deleted)}</span>
                )}
              </span>
            )}

          {hasChildren ? (
            <button
              type="button"
              className="shrink-0 rounded-md p-1 text-foreground/30 hover:text-foreground/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              onClick={(e) => {
                e.stopPropagation()
                useSidebarStore.getState().toggleWorkspace(workspace.id)
              }}
              onPointerDown={(e) => e.stopPropagation()}
              aria-label={expanded ? 'Collapse' : 'Expand'}
            >
              <svg
                aria-hidden="true"
                className={cn('size-3 transition-transform', expanded && 'rotate-90')}
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
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
                if (isCollapsed) useSidebarStore.getState().toggleWorkspace(workspace.id)
                startCreating(repoId, workspace.id)
              }}
              onPointerDown={(e) => e.stopPropagation()}
              aria-label="Add child workspace"
            >
              <svg
                aria-hidden="true"
                className="size-3"
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              >
                <path d={ADD_GLYPH_PATH} />
              </svg>
            </button>
          ) : null}
        </div>

        {showPlaceholderDetails && (
          <div
            className={cn(
              'mx-1.5 mb-0.5 rounded-b-lg px-2.5 pb-2 pt-0.5',
              // Continue the ACTIVE row's raised surface so row + details read
              // as one card (the row squares its bottom corners while shown).
              'bg-background shadow-xs shadow-black/10',
            )}
          >
            <PlaceholderRowActions workspace={workspace} />
          </div>
        )}
      </div>

      {showChildrenSection && (
        <div>
          {hasChildren &&
            expanded &&
            children.map((child) => (
              <WorkspaceTreeItem
                key={child.workspace.id}
                node={child}
                depth={depth + 1}
                repoId={repoId}
                projectId={projectId}
                activeWorkspaceId={activeWorkspaceId}
                onWorkspaceClick={onWorkspaceClick}
              />
            ))}

          {Array.from(pendingCreates.entries())
            .filter(([, p]) => p.parentId === workspace.id)
            .map(([tempId, pending]) => (
              <PendingCreateRow
                key={tempId}
                tempId={tempId}
                pending={pending}
                paddingLeft={(depth + 2) * 14}
                onClear={clearPendingCreate}
              />
            ))}

          <div style={{ paddingLeft: (depth + 2) * 14 }}>
            {isCreatingChild ? (
              <div className={cn(ROW_BASE, 'border-transparent text-foreground')}>
                <svg
                  aria-hidden="true"
                  className="size-4 shrink-0 text-foreground/30"
                  viewBox="0 0 16 16"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                >
                  <path d={ADD_GLYPH_PATH} />
                </svg>
                <WorkspaceInlineInput
                  onConfirm={confirmCreate}
                  onCancel={cancelCreate}
                  resolveExisting={(b) => (repo ? findWorkspaceForBranch(repo, b) : null)}
                  onOpenExisting={(wsId) => onWorkspaceClick(wsId, projectId, repoId)}
                />
              </div>
            ) : (
              <div
                role="button"
                tabIndex={0}
                className={cn(
                  ROW_BASE,
                  'border-transparent text-muted-foreground/40 hover:bg-accent hover:text-muted-foreground/60',
                )}
                onClick={() => startCreating(repoId, workspace.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault()
                    startCreating(repoId, workspace.id)
                  }
                }}
              >
                <svg
                  aria-hidden="true"
                  className="size-4 shrink-0"
                  viewBox="0 0 16 16"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2"
                  strokeLinecap="round"
                >
                  <path d={ADD_GLYPH_PATH} />
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
