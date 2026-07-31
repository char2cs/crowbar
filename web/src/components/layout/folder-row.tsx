import { Folder, FolderOpen } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import {
  ROW_BASE,
  ROW_ACTIVE,
  ROW_INACTIVE,
  ROW_GLYPH_BOX,
  ROW_INDENT_STEP,
  ROW_INDENT_TRANSITION,
  ROW_SUB_ACTION,
  ROW_SUB_ACTION_HOVER,
  ROW_NEST_TARGET,
  ADD_GLYPH_PATH,
} from './workspace-row-base'
import { useWorkspaceTreeActions, useWorkspaceTreeDrag } from './workspace-tree-context'
import { CollapseSection } from './collapse-section'
import { InlineCreateRow } from './inline-create-row'
import { PendingCreateRow } from './pending-create-row'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { FoldAwayButton } from './fold-away-button'
import { HeldRows, SidebarNode } from './sidebar-tree-node'
import { useKeptRows } from './use-kept-rows'
import { foldAwayRows, handleRowSelectionClick, toggleRowKeepingRows } from './row-selection'
import { dropRowProps } from './drop-target-dom'
import type { SidebarRepoTree, SidebarTreeNode } from './workspace-tree-utils'

interface FolderRowProps {
  /** The folder node; `kind` is already narrowed by the caller's dispatch. */
  node: Extract<SidebarTreeNode, { kind: 'folder' }>
  depth: number
  repoId: string
  projectId: string
  /** The row this folder sits under: a workspace id, a folder id, or '' at the
   *  repo root. Drag reads it for the sibling space a reorder lands in. */
  containerId?: string
  activeWorkspaceId: string
  onWorkspaceClick: (wsId: string, projectId: string, repoId: string) => void
  /** This repo's rows, indexed — what a folded row asks what it is holding. */
  tree: SidebarRepoTree
}

/**
 * A grouping folder in the workspace tree.
 *
 * It joins the same contract every other row keeps rather than inventing its
 * own: `role="treeitem"` (never `role="button"`, which takes PRESENTATIONAL
 * children and would erase the row's own `+` and chevron), `aria-expanded`
 * reflecting this folder's collapse state, and children in a fresh
 * `role="group"`. Chrome comes from ROW_BASE — a hand-rolled row would drift
 * from the branch rows it interleaves with.
 *
 * A folder has no destination, so the row body toggles it. Everything a
 * workspace row can hold, a folder can hold: workspaces, and other folders.
 */
