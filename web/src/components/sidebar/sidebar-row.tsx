import { cn } from '@/lib/utils'
import {
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_HAS_VIEW_IDLE,
  ROW_INACTIVE,
  ROW_INDENT_TRANSITION,
  ROW_NEST_TARGET,
  ROW_SUB_ACTION_HOVER,
  ROW_SUB_ACTION_INVERTED,
  ROW_SUBLABEL,
  ROW_SUBLABEL_ADD,
  ROW_SUBLABEL_DEL,
} from '@/components/layout/workspace-row-base'
import { formatChangeCount } from '@/components/layout/format-change-count'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { performRenameRow } from '@/components/sidebar/lib/row-actions'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { treeUiStore, useTreeUi } from '@/features/workspace/tree/tree-ui-store'
import { SidebarRowActions } from '@/components/sidebar/sidebar-row-actions'
import { SidebarRowGlyph } from '@/components/sidebar/sidebar-row-glyph'
import { OperationSidebarRow } from '@/components/sidebar/operation-sidebar-row'
import { RemovingSidebarRow } from '@/components/sidebar/sidebar-row-removing'
import { useChatField } from '@/features/agent/hooks/use-chat-entity'
import {
  rowBoxStyle,
  rowGlyphStyle,
  rowIndent,
  rowSubStyle,
  rowTitleStyle,
  useRowMetrics,
  type RowMetrics,
} from '@/components/sidebar/lib/row-metrics'

interface SidebarRowProps {
  row: SidebarRowType
  /** Tree depth for the indent step. 0 for a Recents entry — no indent there (spec §5.1). */
  depth: number
  onOpen: (id: string) => void
  /** An explicit close (X) beside drag-to-trash; calls the same trash handler, so a refusal
   *  (locked branch, repo home) shows the same toast. */
  onTrash?: (id: string) => void
  /** Recents' "end this view" control — the opposite of `onTrash`, so never set on the same
   *  row. Hidden and out of flow until hover, like the other trailing actions, so it reserves
   *  no margin when unseen. */
  onClose?: (id: string) => void
  onCreate?: (id: string, kind: 'workspace' | 'thread') => void
  onToggleFold?: (id: string) => void
  folded?: boolean
  /** Spread from useSidebarDrag's row DOM props; an unset optional field is omitted, not ''. */
  dragProps?: Record<string, string | undefined>
  /** This row is one of the rows currently in the air. */
  isDragging?: boolean
  /** A drop here would land INSIDE this row — fills instead of the hairline between rows
   *  ("two signals, never both"). */
  isNestTarget?: boolean
  /** Arms a press-and-hold-to-drag on this row. */
  onPointerDownDrag?: (e: React.PointerEvent) => void
  /** Marks Recents' mirror of a row the tree may also render under the same id. The rename
   *  store picks one instance when the edit starts: the tree copy if mounted, else this one —
   *  the store's `renamingRole`, not this flag alone, decides which draws the editor. */
  inlineRenameDisabled?: boolean
  /** Tree render path only: a row with `hasView` gets the idle ground instead of a muted label,
   *  since an open row should read as present. Recents leaves it unset — a ground there would
   *  outrank the ROW_ACTIVE entry beside it; the tree never draws ROW_ACTIVE. */
  hasViewIdle?: boolean
  /** The row sits on an inverted ROW_ACTIVE ground (a showing Recents row or set member). The
   *  default text tokens track the ambient theme and would be illegible there, so label and
   *  sublabel swap to `text-foreground-inverse`. Never set by the tree. */
  activeGround?: boolean
  /** Drawn inside a view-tab shell that owns margin and padding: `tab` drops ROW_BASE's margin
   *  and takes `h-8`; `tab-member` also drops the resting glyph (the spinner still shows). */
  tab?: 'tab' | 'tab-member'
}

