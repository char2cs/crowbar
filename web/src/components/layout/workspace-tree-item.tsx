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
  ROW_GLYPH_BOX,
  ROW_INDENT_STEP,
  ROW_INDENT_TRANSITION,
  ROW_SUB_ACTION_HOVER,
  ROW_SUBLABEL,
  ROW_SUBLABEL_ADD,
  ROW_SUBLABEL_DEL,
  ROW_NEST_TARGET,
} from './workspace-row-base'
import { RowDisclosureButton } from './row-disclosure-button'
import { useWorkspaceTreeActions, useWorkspaceTreeDrag } from './workspace-tree-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { CollapseSection } from './collapse-section'
import { InlineCreateRow } from './inline-create-row'
import { FoldAwayButton } from './fold-away-button'
import { HeldRows, SidebarNode } from './sidebar-tree-node'
import { useKeptRows } from './use-kept-rows'
import { foldAwayRows, handleRowSelectionClick, toggleRowKeepingRows } from './row-selection'
import { dropRowProps } from './drop-target-dom'
import type { SidebarRepoTree } from './workspace-tree-utils'
import type { PlacedWorkspace, SidebarTreeNode } from './workspace-tree-utils'

interface WorkspaceRowLabelProps {
  workspace: PlacedWorkspace
  /** Whether this row is the one whose change counts are worth the second line. */
  showCounts: boolean
  /** Double-click handler; omitted on a locked row, which cannot be renamed. */
  onRename?: () => void
}

/**
 * The row's label column: branch name, and the change counts on a second line
 * beneath it.
 *
 * A flex COLUMN, not a row. The counts used to sit beside the name and take
 * width from it; moving them under it gives the width back to the branch name,
 * which is what the sidebar is for. The row itself is untouched — ROW_BASE's
 * `h-9` is a fixed 36px and the two leadings (16px + 13px) are sized to fit
 * inside it. If the active row grew to fit its second line, every row beneath it
 * would shift each time you switched workspaces.
 */
function WorkspaceRowLabel({ workspace, showCounts, onRename }: WorkspaceRowLabelProps) {
  const added = workspace.added ?? 0
  const deleted = workspace.deleted ?? 0

  return (
    <span className="flex min-w-0 flex-1 flex-col justify-center">
      {/* NOTE: unlike the repo header row's name, this span deliberately lets its
          clicks bubble. `dblclick` fires AFTER its two `click` events, so
          double-clicking to rename DOES navigate into the workspace first — but
          single-clicking a branch name is also the primary way to open it
          (asserted by workspace-tree-item-placeholder "still navigates into the
          placeholder on click"), and the two are indistinguishable at the first
          click without a dblclick-window timer on every row click. The repo
          header row has no such single-click contract on its name, so it stops
          the clicks there. */}
      <span
        className="truncate font-mono text-[13px]/[16px] text-left"
        onDoubleClick={
          onRename
            ? (e) => {
                e.stopPropagation()
                onRename()
              }
            : undefined
        }
      >
        {workspace.branch}
      </span>
      {showCounts && added + deleted > 0 && (
        <span className={ROW_SUBLABEL}>
          {added > 0 && <span className={ROW_SUBLABEL_ADD}>+{formatChangeCount(added)}</span>}
          {added > 0 && deleted > 0 && ' '}
          {deleted > 0 && <span className={ROW_SUBLABEL_DEL}>-{formatChangeCount(deleted)}</span>}
        </span>
      )}
    </span>
  )
}

/**
 * The row's add-child "+", which keeps a stable flex slot at rest.
 *
 * Keeping the invisible button in layout (see ROW_SUB_ACTION_HOVER) prevents
 * the branch label from resizing whenever the pointer crosses a row. The
 * pointerdown is stopped so pressing the control does not arm a drag of the row
 * under it.
 */