export function FolderRow({
  node,
  depth,
  repoId,
  projectId,
  containerId = '',
  activeWorkspaceId,
  onWorkspaceClick,
  tree,
}: FolderRowProps) {
  const { folder, children } = node
  // Folders share the workspace collapse set: the two id spaces never overlap
  // (both are backend uuids) and a folder collapse is the same user intent,
  // persisted the same way, at the same level of the same tree.
  const isCollapsed = useSidebarStore((s) => s.collapsedWorkspaces.has(folder.id))
  const expanded = !isCollapsed
  const hasChildren = children.length > 0
  // Narrow boolean: a cmd-click re-renders the rows whose answer changed, and
  // a selected folder is drawn exactly like the open workspace.
  const isSelected = useSidebarSelectionStore((s) => s.selected.has(folder.id))
  const held = useKeptRows(folder.id, tree, isCollapsed, hasChildren)

  const repo = useSidebarStore((s) => s.repos.find((r) => r.id === repoId))
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
  const { draggingIds, dropTarget } = useWorkspaceTreeDrag()
  const dropMode = dropTarget?.id === folder.id ? dropTarget.mode : null
  const isCreatingChild = creatingChildOf?.parentId === folder.id
  const isRenaming = renamingId === folder.id

  // The create input is hidden the moment it is confirmed, so a folder that had
  // no children until now would close over its own in-flight row and the create
  // would show nothing at all while the worktree is cut.
  // react-doctor-disable-next-line js-combine-iterations -- pendingCreates is the whole tree's in-flight create operations (bounded by concurrent UI actions, realistically 0-2 at once); a single-pass rewrite here would cost JSX readability for no measurable gain.
  const pendingCreateRows = Array.from(pendingCreates.entries())
    .filter(([, p]) => p.parentId === folder.id)
    .map(([tempId, pending]) => (
      <PendingCreateRow
        key={tempId}
        tempId={tempId}
        pending={pending}
        indent={(depth + 2) * ROW_INDENT_STEP}
        onClear={clearPendingCreate}
      />
    ))

  const toggle = () => toggleRowKeepingRows(folder.id, tree.index, activeWorkspaceId)

  return (
    <div>
      <div
        className={ROW_INDENT_TRANSITION}
        style={{ marginInlineStart: (depth + 1) * ROW_INDENT_STEP }}
      >
        <div
          role="treeitem"
          // One tab stop for the whole tree; the roving 0 is set in
          // use-tree-keyboard.ts.
          tabIndex={-1}
          aria-expanded={expanded}
          aria-selected={isSelected}
          {...(!isRenaming
            ? dropRowProps({
                kind: 'folder',
                id: folder.id,
                repoId,
                parentId: containerId,
                label: folder.name,
                expanded,
                hasChildren,
              })
            : {})}
          className={cn(
            ROW_BASE,
            isSelected ? ROW_ACTIVE : ROW_INACTIVE,
            'group',
            draggingIds.has(folder.id) && 'opacity-40',
            dropMode === 'into' && ROW_NEST_TARGET,
          )}
          onClick={(e) => {
            if (isRenaming) return
            if (handleRowSelectionClick(e, folder.id, tree.index)) return
            toggle()
          }}
          onKeyDown={(e) => {
            if (e.target !== e.currentTarget) return
            if (e.key === 'Enter' || e.key === ' ') {
              e.preventDefault()
              toggle()
            }
          }}
          onPointerDown={
            !isRenaming
              ? (e) =>
                  onPointerDownDrag(
                    { kind: 'folder', id: folder.id, repoId, parentId: containerId },
                    e,
                  )
              : undefined
          }
        >
          {/* Open and closed are two glyphs, swapped, rather than one glyph
              animating between them. At 16px an open folder is a few pixels of
              shear away from a closed one, so a tween spends 200ms saying
              something the swap says instantly — and the swapped pair is drawn
              at the same weight as the branch glyph this row interleaves with,
              which a filled or tinted mark of our own could not be. No
              strokeWidth override: the default IS the sidebar's weight (see
              workspace-row-base.ts).

              The holding dots ride INSIDE the glyph rather than beside it,
              because they are a state of the folder, not a second mark. They
              only ever appear on the closed one — a row can hold rows only
              while it is folded. */}
          <span className={ROW_GLYPH_BOX}>
            {expanded ? (
              <FolderOpen aria-hidden="true" className="size-4" />
            ) : (
              <Folder aria-hidden="true" className="size-4">
                {held.holding && (
                  <g data-holding-rows="" fill="currentColor" stroke="none">
                    <circle cx="8" cy="13.5" r="1.1" />
                    <circle cx="12" cy="13.5" r="1.1" />
                    <circle cx="16" cy="13.5" r="1.1" />
                  </g>
                )}
              </Folder>
            )}
          </span>

          {/* A folder is never locked, so it always offers the rename a branch
              row offers — the same editor, opened from the same gesture and
              committed through the same `renamingId`. Anything else would be a
              second rename path in a tree that already has one.

              Like the branch row's name (see workspace-tree-item.tsx), this span
              lets its clicks bubble: `dblclick` is delivered only AFTER both of
              its `click` events, so the folder collapses and re-expands on the
              way to the editor and lands back where it started. The alternative
              is holding every row click open for a double-click window, which
              would put a stall in front of the sidebar's most-used gesture to
              pay for its rarest. */}
          {isRenaming ? (
            <WorkspaceInlineInput
              defaultValue={folder.name}
              placeholder="folder-name"
              // A folder name is free text, not a ref git has to accept, so the
              // editor reads in the same face as the label it replaced. Leaving
              // it on the default would change face mid-rename.
              kind="prose"
              onConfirm={confirmRename}
              onCancel={cancelRename}
            />
          ) : (
            /* Sans, not the branch rows' mono. Mono earns its place on a branch
               name, which is a git ref the user has to read character-exactly —
               slashes, hyphens and digits all count. A folder name is prose
               someone typed, so it reads better in the UI face, and the change of
               face is itself the cheapest signal that this row is not a branch.
               600 against the row's 500 for the same reason. */
            <span
              className="min-w-0 flex-1 truncate text-left font-sans font-semibold tracking-[0.005em]"
              onDoubleClick={(e) => {
                e.stopPropagation()
                startRenaming(folder.id)
              }}
            >
              {folder.name}
            </span>
          )}

          {held.holding && (
            <FoldAwayButton
              label={folder.name}
              onFold={() => foldAwayRows(folder.id, tree.index)}
            />
          )}

          <button
            type="button"
            className={ROW_SUB_ACTION_HOVER}
            aria-label="Add child workspace"
            onClick={(e) => {
              e.stopPropagation()
              if (isCollapsed) toggle()
              startCreating(repoId, folder.id)
            }}
            onPointerDown={(e) => e.stopPropagation()}
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

          {hasChildren && (
            <button
              type="button"
              className={ROW_SUB_ACTION}
              aria-label={expanded ? 'Collapse' : 'Expand'}
              onClick={(e) => {
                e.stopPropagation()
                toggle()
              }}
              onPointerDown={(e) => e.stopPropagation()}
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
          )}
        </div>
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

      <CollapseSection
        open={expanded && (hasChildren || isCreatingChild || pendingCreateRows.length > 0)}
        role="group"
      >
        {children.map((child) => (
          <SidebarNode
            key={child.id}
            node={child}
            depth={depth + 1}
            repoId={repoId}
            projectId={projectId}
            containerId={folder.id}
            activeWorkspaceId={activeWorkspaceId}
            onWorkspaceClick={onWorkspaceClick}
            tree={tree}
          />
        ))}

        {pendingCreateRows}

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
