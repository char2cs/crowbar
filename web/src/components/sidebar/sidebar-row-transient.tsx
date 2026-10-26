import { cn } from '@/lib/utils'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { secondsLeft } from '@/components/layout/removal-clock'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import {
  CREATE_ROW_PLACEHOLDER,
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_INACTIVE,
  ROW_INDENT_TRANSITION,
  ROW_SUB_ACTION,
} from '@/components/layout/workspace-row-base'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { confirmPendingCreateName, cancelPendingCreate } from '@/components/layout/create-actions'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { RowGlyph } from './row-glyph'
import {
  rowBoxStyle,
  rowGlyphStyle,
  rowHitStyle,
  rowIndent,
  useRowMetrics,
} from './lib/row-metrics'

/**
 * A create still in flight, drawn at the exact tree slot the finished create
 * lands in (pending-creates.ts) — a naming input, a spinner, or an inline
 * error with a dismiss, matching the pre-migration tree's own
 * `PendingCreateRow` (workspace-tree-context.tsx/pending-create-row.tsx,
 * since deleted) in spirit: dimmed and non-interactive except while naming,
 * which needs a real focused input.
 */
export function PendingSidebarRow({
  row,
  depth,
  pending,
}: {
  row: SidebarRowType
  depth: number
  pending: NonNullable<SidebarRowType['pending']>
}) {
  const metrics = useRowMetrics()
  const isNaming = pending.status === 'naming'
  const isError = pending.status === 'error'
  return (
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: rowIndent(depth, metrics) }}>
      <div
        className={cn(
          ROW_BASE,
          'border-transparent',
          !isNaming && 'pointer-events-none opacity-60',
        )}
        style={rowBoxStyle(metrics)}
      >
        <span
          className={cn(ROW_GLYPH_BOX, metrics && '[&_svg]:size-full')}
          style={rowGlyphStyle(metrics)}
        >
          {isError ? (
            <span
              aria-hidden="true"
              className="flex size-4 shrink-0 items-center justify-center text-xs text-destructive"
            >
              ✕
            </span>
          ) : pending.status === 'creating' ? (
            <FlickerSpinner className="size-3.5" />
          ) : (
            <RowGlyph row={row} large={false} expanded={false} />
          )}
        </span>

        {isNaming ? (
          <InlineRenameInput
            defaultValue=""
            mono={row.kind === 'branch'}
            // Naming is reached ONLY for a fork (pending-creates.ts's own
            // doc: a thread has nothing to name, so it skips straight to
            // 'creating') — an unlabeled empty input read as a chat box to
            // type INTO rather than a name to give something, so what was
            // typed there became the branch's name — caught live.
            placeholder={CREATE_ROW_PLACEHOLDER}
            onConfirm={(name) => confirmPendingCreateName(pending.tempId, name)}
            onCancel={() => cancelPendingCreate(pending.tempId)}
          />
        ) : (
          <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-muted-foreground">
            {row.kind === 'branch' ? row.label || 'New workspace' : 'New thread'}
          </span>
        )}

        {isError && (
          <>
            <span
              className="pointer-events-auto shrink-0 text-xs text-destructive"
              title={pending.error}
            >
              failed
            </span>
            {pending.error && (
              <span
                className="pointer-events-auto min-w-0 max-w-[50%] truncate text-xs text-muted-foreground"
                title={pending.error}
              >
                {pending.error}
              </span>
            )}
            <button
              type="button"
              aria-label="Dismiss"
              className="pointer-events-auto ml-1 text-xs text-muted-foreground hover:text-foreground"
              onClick={() => cancelPendingCreate(pending.tempId)}
            >
              ✕
            </button>
          </>
        )}
      </div>
    </div>
  )
}

/**
 * A REAL row currently held in the removal tray, transformed in place —
 * explicit product request: an X on the row starts this instead of hiding
 * the row while a separate `RemovalTray` shows the same thing elsewhere.
 *
 * Same glyph and label the row wears at rest (a held row has not become a
 * different kind of thing — `removal-tray.tsx`'s own module doc, still true
 * here), with the trailing cluster replaced by the countdown, an undo
 * ("Keep") control, and the "+N" count of what goes with it. The seconds
 * figure and the drain hairline are the exact same DOM contract
 * `removal-tray.tsx` already draws from (`data-removal-secs`,
 * `data-removal-drain`) — that file's own shared clock ticks BOTH, now
 * scoped to the whole document rather than one tray, so N rows scattered
 * across the tree still cost exactly one timer.
 */
export function RemovingSidebarRow({
  row,
  depth,
  removal,
}: {
  row: SidebarRowType
  depth: number
  removal: NonNullable<SidebarRowType['removal']>
}) {
  const metrics = useRowMetrics()
  return (
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: rowIndent(depth, metrics) }}>
      {/* `data-sidebar-row-id` is the row's identity, not one of its
          affordances, and a held row is still on screen with its children
          still drawn under it — dropping the attribute made a held FOLDER
          read as a row that does not exist, leaving its children parented to
          an id nothing published (and taking the right-click menu with it:
          `row-context-menu.tsx` resolves a row by this attribute alone). The
          drag attributes stay off: a row on its way out is deliberately not a
          drop target. */}
      <div
        data-sidebar-row-id={row.id}
        className={cn(ROW_BASE, ROW_INACTIVE, 'relative')}
        style={rowBoxStyle(metrics)}
      >
        <span
          className={cn(ROW_GLYPH_BOX, metrics && '[&_svg]:size-full')}
          style={rowGlyphStyle(metrics)}
        >
          <RowGlyph row={row} large={false} expanded={false} />
        </span>
        <span
          className={cn(
            'min-w-0 flex-1 truncate',
            row.kind === 'branch' && 'font-mono',
            row.labelProvisional && 'italic',
          )}
        >
          {row.label}
        </span>
        {removal.extra > 0 && (
          <span className="shrink-0 rounded-full bg-sidebar-element-idle px-1.5 py-px text-[11px] tabular-nums text-muted-foreground">
            +{removal.extra}
          </span>
        )}
        {/* Rendered from the deadline, not from a tick — a row that
            re-renders for some other reason still shows the truth; the
            counting itself happens in `removal-tray.tsx`'s shared clock and
            never comes back through React (see this function's own doc). */}
        <span
          data-removal-secs={removal.entryId}
          className="shrink-0 text-[11px] tabular-nums text-muted-foreground"
        >
          {removal.deadlineAt === null ? 0 : secondsLeft(removal.deadlineAt, Date.now())}
        </span>
        <button
          type="button"
          className={cn(ROW_SUB_ACTION, metrics && 'items-center justify-center')}
          style={rowHitStyle(metrics)}
          aria-label={`Keep ${row.label}`}
          title="Keep"
          onClick={(e) => {
            e.stopPropagation()
            useRemovalTrayStore.getState().cancel(removal.entryId)
          }}
        >
          <svg
            aria-hidden="true"
            className="size-3"
            viewBox="0 0 16 16"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="M6 3L2.5 6.5 6 10M2.5 6.5H10a3.5 3.5 0 0 1 0 7H7" />
          </svg>
        </button>
        {/* `data-essential-motion` keeps this under prefers-reduced-motion:
            not decoration, it is how long is left before something is
            deleted — same reasoning as `removal-tray.tsx`'s own hairline. */}
        <span
          data-removal-drain=""
          data-essential-motion=""
          className="pointer-events-none absolute inset-x-2 bottom-[3px] h-0.5 origin-left rounded-full bg-destructive animate-tray-drain"
        />
      </div>
    </div>
  )
}
