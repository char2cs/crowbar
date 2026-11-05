import { cn } from '@/lib/utils'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import {
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_INACTIVE,
  ROW_INDENT_TRANSITION,
  ROW_SUB_ACTION,
} from '@/components/layout/workspace-row-base'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { RowGlyph } from '@/components/sidebar/sidebar-row-glyph'
import { secondsLeft } from '@/components/layout/removal-clock'
import {
  rowBoxStyle,
  rowGlyphStyle,
  rowHitStyle,
  rowIndent,
  useRowMetrics,
} from '@/components/sidebar/lib/row-metrics'

/**
 * A row held in the removal tray, drawn in place: its glyph and label, with
 * the countdown, Keep and the "+N" count where its actions were.
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
      {/* A held row keeps its id so its children and the context menu still
          resolve it; the drag attributes stay off, it is no drop target. */}
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
        {/* From the deadline, not a tick: `removal-tray.tsx`'s shared clock
            does the counting outside React. */}
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
            treeUiStore.getState().cancel(removal.entryId)
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
        {/* `data-essential-motion` keeps this under prefers-reduced-motion: it shows how long is
            left before a delete, not decoration. */}
        <span
          data-removal-drain=""
          data-essential-motion=""
          className="pointer-events-none absolute inset-x-2 bottom-[3px] h-0.5 origin-left rounded-full bg-destructive animate-tray-drain"
        />
      </div>
    </div>
  )
}
