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

interface SidebarRowActionsProps {
  row: SidebarRowType
  isProjectHome: boolean
  expanded: boolean
  subActionClass: string
  onCreate?: (id: string, kind: 'workspace' | 'thread') => void
  onTrash?: (id: string) => void
  onClose?: (id: string) => void
  onToggleFold?: (id: string) => void
}

// SidebarRow's trailing cluster, left to right: repo-menu, Remove, Thread, Fork, close, fold —
// whichever the row earns (row-menu.ts). Remove never shows beside repo-menu, which carries Delete
// Repo; an absent button leaves no gap.
export function SidebarRowActions({
  row,
  isProjectHome,
  expanded,
  subActionClass,
  onCreate,
  onTrash,
  onClose,
  onToggleFold,
}: SidebarRowActionsProps) {
  // The two remedies a worktree-less row's copy names, read straight from the stores/API rather
  // than threaded as handler props: neither needs anything the tree knows.
  const openDetach = useDetachModalStore((s) => s.open)
  const metrics = useRowMetrics()
  // One source for "does this row get the tree-dots menu", gating both that button and Remove.
  const showRepoMenu = isProjectHome && Boolean(row.repoIcon)
  // Which verbs the row earns is decided once, in row-menu.ts, for the phone's menu too.
  const verbs = new Set(rowButtonVerbs(row, isProjectHome))
  return (
    <>
      {/* A holder makes Detach… the only working verb (RetryProvision refuses a held branch).
          Gated on `heldByPath`, not `needsProvisioning`: the repo's own checkout raises no alarm
          but is detachable all the same, with consent (spec §3.5). */}
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

      {/* No holder and still no worktree: the provision itself failed, and
          Retry is the verb the copy names for exactly this case. */}
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
            // A failure is the operations watcher's to toast and the row's to explain.
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

      {/* The repo header's overflow: the same menu a right-click opens (row-context-menu.tsx's
          capture-phase listener). Absent until the repo's project seeds `row.repoIcon`. */}
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

      {/* Every row that owns something carries a trash (spec §9), in Recents' close token, not a
          red icon. Not on a locked branch (handleTrash refuses it), nor beside the repo menu,
          which carries Delete Repo — one delete affordance per row. */}
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

      {/* Fork and Thread are separate verbs (addendum §1); a folder follows its parent context's
          gates and a chat never forks. Neither shows on a placeholder row: with no worktree on
          disk the create succeeds but every read in the new pane fails. */}
      {onCreate && verbs.has('new-thread') && (
        <button
          type="button"
          data-control="thread"
          className={subActionClass}
          aria-label={`Thread ${row.label}`}
          onClick={(e) => {
            e.stopPropagation()
            // The cluster also shows on `group-focus-within`, and a click leaves the button
            // `:focus`ed with no ring, keeping the cluster lit; blur it.
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
            // Same stuck-focus fix as the Thread button.
            e.currentTarget.blur()
            onCreate(row.id, 'workspace')
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          {/* Forking is a git operation, so it draws RowGlyph's GitBranch mark (bold, matching
              Thread's weight; "fill" reads too heavy here). */}
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
          // No hover on a phone: the disclosure stays drawn, with a full-size tap target.
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
            // Same stuck-focus fix as the Thread button.
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

// A branch row is titled by its owning chat, but removing it deletes the branch's worktree.
function removeLabel(row: SidebarRowType): string {
  if (row.kind === 'branch' && row.ownsWorktree && row.branchName)
    return `Remove branch ${row.branchName} and its worktree`
  return `Remove ${row.label}`
}
