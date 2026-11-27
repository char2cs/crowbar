import {
  ArrowClockwise,
  ArrowElbowDownRight,
  DotsThree,
  GitBranch,
  LinkBreak,
  X,
} from '@phosphor-icons/react'
import { DISCLOSURE_GLYPH_PATH } from '@/components/layout/workspace-row-base'
import { cn } from '@/lib/utils'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { retryProvision } from '@/lib/api/workspace'
import { runOperation } from '@/core/entities/run-operation'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { rowButtonVerbs } from '@/components/sidebar/lib/row-menu'
import { rowHitStyle, useRowMetrics } from '@/components/sidebar/lib/row-metrics'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'

interface SidebarRowActionsProps {
  row: SidebarRowType
  isProjectHome: boolean
  expanded: boolean
  movePending?: boolean
  subActionClass: string
  onCreate?: (id: string, kind: 'workspace' | 'thread') => void
  onTrash?: (id: string) => void
  onClose?: (id: string) => void
  onToggleFold?: (id: string) => void
}

export function SidebarRowActions({
  row,
  isProjectHome,
  expanded,
  movePending = false,
  subActionClass,
  onCreate,
  onTrash,
  onClose,
  onToggleFold,
}: SidebarRowActionsProps) {
  const openDetach = useDetachModalStore((s) => s.open)
  const metrics = useRowMetrics()
  const showRepoMenu = isProjectHome && Boolean(row.repoIcon)
  const verbs = new Set(rowButtonVerbs(row, isProjectHome, movePending))
  return (
    <>
      {verbs.has('detach') && (
        <button
          type="button"
          data-control="detach"
          className={subActionClass}
          aria-label={`Detach ${row.branchName || row.label}`}
          title={row.placeholderReason}
          onClick={(e) => {
            e.stopPropagation()
            e.currentTarget.blur()
            openDetach({
              wsId: row.workspaceId ?? '',
              branch: row.branchName || row.label,
              heldByPath: row.heldByPath ?? '',
            })
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <LinkBreak aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}

      {verbs.has('retry') && (
        <button
          type="button"
          data-control="retry-provision"
          className={subActionClass}
          aria-label={`Retry ${row.branchName || row.label}`}
          title={row.placeholderReason}
          onClick={(e) => {
            e.stopPropagation()
            e.currentTarget.blur()
            const wsId = row.workspaceId
            if (!wsId) return
            runOperation(
              {
                kind: 'workspace.retry_provision',
                subject: { kind: 'workspace', id: wsId },
                projectId: '',
                label: `Retry ${row.branchName || row.label}`,
              },
              (opId) => retryProvision(wsId, opId),
            )
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <ArrowClockwise aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}

      {showRepoMenu && (
        <button
          type="button"
          data-control="repo-menu"
          className={subActionClass}
          aria-label={`More actions for ${row.label}`}
        >
          <DotsThree aria-hidden="true" className="size-3.5" weight="bold" />
        </button>
      )}

      {onTrash && verbs.has('remove') && (
        <button
          type="button"
          data-control="remove"
          className={subActionClass}
          aria-label={removeLabel(row)}
          title={removeLabel(row)}
          onClick={(e) => {
            e.stopPropagation()
            e.currentTarget.blur()
            onTrash(row.id)
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <X aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}

      {onCreate && verbs.has('new-thread') && (
        <button
          type="button"
          data-control="thread"
          className={subActionClass}
          aria-label={`Thread ${row.label}`}
          onClick={(e) => {
            e.stopPropagation()
            e.currentTarget.blur()
            onCreate(row.id, 'thread')
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <ArrowElbowDownRight aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}

      {onCreate && verbs.has('fork') && (
        <button
          type="button"
          data-control="fork"
          className={subActionClass}
          aria-label={`Fork ${row.label}`}
          onClick={(e) => {
            e.stopPropagation()
            e.currentTarget.blur()
            onCreate(row.id, 'workspace')
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <GitBranch aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}

      {verbs.has('branch-out') && (
        <button
          type="button"
          data-control="branch-out"
          className={subActionClass}
          aria-label="Make this thread a branch"
          title="Make this thread a branch"
          onClick={(e) => {
            e.stopPropagation()
            e.currentTarget.blur()
            treeUiStore.getState().startBranching(row.id)
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <GitBranch aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}

      {onClose && (
        <button
          type="button"
          data-control="close"
          className={subActionClass}
          aria-label={`Close ${row.label}`}
          onClick={(e) => {
            e.stopPropagation()
            e.currentTarget.blur()
            onClose(row.id)
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <X aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}

      {onToggleFold && (
        <button
          type="button"
          data-control="fold"
          className={
            metrics
              ? 'relative inline-flex shrink-0 items-center justify-center text-muted-foreground focus-visible:outline-none'
              : subActionClass
          }
          style={rowHitStyle(metrics)}
          aria-label={`${expanded ? 'Collapse' : 'Expand'} ${row.label}`}
          aria-expanded={expanded}
          onClick={(e) => {
            e.stopPropagation()
            e.currentTarget.blur()
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
    </>
  )
}

function removeLabel(row: SidebarRowType): string {
  if (row.kind === 'branch' && row.ownsWorktree && row.branchName)
    return `Remove branch ${row.branchName} and its worktree`
  return `Remove ${row.label}`
}