function AddChildButton({ onAdd }: { onAdd: () => void }) {
  return (
    <button
      type="button"
      className={ROW_SUB_ACTION_HOVER}
      onClick={(e) => {
        e.stopPropagation()
        onAdd()
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
  )
}

interface WorkspaceTreeItemProps {
  /** The workspace node; `kind` is already narrowed by the caller's dispatch. */
  node: Extract<SidebarTreeNode, { kind: 'workspace' }>
  depth: number
  repoId: string
  projectId: string
  /**
   * The row this one sits under in the TREE — a workspace id, a folder id, or
   * '' at the repo root. Not the same thing as the workspace's `parentId`: a
   * root row names the repo-home workspace, which is the header, not a row.
   * Drag reads it for the sibling space a reorder lands in.
   */
  containerId?: string
  activeWorkspaceId: string
  onWorkspaceClick: (wsId: string, projectId: string, repoId: string) => void
  /** This repo's rows, indexed — what a folded row asks what it is holding. */
  tree: SidebarRepoTree
}

export function WorkspaceTreeItem({
  node,
  depth,
  repoId,
  projectId,
  containerId = '',
  activeWorkspaceId,
  onWorkspaceClick,
  tree,
}: WorkspaceTreeItemProps) {
  const { workspace, children } = node
  const isActive = workspace.id === activeWorkspaceId
  const isLocked = workspace.status === 'locked'
  const isPlaceholder = isPlaceholderWorkspace(workspace)
  const hasChildren = children.length > 0
  const isCollapsed = useSidebarStore((s) => s.collapsedWorkspaces.has(workspace.id))
  const repo = useSidebarStore((s) => s.repos.find((r) => r.id === repoId))
  const expanded = !isCollapsed
  // A boolean, so a cmd-click re-renders the two rows whose answer changed and
  // nothing else. The multiselection is drawn EXACTLY like the open workspace;
  // there is no third treatment, and `aria-selected` is where the drag and
  // assistive tech both read it from, so they cannot disagree.
  const isSelected = useSidebarSelectionStore((s) => s.selected.has(workspace.id))
  const held = useKeptRows(workspace.id, tree, isCollapsed, hasChildren)

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
  const { draggingIds, dropTarget, movingIds } = useWorkspaceTreeDrag()

  // A placeholder row keeps its reason + Retry/Detach… collapsed until the user
  // ENTERS the workspace: the details render as an attached part of the row
  // while it is the active workspace and disappear when the user moves away.
  const showPlaceholderDetails = isPlaceholder && isActive

  const isCreatingChild = creatingChildOf?.parentId === workspace.id
  const isRenaming = renamingId === workspace.id
  const isDraggingThis = draggingIds.has(workspace.id)
  const isMoving = movingIds.has(workspace.id)
  // The decision was made once, in the hit test — the row only asks whether it
  // was about this row, and draws the one signal it names.
  const dropMode = dropTarget?.id === workspace.id ? dropTarget.mode : null
  // An in-flight create lives in pendingCreates AFTER the inline input is hidden
  // (confirmCreate clears creatingChildOf immediately). For a LEAF workspace
  // hasChildren and isCreatingChild are both false by then, so without this the
  // children section — and the optimistic spinner row inside it — would not
  // render and the create would show nothing.
  const hasPendingChild = Array.from(pendingCreates.values()).some(
    (p) => p.parentId === workspace.id,
  )
  // The one escape hatch that decides whether the collapsible box is open: the
  // rows themselves, an inline create, or an optimistic row still in flight.
  // Generalised from "has children" so the keep set and the create flow both
  // ride it rather than growing a parallel mechanism.
  const showChildrenSection = expanded && (hasChildren || isCreatingChild || hasPendingChild)

  const variant = isActive || isSelected ? ROW_ACTIVE : ROW_INACTIVE

  const toggle = () => toggleRowKeepingRows(workspace.id, tree.index, activeWorkspaceId)

  // react-doctor-disable-next-line js-combine-iterations -- pendingCreates is the whole tree's in-flight create operations (bounded by concurrent UI actions, realistically 0-2 at once); a single-pass rewrite here would cost JSX readability for no measurable gain.
  const pendingCreateRows = Array.from(pendingCreates.entries())
    .filter(([, p]) => p.parentId === workspace.id)
    .map(([tempId, pending]) => (
      <PendingCreateRow
        key={tempId}
        tempId={tempId}
        pending={pending}
        indent={(depth + 2) * ROW_INDENT_STEP}
        onClear={clearPendingCreate}
      />
    ))

  return (
    <div>
      <div
        className={ROW_INDENT_TRANSITION}
        style={{ marginInlineStart: (depth + 1) * ROW_INDENT_STEP }}
      >
        <div
          role="treeitem"
          // Every row is -1; the tree promotes exactly one to 0 (see
          // use-tree-keyboard.ts). A tree is ONE stop in the tab order, not one
          // per branch you happen to have open.
          tabIndex={-1}
          aria-selected={isSelected}
          {...(!isRenaming
            ? dropRowProps({
                kind: 'workspace',
                id: workspace.id,
                repoId,
                parentId: containerId,
                label: workspace.branch,
                locked: isLocked,
                expanded,
                hasChildren,
              })
            : {})}
          aria-expanded={isPlaceholder ? showPlaceholderDetails : undefined}
          className={cn(
            ROW_BASE,
            variant,
            // The hover-only "+" below leaves the flow at rest and comes back on
            // `group-hover` / `group-focus-within`; without `group` here it can
            // never come back at all.
            'group',
            isDraggingThis && 'opacity-40',
            isMoving && 'opacity-50 pointer-events-none',
            dropMode === 'into' && ROW_NEST_TARGET,
            showPlaceholderDetails && 'mb-0 rounded-b-none',
          )}
          onClick={(e) => {
            if (isRenaming) return
            // cmd/shift-click is a selection gesture, not a request to open the
            // workspace; a plain click drops the multiselection on its way in.
            if (handleRowSelectionClick(e, workspace.id, tree.index)) return
            onWorkspaceClick(workspace.id, projectId, repoId)
          }}
          onKeyDown={(e) => {
            if (!isRenaming && (e.key === 'Enter' || e.key === ' ')) {
              e.preventDefault()
              onWorkspaceClick(workspace.id, projectId, repoId)
            }
          }}
          // A protected branch drags too. It may only be reordered among its
          // own siblings, and drop-rules.ts is what enforces that — refusing to
          // pick it up at all also refused the one move it IS allowed.
          onPointerDown={
            !isRenaming
              ? (e) =>
                  onPointerDownDrag(
                    {
                      kind: 'workspace',
                      id: workspace.id,
                      repoId,
                      parentId: containerId,
                      locked: isLocked,
                    },
                    e,
                  )
              : undefined
          }
        >
          {/* Same 16px box every other leading glyph uses. The icons inside are
              already size-4; the box is what guarantees it, so a status glyph
              that ever renders at another size cannot move the label. */}
          <span className={ROW_GLYPH_BOX}>
            <WorkspaceBranchIcon
              status={workspace.status ?? 'new'}
              working={workspace.working || isMoving}
              isPlaceholder={isPlaceholder}
            />
          </span>

          {isRenaming ? (
            <WorkspaceInlineInput
              defaultValue={workspace.branch}
              onConfirm={confirmRename}
              onCancel={cancelRename}
            />
          ) : (
            <WorkspaceRowLabel
              workspace={workspace}
              showCounts={isActive && !isLocked}
              onRename={isLocked ? undefined : () => startRenaming(workspace.id)}
            />
          )}

          {held.holding && (
            <FoldAwayButton
              label={workspace.branch}
              onFold={() => foldAwayRows(workspace.id, tree.index)}
            />
          )}

          {!isCreatingChild && (
            <AddChildButton
              onAdd={() => {
                if (isCollapsed) toggle()
                startCreating(repoId, workspace.id)
              }}
            />
          )}

          {hasChildren && <RowDisclosureButton expanded={expanded} onToggle={toggle} />}
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

      {/* Held rows sit OUTSIDE the collapsible box: they are what stays behind
          when it closes over everything else. */}
      <HeldRows
        ids={held.roots}
        depth={depth + 1}
        repoId={repoId}
        projectId={projectId}
        activeWorkspaceId={activeWorkspaceId}
        onWorkspaceClick={onWorkspaceClick}
        tree={tree}
      />

      {/* One wrapper per section, so the collapse has a single box to close
          rather than N rows to keep in step. */}
      <CollapseSection open={showChildrenSection} role="group">
        {children.map((child) => (
          <SidebarNode
            key={child.id}
            node={child}
            depth={depth + 1}
            repoId={repoId}
            projectId={projectId}
            containerId={workspace.id}
            activeWorkspaceId={activeWorkspaceId}
            onWorkspaceClick={onWorkspaceClick}
            tree={tree}
          />
        ))}

        {pendingCreateRows}

        {/* The standing "New" row that used to close every expanded section is
            gone: at three levels of nesting it stacked three deep at different
            indents with nothing saying which level you were adding to. The row's
            own "+" says that unambiguously, so only the input it opens is left. */}
        {isCreatingChild && (
          <InlineCreateRow
            indent={(depth + 2) * ROW_INDENT_STEP}
            repo={repo}
            onConfirm={confirmCreate}
            onCancel={cancelCreate}
            onOpenExisting={(wsId) => onWorkspaceClick(wsId, projectId, repoId)}
          />
        )}
      </CollapseSection>
    </div>
  )
}
