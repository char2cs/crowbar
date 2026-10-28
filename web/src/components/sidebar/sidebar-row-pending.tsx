import { cn } from '@/lib/utils'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import {
  CREATE_ROW_PLACEHOLDER,
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_INDENT_STEP,
  ROW_INDENT_TRANSITION,
} from '@/components/layout/workspace-row-base'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { confirmPendingCreateName, cancelPendingCreate } from '@/components/layout/create-actions'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { RowGlyph } from '@/components/sidebar/sidebar-row-glyph'

/**
 * A create still in flight, at the slot it lands in (pending-creates.ts): a
 * naming input, a spinner, or an inline error with a dismiss.
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
  const isNaming = pending.status === 'naming'
  const isError = pending.status === 'error'
  return (
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: depth * ROW_INDENT_STEP }}>
      <div
        className={cn(
          ROW_BASE,
          'border-transparent',
          !isNaming && 'pointer-events-none opacity-60',
        )}
      >
        <span className={ROW_GLYPH_BOX}>
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
            // Only a fork is named; without a placeholder the input read as a chat box.
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
