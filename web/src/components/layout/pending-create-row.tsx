import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { WorkspaceAgentSpinner } from './workspace-branch-icon'
import type { PendingCreate } from './workspace-tree-context'

interface PendingCreateRowProps {
  tempId: string
  pending: PendingCreate
  /** Left padding (px) so the optimistic row sits at its parent's tree depth. */
  paddingLeft: number
  onClear: (tempId: string) => void
}

/**
 * The optimistic row shown while a workspace create is in flight: a spinner plus
 * the branch name, or — if the create failed — an inline error with a dismiss
 * button. Rendered both at the repo root (for default-parent creates, in
 * workspace-tree) and nested under a parent workspace (for child creates, in
 * workspace-tree-item), so every create shows its loading animation regardless
 * of where it forks from.
 */
export function PendingCreateRow({ tempId, pending, paddingLeft, onClear }: PendingCreateRowProps) {
  return (
    // The outer padding div carries no id of its own: it is a plain
    // positioning wrapper (native/mapping/layout-denominator.md's `depth *
    // 14` indentation), and the inner row below already reports its own
    // absolute origin including that offset — a second anchor here would not
    // tell the differ anything the row's own bounds do not already carry.
    <div style={{ paddingLeft }}>
      <div
        className={cn(ROW_BASE, 'border-transparent opacity-60 pointer-events-none')}
        data-oracle-id="pending-create-row"
      >
        {pending.error ? (
          <>
            {/* Shares one id with the spinner branch below — the two are
                mutually exclusive pictures in the same slot, the same
                shape workspace-branch-icon.tsx's own seven branches share
                `workspace-branch-icon` under. */}
            <span
              className="flex size-4 shrink-0 items-center justify-center text-xs text-destructive"
              data-oracle-id="pending-create-row-icon"
            >
              ✕
            </span>
            <span
              className="min-w-0 flex-1 truncate font-mono text-[13px] text-muted-foreground"
              data-oracle-id="pending-create-row-label"
              data-oracle-line-sized="true"
            >
              {pending.branch}
            </span>
            <span className="text-xs text-destructive" data-oracle-id="pending-create-row-status">
              failed
            </span>
            <button
              type="button"
              className="pointer-events-auto ml-1 text-xs text-muted-foreground hover:text-foreground"
              onClick={() => onClear(tempId)}
              data-oracle-id="pending-create-row-dismiss"
            >
              ✕
            </button>
          </>
        ) : (
          <>
            <WorkspaceAgentSpinner />
            <span
              className="min-w-0 flex-1 truncate font-mono text-[13px] text-muted-foreground"
              data-oracle-id="pending-create-row-label"
              data-oracle-line-sized="true"
            >
              {pending.branch}
            </span>
          </>
        )}
      </div>
    </div>
  )
}