// The one row every tree and Recents entry renders: `[glyph][label] [Fork][Thread][chevron]`.
// Dumb: no active state; each trailing control renders only when its handler is supplied. An
// unlocked branch row owning a workspace adds a second line: branch name and change counts.
export function SidebarRow({
  row,
  depth,
  onOpen,
  onTrash,
  onClose,
  onCreate,
  onToggleFold,
  folded,
  dragProps,
  isDragging,
  isNestTarget,
  onPointerDownDrag,
  inlineRenameDisabled,
  hasViewIdle,
  activeGround,
  tab,
}: SidebarRowProps) {
  // Hooks run before the early returns: a row's operation/removal state flips between renders of
  // the same instance, which would change the hook count. The selector includes `renamingRole`
  // so only the instance the store chose answers true (see `inlineRenameDisabled`).
  const metrics = useRowMetrics()
  const isThisRowRenaming = useTreeUi(
    (s) =>
      s.renaming === row.id &&
      (inlineRenameDisabled ? s.renamingRole === 'recents' : s.renamingRole !== 'recents'),
  )
  // A chat dropped into another workspace mid-turn stays put until its turn ends; null: no move.
  const pendingMove = useChatField(row.id, (c) =>
    c?.pendingMove ? (c.pendingMove.label ?? '') : null,
  )
  // A create whose entity has not arrived (or is still being named) has no
  // chat/workspace behind it, so none of the interactive state below applies.
  if (row.operation || row.draft) {
    return <OperationSidebarRow row={row} depth={depth} />
  }
  // A real row held in the removal tray draws its countdown/undo in place rather than hiding;
  // it never coexists with `row.operation`.
  if (row.removal) {
    return <RemovingSidebarRow row={row} depth={depth} removal={row.removal} />
  }
  // The repo header is the `branch` row carrying `repoIcon` (deriveRows stamps exactly one per
  // repo) — not `parentId === null`, since a header filed into a home folder keeps a parent.
  // Recents' mirror never counts: it is not where the header's icon and menu are edited.
  const isProjectHome = row.kind === 'branch' && row.repoIcon !== undefined && !inlineRenameDisabled
  const expanded = !folded
  // Double-click and the context menu's Rename both start this row's turn in the rename store;
  // the role check already ran in `isThisRowRenaming`'s selector.
  const renaming = isThisRowRenaming
  // An unlocked branch row owning a workspace shows its owning chat's title, with branch name and
  // counts on a second line. A locked branch, the repo header and an unowned workspace keep one
  // line: their label IS the branch name, which the last check catches.
  const showBranchSecondLine =
    !tab &&
    row.kind === 'branch' &&
    row.repoIcon === undefined &&
    !row.locked &&
    !!row.branchName &&
    row.label !== row.branchName
  // An inverted ROW_ACTIVE ground re-keys the trailing buttons too (see ROW_SUB_ACTION_INVERTED).
  const subActionClass = cn(ROW_SUB_ACTION_HOVER, activeGround && ROW_SUB_ACTION_INVERTED)
  // A header drawn where its icon can't be edited (the phone's list).
  const isReadOnlyHeader = !isProjectHome && row.kind === 'branch' && !!row.repoIcon

  return (
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: rowIndent(depth, metrics) }}>
      <div
        // A view tab is one of the strip's tabs; `hasView` marks a view that is open but not showing.
        role={tab ? 'tab' : 'treeitem'}
        aria-level={tab ? undefined : depth + 1}
        aria-selected={tab ? !row.hasView : undefined}
        aria-label={row.label}
        tabIndex={0}
        data-sidebar-row-id={row.id}
        {...dragProps}
        style={rowBoxStyle(metrics)}
        className={cn(
          ROW_BASE,
          isNestTarget ? ROW_NEST_TARGET : ROW_INACTIVE,
          // Only ROW_INACTIVE's text color: the fill is the ancestor's ROW_ACTIVE ground.
          !isNestTarget && activeGround && 'text-foreground-inverse',
          !isNestTarget && hasViewIdle && row.hasView && ROW_HAS_VIEW_IDLE,
          isDragging && 'opacity-40',
          'group',
          tab && 'mx-0 my-0',
          // The shell's own inset plus this gives the pill the same 6px at the
          // edges as above and below its single line.
          tab && 'h-8 px-1',
        )}
        onClick={(e) => {
          // A click inside the inline editor (or on the space it just
          // vacated before React re-renders) must not open the row.
          if (renaming) return
          // A pointer click leaves this tabIndex=0 row `:focus`ed with no ring, which keeps
          // `group-focus-within:` actions lit until something else takes focus; blur it.
          e.currentTarget.blur()
          onOpen(row.id)
        }}
        onPointerDown={renaming ? undefined : onPointerDownDrag}
        onKeyDown={(e) => {
          if (e.target !== e.currentTarget) return
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onOpen(row.id)
          }
        }}
      >
        {/* The only signal of ownership (spec §3.1); work in flight swaps it
            for the spinner in place, never beside it (§3.2). A split's tab
            members carry no resting glyph. */}
        {(tab !== 'tab-member' || row.working) && (
          <span
            className={cn(
              ROW_GLYPH_BOX,
              isProjectHome && 'size-5',
              metrics && '[&_svg]:size-full',
              metrics && isReadOnlyHeader && '[&>*]:size-full',
            )}
            style={rowGlyphStyle(metrics)}
          >
            <SidebarRowGlyph
              row={row}
              isProjectHome={isProjectHome}
              readOnlyHeader={isReadOnlyHeader}
              expanded={expanded}
              activeGround={activeGround}
            />
          </span>
        )}

        {renaming ? (
          <InlineRenameInput
            defaultValue={row.label}
            mono={row.kind === 'branch' && !showBranchSecondLine && !tab}
            onConfirm={(name) => {
              treeUiStore.getState().stopRenaming()
              if (name !== row.label) void performRenameRow(row.id, name)
            }}
            onCancel={() => treeUiStore.getState().stopRenaming()}
          />
        ) : showBranchSecondLine ? (
          // A column, so counts sit under the title and it keeps the row's full width; the two
          // leadings (16px + 13px) fit inside ROW_BASE's fixed 36px height.
          <span
            data-sidebar-row-label=""
            className={cn(
              'flex min-w-0 flex-1 flex-col justify-center',
              // Grey only where nothing else says "open": not with the idle ground (hasViewIdle)
              // and never on an inverted ground (activeGround).
              row.hasView && !hasViewIdle && !activeGround && 'text-muted-foreground',
            )}
          >
            <span
              className={cn('truncate', row.labelProvisional && 'italic')}
              style={rowTitleStyle(metrics)}
            >
              {row.label}
            </span>
            <BranchSecondLine row={row} activeGround={activeGround} metrics={metrics} />
          </span>
        ) : pendingMove !== null && !tab ? (
          <span data-sidebar-row-label="" className="flex min-w-0 flex-1 flex-col justify-center">
            <span className="truncate" style={rowTitleStyle(metrics)}>
              {row.label}
            </span>
            <span
              className={cn(ROW_SUBLABEL, activeGround && 'text-foreground-inverse')}
              style={rowSubStyle(metrics)}
            >
              {pendingMove ? `Moving to ${pendingMove} after this turn` : 'Moving after this turn'}
            </span>
          </span>
        ) : (
          <span
            // Delegation marker for double-click-to-rename: the listener sits on an ancestor and
            // must tell the label from trailing controls, which don't stop a bubbling `dblclick`.
            data-sidebar-row-label=""
            className={cn(
              'min-w-0 flex-1 truncate',
              row.kind === 'branch' && !tab && 'font-mono',
              row.labelProvisional && 'italic',
              // A row with a view is grey (§3.2) unless the idle ground or an inverted ground
              // already carries that signal.
              row.hasView && !hasViewIdle && !activeGround && 'text-muted-foreground',
            )}
          >
            {row.label}
          </span>
        )}

        <SidebarRowActions
          row={row}
          isProjectHome={isProjectHome}
          expanded={expanded}
          subActionClass={subActionClass}
          onCreate={onCreate}
          onTrash={onTrash}
          onClose={onClose}
          onToggleFold={onToggleFold}
        />
      </div>
    </div>
  )
}

// A branch row's second line: `branchName -- +added -deleted`, muted except the green/red counts.
function BranchSecondLine({
  row,
  activeGround,
  metrics,
}: {
  row: SidebarRowType
  metrics: RowMetrics | null
  /** Swaps ROW_SUBLABEL's ambient muted text on an inverted ground, as the label does. */
  activeGround?: boolean
}) {
  const added = row.added ?? 0
  const deleted = row.deleted ?? 0
  return (
    <span
      className={cn(ROW_SUBLABEL, activeGround && 'text-foreground-inverse')}
      style={rowSubStyle(metrics)}
    >
      {row.branchName}
      {(added > 0 || deleted > 0) && ' '}
      {added > 0 && <span className={ROW_SUBLABEL_ADD}>+{formatChangeCount(added)}</span>}
      {added > 0 && deleted > 0 && ' '}
      {deleted > 0 && <span className={ROW_SUBLABEL_DEL}>-{formatChangeCount(deleted)}</span>}
    </span>
  )
}
