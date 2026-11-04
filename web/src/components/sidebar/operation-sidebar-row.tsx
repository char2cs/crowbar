import { useNavigate } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import {
  CREATE_ROW_PLACEHOLDER,
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_INDENT_TRANSITION,
} from '@/components/layout/workspace-row-base'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { cancelCreateDraft, confirmCreateDraft } from '@/components/layout/create-actions'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { RowGlyph } from '@/components/sidebar/sidebar-row-glyph'
import {
  rowBoxStyle,
  rowGlyphStyle,
  rowIndent,
  useRowMetrics,
} from '@/components/sidebar/lib/row-metrics'
import { dismissOperation } from '@/core/entities/run-operation'
import { report } from '@/lib/errors/report'

function DraftNameInput({ row }: { row: SidebarRowType }) {
  const navigate = useNavigate()
  return (
    <InlineRenameInput
      defaultValue=""
      mono={row.kind === 'branch'}
      // Only a fork is named; without a placeholder the input read as a chat box.
      placeholder={CREATE_ROW_PLACEHOLDER}
      onConfirm={(name) => confirmCreateDraft(name, navigate)}
      onCancel={cancelCreateDraft}
    />
  )
}

/**
 * A create the sidebar draws before its entity exists, at the slot it lands
 * in: the naming input of a draft, a spinner while its operation runs, or the
 * reason it failed with a Dismiss.
 */
export function OperationSidebarRow({ row, depth }: { row: SidebarRowType; depth: number }) {
  const operation = row.operation
  const isFailed = operation?.state === 'failed'
  const reason = operation?.problem?.message
  const metrics = useRowMetrics()
  return (
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: rowIndent(depth, metrics) }}>
      <div
        className={cn(
          ROW_BASE,
          'border-transparent',
          !row.draft && 'pointer-events-none opacity-60',
        )}
        style={rowBoxStyle(metrics)}
      >
        <span
          className={cn(ROW_GLYPH_BOX, metrics && '[&_svg]:size-full')}
          style={rowGlyphStyle(metrics)}
        >
          {isFailed ? (
            <span
              aria-hidden="true"
              className="flex size-4 shrink-0 items-center justify-center text-xs text-destructive"
            >
              ✕
            </span>
          ) : operation ? (
            <FlickerSpinner className="size-3.5" />
          ) : (
            <RowGlyph row={row} large={false} expanded={false} />
          )}
        </span>

        {row.draft ? (
          <DraftNameInput row={row} />
        ) : (
          <span className="min-w-0 flex-1 truncate font-mono text-[13px] text-muted-foreground">
            {row.kind === 'branch' ? row.label || 'New workspace' : 'New thread'}
          </span>
        )}

        {isFailed && operation && (
          <>
            <span className="pointer-events-auto shrink-0 text-xs text-destructive" title={reason}>
              failed
            </span>
            {reason && (
              <span
                className="pointer-events-auto min-w-0 max-w-[50%] truncate text-xs text-muted-foreground"
                title={reason}
              >
                {reason}
              </span>
            )}
            <button
              type="button"
              aria-label="Dismiss"
              className="pointer-events-auto ml-1 text-xs text-muted-foreground hover:text-foreground"
              onClick={() =>
                void dismissOperation(operation.opId).catch((err: unknown) =>
                  report(err, { action: 'Dismiss' }),
                )
              }
            >
              ✕
            </button>
          </>
        )}
      </div>
    </div>
  )
}
