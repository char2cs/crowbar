import { ArrowElbowDownRight, ChatsCircle, Folder, GitBranch, Lock } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
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
import { performPromoteChat, performRenameRow } from '@/components/sidebar/lib/row-actions'
import { EditableRepoIcon } from '@/components/layout/repo-icon-mark'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { useSidebarInlineRenameStore } from '@/lib/store/sidebar-inline-rename'

interface SidebarRowProps {
  row: SidebarRowType
  /** Tree depth for the indent step. 0 for a Recents entry — no indent there (spec §5.1). */
  depth: number
  onOpen: (id: string) => void
  /** Addendum §1/§4: the row no longer carries a trash button — deleting moved
   *  to the drag-to-trash gesture on the file explorer card. Kept only in the
   *  prop type (never read below) because `sidebar-tree.tsx` still threads a
   *  handler down to every row it renders; dropping the field here would be a
   *  type error at that call site, which is outside this fix's file list. */
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
  /** A chat that is the live pane (`row.hasView`) renders through TWO
   *  `SidebarRow` instances at once — its own tree row, and a second one
   *  `RecentsMemberRow` builds for the same chat id (§5: "what is up").
   *  `sidebar-inline-rename.ts`'s store is keyed only by row id, with no
   *  notion of which DOM instance is "the" one being edited — so without
   *  this, double-clicking either one flips BOTH into rename mode. Two
   *  `InlineRenameInput`s then mount, the second one's own focus+select
   *  effect steals focus from the first, and that unhandled blur commits
   *  (matching develop — see `inline-rename-input.tsx`'s `handleBlur`) with
   *  the unchanged value, cancelling the rename before it's ever visible.
   *  Recents already renders `SidebarRow` with reduced affordances of its
   *  own (no trash, no create, no fold — see `recents-band.tsx`), so opting
   *  its instance out of inline-rename here rather than teaching the store
   *  which instance "wins" keeps the tree as the one place a chat's name is
   *  actually edited. */
  inlineRenameDisabled?: boolean
}

