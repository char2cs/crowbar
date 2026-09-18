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
import { toast } from '@/features/window/stores/toast-store'
import { retryProvision } from '@/lib/api/workspace'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

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

/**
 * `SidebarRow`'s trailing action cluster — repo-menu, Remove, Thread, Fork,
 * close, fold, left to right (Remove renders only when repo-menu doesn't;
 * that menu carries its own "Delete Repo" instead) — split out because which
 * buttons show varies entirely by `row.kind` and which handler props the
 * caller passed, with none of it touching the glyph/label rendering above it
 * in the row.
 */
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
  // The two remedies a worktree-less branch row's own copy names
  // (`placeholderReason`). They are read straight from the stores/API here,
  // like `placeholder-toast-watcher.tsx`'s Fix… already is, rather than
  // threaded as two more handler props through every tree that draws a row —
  // neither verb needs anything the tree knows.
  const openDetach = useDetachModalStore((s) => s.open)
  // Single source of truth for "does this row get the tree-dots menu" — used
  // both to gate that button and to gate Remove's fold-into-it below, so the
  // two can never drift apart the way two separately-written copies of
  // `isProjectHome && row.repoIcon` could.
  const showRepoMenu = isProjectHome && Boolean(row.repoIcon)
  return (
    <>
      {/* A holder means Detach… is the only verb that works: RetryProvision
          refuses a held branch outright (ErrBranchStillHeld), which is why the
          row's own copy promised Retry/detach and the row offered neither.
          Gated on `heldByPath` rather than `needsProvisioning`: the repo's OWN
          checkout holding its own default branch raises no alarm, but it is
          detachable all the same, with consent (spec §3.5). */}
      {row.kind === 'branch' && row.workspaceId && row.heldByPath && (
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
      {row.kind === 'branch' && row.workspaceId && row.isPlaceholder && !row.heldByPath && (
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
            void retryProvision(wsId).catch((err: unknown) => {
              toast.show({
                message: `Couldn't set up ${row.branchName || row.label}`,
                description: err instanceof Error ? err.message : String(err),
                type: 'error',
                key: `retry-provision-${wsId}`,
              })
            })
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <ArrowClockwise aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}

      {/* The repo-home row's own overflow, first in the cluster (the fold
          button is last — everything else sits between the two). Opens the
          SAME menu a right-click on this row opens (row-context-menu.tsx,
          its own `data-control="repo-menu"` capture-phase listener on
          `treeRef`) — explicit user correction: this used to be a second,
          separate one-item menu (just "Delete Repo") that drifted out of
          sync with the right-click menu's own Rename/Import
          branches/New folder. `row.repoIcon` gates it the same way the icon
          swap above does: absent until the repo's project has seeded. */}
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

      {/* Spec §9: "every row that owns something carries a trash: chats,
          workspaces, folders, repos, and the space header for the
          project." A locked branch and the repo's own project-home row
          are the two `handleTrash` itself refuses (space-content-actions.ts's
          own doc) — surfacing a toast rather than pretending to succeed —
          so those are excluded here rather than offered a dead click. Same
          token+glyph Recents' own close button uses (recents-band.tsx),
          not a hard-coded destructive-red trash icon. Calls the identical
          `handleTrash` the (removed) drag-to-trash gesture used to.

          Gated on `!showRepoMenu`, not `!isProjectHome`: a row that gets the
          tree-dots menu gets its removal verb IN that menu ("Delete Repo",
          row-context-menu.tsx) instead of a second, standalone button — one
          delete affordance per row, never two. */}
      {onTrash &&
        !showRepoMenu &&
        (row.kind === 'chat' ||
          row.kind === 'folder' ||
          (row.kind === 'branch' && !row.locked)) && (
          <button
            type="button"
            data-control="remove"
            className={subActionClass}
            aria-label={`Remove ${row.label}`}
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

      {/* Trailing cluster order, explicit product spec: repo-menu (tree
          dots), Remove, Thread, Branch (Fork), Dropdown (fold) — left to
          right, whichever apply; an absent button leaves no gap, it is
          plain conditional JSX, not a reserved slot.

          Addendum §1 (revises spec §3.1): Fork and Thread are two separate
          buttons, not one contextual "+" that picked between them off
          `row.ownsWorktree`. A FOLDER gets BOTH now, each gated exactly the
          way its parent context already gates it for every other row kind
          — a folder applies "the same logic as its parent," not a rule of
          its own:

            - Fork: `row.ownsWorktree` (`rows-from-repo.ts`: true under a
              real repo; `rows-from-home.ts`: always false — no repo means
              no worktree to clone), identical to a `branch` row's own gate.
            - Thread: no extra gate at all, identical to every OTHER row
              kind here (a `branch` row gets Thread even when `locked`) —
              `handleCreate` resolves the folder's nearest owning workspace
              itself (repo-scoped: the closest ancestor branch, or the
              repo's own home; project-home: the project's home workspace,
              always) rather than the folder needing to know which.

          A `chat` row (a thread/bubble) never gets Fork at all — explicit
          product correction: a thread is not itself a branch, so it must
          not be allowed to mint one as a child. Only `branch` (always) and
          `folder` (when `ownsWorktree`) can fork.

          Both verbs additionally need a worktree on disk to run in, which a
          PLACEHOLDER row has none of (`isPlaceholder` — `localPath: null`,
          lib/workspace/placeholder.ts). The create 201'd anyway and opened an
          ordinary-looking pane whose every read then 500'd ("catalog worktree
          is invalid") with nothing surfaced — so a row with no worktree offers
          no verb that needs one, the same way the trash button below excludes
          the kinds `handleTrash` refuses rather than offering a dead click.
          Detach/Retry above are the verbs such a row does get. */}
      {onCreate && !row.isPlaceholder && (
        <button
          type="button"
          data-control="thread"
          className={subActionClass}
          aria-label={`Thread ${row.label}`}
          onClick={(e) => {
            e.stopPropagation()
            // `ROW_SUB_ACTION_HOVER` shows this cluster on `group-focus-within`
            // too (for a keyboard user tabbing to it) — a mouse click leaves
            // the button genuinely `:focus`ed with no visible ring
            // (`:focus-visible` suppresses that for a pointer click, but
            // `:focus-within` still matches plain `:focus`), so without this
            // the whole cluster stayed lit long after the pointer moved on.
            e.currentTarget.blur()
            onCreate(row.id, 'thread')
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <ArrowElbowDownRight aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}

      {onCreate &&
        !row.isPlaceholder &&
        (row.kind === 'folder' ? row.ownsWorktree : row.kind === 'branch') && (
          <button
            type="button"
            data-control="fork"
            className={subActionClass}
            aria-label={`Fork ${row.label}`}
            onClick={(e) => {
              e.stopPropagation()
              // See the Thread button's own comment above — same stuck-focus fix.
              e.currentTarget.blur()
              onCreate(row.id, 'workspace')
            }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            {/* Rule 8: mints a CHILD chat session with its OWN workspace
              forked from this row's branch — a git operation, not a generic
              "+" — so it draws the same `GitBranch` mark `RowGlyph` already
              uses for a row that owns a worktree (`weight="bold"`, matching
              the thread button's own weight rather than `"fill"`, which
              reads too heavy at this size next to it). */}
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
          className={subActionClass}
          aria-label={`${expanded ? 'Collapse' : 'Expand'} ${row.label}`}
          onClick={(e) => {
            e.stopPropagation()
            // See the Fork button's own comment above — same stuck-focus fix.
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
