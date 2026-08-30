import { ArrowElbowDownRight, ChatsCircle, Folder, GitBranch, Trash } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import {
  ADD_GLYPH_PATH,
  DISCLOSURE_GLYPH_PATH,
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_INACTIVE,
  ROW_INDENT_STEP,
  ROW_INDENT_TRANSITION,
  ROW_NEST_TARGET,
  ROW_SUB_ACTION_HOVER,
} from '@/components/layout/workspace-row-base'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

interface SidebarRowProps {
  row: SidebarRowType
  /** Tree depth for the indent step. 0 for a Recents entry — no indent there (spec §5.1). */
  depth: number
  onOpen: (id: string) => void
  onTrash?: (id: string) => void
  onCreate?: (id: string, kind: 'workspace' | 'thread') => void
  onToggleFold?: (id: string) => void
  folded?: boolean
  /** Spread from drop-dom's createDropRowDom (Task 21's useSidebarDrag) — its
   *  own `props()` omits an unset optional field rather than writing it as
   *  '', so the value side of this is `string | undefined`. */
  dragProps?: Record<string, string | undefined>
  /** This row is one of the rows currently in the air (Task 21). */
  isDragging?: boolean
  /** A drop here would land INSIDE this row (Task 21) — fills instead of the
   *  hairline drawn between rows, spec's "two signals, never both". */
  isNestTarget?: boolean
  /** Arms a press-and-hold-to-drag on this row (Task 21's `useSidebarDrag`). */
  onPointerDownDrag?: (e: React.PointerEvent) => void
}

/**
 * The one row every tree and every Recents entry renders through — spec §3.1:
 * `[glyph][label] [trash][+][chevron]`. Replaces the markup that used to be
 * hand-rolled per row-kind across two separate tree implementations.
 *
 * Deliberately dumb: no selected/active state (§3.2 retires the tree's raised
 * ROW_ACTIVE surface — that concept moved to Recents' own "is-active" shell),
 * no second line ever (§3.3), and each trailing control renders only when its
 * handler prop is supplied, so a caller opts into exactly the affordances a
 * given row needs.
 */
export function SidebarRow({
  row,
  depth,
  onOpen,
  onTrash,
  onCreate,
  onToggleFold,
  folded,
  dragProps,
  isDragging,
  isNestTarget,
  onPointerDownDrag,
}: SidebarRowProps) {
  // The project-home row is `branch` with no parent — the sidebar's one 20px
  // glyph exception outside the project header itself (spec §3.1).
  const isProjectHome = row.kind === 'branch' && row.parentId === null
  const expanded = !folded
  // §3.1: "+" makes a workspace on a row that is itself git-capable, a thread
  // otherwise. `ownsWorktree` is the only fact this row carries that answers it.
  const createKind: 'workspace' | 'thread' = row.ownsWorktree ? 'workspace' : 'thread'

  return (
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: depth * ROW_INDENT_STEP }}>
      <div
        role="treeitem"
        tabIndex={0}
        data-sidebar-row-id={row.id}
        {...dragProps}
        className={cn(
          ROW_BASE,
          isNestTarget ? ROW_NEST_TARGET : ROW_INACTIVE,
          isDragging && 'opacity-40',
          'group pr-2.5',
        )}
        onClick={() => onOpen(row.id)}
        onPointerDown={onPointerDownDrag}
        onKeyDown={(e) => {
          if (e.target !== e.currentTarget) return
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onOpen(row.id)
          }
        }}
      >
        {/* The only signal of ownership (spec §3.1): a git mark for a row that
            owns a worktree, a chat bubble for one that borrows its parent's,
            a folder mark for pure organisation. `working` swaps it for the
            flip-dot spinner IN PLACE — never beside it (§3.2). */}
        <span className={cn(ROW_GLYPH_BOX, isProjectHome && 'size-5')}>
          {row.working ? (
            <FlickerSpinner className="size-3.5" />
          ) : (
            <RowGlyph row={row} large={isProjectHome} />
          )}
        </span>

        <span
          className={cn(
            'min-w-0 flex-1 truncate',
            row.kind === 'branch' && 'font-mono',
            row.labelProvisional && 'italic',
            // A row with a view is grey — focused or not (§3.2). The mark above
            // keeps full strength either way.
            row.hasView && 'text-muted-foreground',
          )}
        >
          {row.label}
        </span>

        {onTrash && (
          <button
            type="button"
            data-control="trash"
            // Leads the trailing cluster and takes the deny tint on hover, the
            // moment before the click is unambiguous (spec §9).
            className={cn(ROW_SUB_ACTION_HOVER, 'hover:bg-destructive/10 hover:text-destructive')}
            aria-label={`Delete ${row.label}`}
            onClick={(e) => {
              e.stopPropagation()
              onTrash(row.id)
            }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            <Trash aria-hidden="true" className="size-3" weight="bold" />
          </button>
        )}

        {onCreate && (
          <button
            type="button"
            data-control="create"
            className={ROW_SUB_ACTION_HOVER}
            aria-label={
              createKind === 'workspace'
                ? `New workspace under ${row.label}`
                : `New thread in ${row.label}`
            }
            onClick={(e) => {
              e.stopPropagation()
              onCreate(row.id, createKind)
            }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            {createKind === 'workspace' ? (
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
            ) : (
              <ArrowElbowDownRight aria-hidden="true" className="size-3" weight="bold" />
            )}
          </button>
        )}

        {onToggleFold && (
          <button
            type="button"
            data-control="fold"
            className={ROW_SUB_ACTION_HOVER}
            aria-label={`${expanded ? 'Collapse' : 'Expand'} ${row.label}`}
            onClick={(e) => {
              e.stopPropagation()
              onToggleFold(row.id)
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
              <path d={DISCLOSURE_GLYPH_PATH} />
            </svg>
          </button>
        )}
      </div>
    </div>
  )
}

function RowGlyph({ row, large }: { row: SidebarRowType; large: boolean }) {
  const size = large ? 'size-5' : 'size-4'
  if (row.kind === 'folder') {
    return <Folder aria-hidden="true" className={size} weight="duotone" />
  }
  if (row.ownsWorktree) {
    return <GitBranch aria-hidden="true" className={size} weight="fill" />
  }
  return <ChatsCircle aria-hidden="true" className={size} weight="regular" />
}