/**
 * The one row every tree and every Recents entry renders through — spec §3.1
 * as revised by the addendum §1: `[glyph][label] [Fork][Thread][chevron]`.
 * Replaces the markup that used to be hand-rolled per row-kind across two
 * separate tree implementations.
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
  onCreate,
  onToggleFold,
  folded,
  dragProps,
  isDragging,
  isNestTarget,
  onPointerDownDrag,
  inlineRenameDisabled,
}: SidebarRowProps) {
  // The project-home row is `branch` with no parent — the sidebar's one 20px
  // glyph exception outside the project header itself (spec §3.1), and also
  // the one row spec §9 calls a protected branch: "the repo's own ground …
  // not workspaces you made". It's the only row this shape can occur on
  // (rows-from-repo.ts gives exactly one row a null parentId, the repo's
  // default worktree).
  const isProjectHome = row.kind === 'branch' && row.parentId === null
  const expanded = !folded
  // §3.5/§4.2: any bubble (no worktree of its own) that isn't currently
  // working can promote itself into one, straight from its own glyph — a
  // bubble's cwd walk always terminates at a real worktree ancestor by
  // construction, so there's no separate "is a parent available" check.
  // Gated purely on the row's own fields, unlike the trailing cluster below,
  // which only renders when a caller opts in with a handler prop: a working
  // row does not move (§4.3), and the backend's own promote.go respawns the
  // chat's CLI regardless of whether it is mid-turn, so refusing here up
  // front is what keeps a click from round-tripping into a confusing error.
  const promotable = row.kind === 'chat' && !row.ownsWorktree && !row.working
  // Double-click-to-rename (sidebar-tree-chrome.tsx's delegated `dblclick`
  // listener) starts this row's turn in `sidebar-inline-rename.ts`'s store —
  // real inline editing in place, matching `develop`, not the modal Task 4
  // wrongly opened. A narrow selector: this row only cares whether IT is the
  // one renaming, not who else might be. `inlineRenameDisabled` (see its own
  // doc above) keeps a second same-id instance — Recents mirroring a live
  // pane — from ALSO answering yes and fighting the tree row for focus.
  const isThisRowRenaming = useSidebarInlineRenameStore((s) => s.renamingRowId === row.id)
  const renaming = !inlineRenameDisabled && isThisRowRenaming

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
        onClick={() => {
          // A click inside the inline editor (or on the space it just
          // vacated before React re-renders) must not open the row.
          if (renaming) return
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
        {/* The only signal of ownership (spec §3.1): a git mark for a row that
            owns a worktree, a chat bubble for one that borrows its parent's,
            a folder mark for pure organisation. `working` swaps it for the
            flip-dot spinner IN PLACE — never beside it (§3.2). A promotable
            bubble's glyph doubles as the one-item "Make workspace" dropdown
            (§3.5) — never for a worktree-owning, working, or non-chat row.
            The project-home row's glyph is a THIRD thing the static
            RowGlyph can't be: the repo's own personalizable icon — clicking
            it (and only it; the click is stopped from reaching the row,
            same as the promote dropdown above) reopens the icon picker the
            tree retirement severed. `repoIcon` is absent until the repo's
            owning project has seeded, in which case this falls back to the
            plain glyph rather than guessing at a REST base it can't build. */}
        {promotable ? (
          <DropdownMenu>
            <DropdownMenuTrigger
              data-testid="promote-dropdown"
              aria-label={`Promote ${row.label} to a workspace`}
              className={cn(
                ROW_GLYPH_BOX,
                'cursor-pointer rounded hover:bg-sidebar-element-hover focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
              )}
              onClick={(e) => e.stopPropagation()}
              onPointerDown={(e) => e.stopPropagation()}
            >
              <RowGlyph row={row} large={false} />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" side="bottom" sideOffset={4}>
              <DropdownMenuItem
                onClick={(e) => {
                  e.stopPropagation()
                  void performPromoteChat(row.id)
                }}
              >
                Make workspace
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        ) : (
          <span className={cn(ROW_GLYPH_BOX, isProjectHome && 'size-5')}>
            {row.working ? (
              <FlickerSpinner className="size-3.5" />
            ) : isProjectHome && row.repoIcon ? (
              <EditableRepoIcon
                repo={row.repoIcon}
                projectId={row.repoIcon.projectId}
                repoId={row.repoIcon.repoId}
                size="lg"
              />
            ) : (
              <RowGlyph row={row} large={isProjectHome} />
            )}
          </span>
        )}

        {renaming ? (
          <InlineRenameInput
            defaultValue={row.label}
            mono={row.kind === 'branch'}
            onConfirm={(name) => {
              useSidebarInlineRenameStore.getState().stopRenaming()
              if (name !== row.label) void performRenameRow(row.id, name)
            }}
            onCancel={() => useSidebarInlineRenameStore.getState().stopRenaming()}
          />
        ) : (
          <span
            // Double-click-to-rename's delegation marker (sidebar-tree-chrome.tsx):
            // that listener sits on an ancestor of every project's rows, so it
            // needs to tell a double-click on the label apart from one on the
            // trailing trash/create/fold controls, which don't stop a bubbling
            // `dblclick` the way they already stop `click`/`pointerdown`.
            data-sidebar-row-label=""
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
        )}

        {/* Addendum §1 (revises spec §3.1): Fork and Thread are two separate,
            always-rendered buttons now, not one contextual "+" that picked
            between them off `row.ownsWorktree`. Both are always legal —
            fork always mints a new workspace whose parent is this row's
            chat, thread always mints a new chat in this row's own
            workspace — so neither is gated on the row's own fields except
            kind: a FOLDER has no owning chat to fork or thread from (it
            groups workspaces, not chats — rows-from-repo.ts), so it gets
            neither button. Its own create action is the nested affordance
            row §3.5 already gives an empty container. The trash button that
            used to lead this cluster is gone entirely (addendum §1/§2):
            deleting is now a drag-to-trash gesture onto the file explorer
            card, built elsewhere. */}
        {onCreate && row.kind !== 'folder' && (
          <button
            type="button"
            data-control="fork"
            className={ROW_SUB_ACTION_HOVER}
            aria-label={`Fork ${row.label}`}
            onClick={(e) => {
              e.stopPropagation()
              onCreate(row.id, 'workspace')
            }}
            onPointerDown={(e) => e.stopPropagation()}
          >
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
          </button>
        )}

        {onCreate && row.kind !== 'folder' && (
          <button
            type="button"
            data-control="thread"
            className={ROW_SUB_ACTION_HOVER}
            aria-label={`Thread ${row.label}`}
            onClick={(e) => {
              e.stopPropagation()
              onCreate(row.id, 'thread')
            }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            <ArrowElbowDownRight aria-hidden="true" className="size-3" weight="bold" />
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
  // A locked/protected branch (the repo/project home, or any other locked
  // branch `rows-from-repo.ts`'s `walk()` mints) is id'd from its OWNING
  // CHAT rather than from its own workspace — see that file's own doc on
  // `branchRowIds` and the `rowId` derivation in `walk()`. An ordinary fork
  // always keeps `id === workspaceId`, so the mismatch is exactly (and only)
  // the locked case, with no extra field needed to carry it here.
  // `workspace-branch-icon.tsx`'s own `status === 'locked'` case renders the
  // same glyph for the same fact.
  if (row.kind === 'branch' && row.id !== row.workspaceId) {
    return <Lock aria-hidden="true" className={size} weight="fill" />
  }
  if (row.ownsWorktree) {
    return <GitBranch aria-hidden="true" className={size} weight="fill" />
  }
  return <ChatsCircle aria-hidden="true" className={size} weight="regular" />
}
