import { cn } from '@/lib/utils'
import { formatChangeCount } from './format-change-count'
import { WorkspaceBranchIcon } from './workspace-branch-icon'
import { PlaceholderRowActions } from './placeholder-row-actions'
import { isPlaceholderWorkspace } from '@/lib/workspace/placeholder'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { PendingCreateRow } from './pending-create-row'
import {
  ROW_BASE,
  ROW_ACTIVE,
  ROW_INACTIVE,
  ADD_GLYPH_PATH,
  ROW_SUB_ACTION,
  ROW_SUB_ACTION_GLYPH,
} from './workspace-row-base'
import { useWorkspaceTreeActions, useWorkspaceTreeDrag } from './workspace-tree-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { findWorkspaceForBranch } from '@/lib/workspace/branch-workspace'
import type { WorkspaceTreeNode } from './workspace-tree-utils'

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

  // react-doctor-disable-next-line js-combine-iterations -- pendingCreates is the whole tree's in-flight create operations (bounded by concurrent UI actions, realistically 0-2 at once); a single-pass rewrite here would cost JSX readability for no measurable gain.
  const pendingCreateRows = Array.from(pendingCreates.entries())
    .filter(([, p]) => p.parentId === workspace.id)
    .map(([tempId, pending]) => (
      <PendingCreateRow
        key={tempId}
        tempId={tempId}
        pending={pending}
        paddingLeft={(depth + 2) * 14}
        onClear={clearPendingCreate}
      />
    ))

  return (
    <div>
      <div style={{ paddingLeft: (depth + 1) * 14 }}>
        <div
          role="treeitem"
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
          data-oracle-id="workspace-tree-item"
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
            // No data-oracle-id here: workspace-inline-input.tsx is a separate,
            // not-yet-ported Tier B target (native/mapping/layout-denominator.md
            // §8's Cluster 7) and carries no data-oracle-id of its own anywhere
            // in its source. Anchoring this call site would invent a React-side
            // id with nothing on the other side of the port to compare against;
            // the item that ports workspace-inline-input.tsx is better placed to
            // decide where its own anchor goes.
            <WorkspaceInlineInput
              defaultValue={workspace.branch}
              onConfirm={confirmRename}
              onCancel={cancelRename}
            />
          ) : (
            // NOTE: unlike the repo header row's name, this span deliberately
            // lets its clicks bubble. `dblclick` fires AFTER its two `click`
            // events, so double-clicking to rename DOES navigate into the
            // workspace first — but single-clicking a branch name is also the
            // primary way to open it (asserted by workspace-tree-item-placeholder
            // "still navigates into the placeholder on click"), and the two are
            // indistinguishable at the first click without a dblclick-window
            // timer on every row click. The repo header row has no such
            // single-click contract on its name, so it stops the clicks there.
            <span
              className="min-w-0 flex-1 truncate font-mono text-left"
              data-oracle-id="workspace-tree-item-label"
              data-oracle-line-sized="true"
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
                  <span className="text-green-300" data-oracle-id="workspace-tree-item-added">
                    +{formatChangeCount(workspace.added)}
                  </span>
                )}
                {workspace.deleted !== undefined && workspace.deleted > 0 && (
                  <span className="text-red-300" data-oracle-id="workspace-tree-item-deleted">
                    -{formatChangeCount(workspace.deleted)}
                  </span>
                )}
              </span>
            )}

          {hasChildren ? (
            <button
              type="button"
              className={ROW_SUB_ACTION}
              onClick={(e) => {
                e.stopPropagation()
                useSidebarStore.getState().toggleWorkspace(workspace.id)
              }}
              onPointerDown={(e) => e.stopPropagation()}
              aria-label={expanded ? 'Collapse' : 'Expand'}
              data-oracle-id="workspace-tree-item-expand"
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
              className={ROW_SUB_ACTION}
              onClick={(e) => {
                e.stopPropagation()
                if (isCollapsed) useSidebarStore.getState().toggleWorkspace(workspace.id)
                startCreating(repoId, workspace.id)
              }}
              onPointerDown={(e) => e.stopPropagation()}
              aria-label="Add child workspace"
              data-oracle-id="workspace-tree-item-add-child"
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
            // This wrapper div is workspace-tree-item.tsx's own chrome (the
            // continued raised surface, background, padding) — anchored here
            // even though placeholder-row-actions.tsx's own content inside it
            // is a separate, not-yet-ported Tier B target carrying no
            // data-oracle-id of its own. Unlike the WorkspaceInlineInput slots
            // below, this div is not invented around foreign content: it is
            // this component's own real markup regardless of what renders
            // inside it.
            data-oracle-id="workspace-tree-item-placeholder-details"
          >
            <PlaceholderRowActions workspace={workspace} />
          </div>
        )}
      </div>

      {showChildrenSection && (
        <div role="group">
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

          {pendingCreateRows}

          {/* flex-col so the child <div>/<button> row STRETCHES to fill the width
              minus its own mx-1.5 (flex stretch respects margins). A bare block
              wouldn't fill the <button> child — a <button> is shrink-to-fit even
              at display:flex in WebKit — and `w-full` would overflow by the
              margins into a horizontal scrollbar (same fix as agent-chats-panel's
              NewChatRow; this is its twin). */}
          <div className="flex flex-col" style={{ paddingLeft: (depth + 2) * 14 }}>
            {isCreatingChild ? (
              // This row's own chrome (border-transparent ROW_BASE plus the
              // leading add-glyph) is this component's own markup, anchored
              // below; WorkspaceInlineInput inside it is not (same reasoning
              // as the rename slot above).
              <div
                className={cn(ROW_BASE, 'border-transparent text-foreground')}
                data-oracle-id="workspace-tree-item-create-input"
              >
                <svg
                  aria-hidden="true"
                  className={cn('size-4', ROW_SUB_ACTION_GLYPH)}
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
              // A real <button> — unlike the row above it, this one has no
              // nested interactive children (no trailing icon buttons, no
              // conditional rename input), so nothing blocks the native tag.
              <button
                type="button"
                // No `w-full`: ROW_BASE is display:flex and the flex-col parent
                // stretches this button to the row width MINUS its mx-1.5. `w-full`
                // would force width:100% AND keep the 6px margins, overflowing the
                // Workspaces panel by 6px → a stray horizontal scrollbar.
                className={cn(
                  ROW_BASE,
                  'border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
                )}
                onClick={() => startCreating(repoId, workspace.id)}
                data-oracle-id="workspace-tree-item-new-button"
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
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
