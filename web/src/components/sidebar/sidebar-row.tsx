import {
  ArrowElbowDownRight,
  ChatsCircle,
  Folder,
  FolderOpen,
  GitBranch,
  Lock,
  X,
} from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import {
  CREATE_ROW_PLACEHOLDER,
  DISCLOSURE_GLYPH_PATH,
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_INACTIVE,
  ROW_INDENT_STEP,
  ROW_INDENT_TRANSITION,
  ROW_NEST_TARGET,
  ROW_SUB_ACTION,
  ROW_SUB_ACTION_HOVER,
  ROW_SUBLABEL,
  ROW_SUBLABEL_ADD,
  ROW_SUBLABEL_DEL,
} from '@/components/layout/workspace-row-base'
import { formatChangeCount } from '@/components/layout/format-change-count'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import { performPromoteChat, performRenameRow } from '@/components/sidebar/lib/row-actions'
import {
  confirmPendingCreateName,
  cancelPendingCreate,
} from '@/components/layout/space-content-actions'
import { EditableRepoIcon } from '@/components/layout/repo-icon-mark'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { useSidebarInlineRenameStore } from '@/lib/store/sidebar-inline-rename'

interface SidebarRowProps {
  row: SidebarRowType
  /** Tree depth for the indent step. 0 for a Recents entry — no indent there (spec §5.1). */
  depth: number
  onOpen: (id: string) => void
  /** Explicit product request: an X close button (Recents' own — same
   *  `ROW_SUB_ACTION_HOVER` token and `X` glyph, see the trailing cluster's
   *  own comment) instead of the drag-to-trash gesture being the ONLY way
   *  to remove a row. Calls the exact same `handleTrash` the drag gesture
   *  already did (`sidebar-tree-surface.tsx`'s `onTrash`) — a refusal (a
   *  locked branch, a repo home) surfaces the same toast either way. */
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
 * and each trailing control renders only when its handler prop is supplied,
 * so a caller opts into exactly the affordances a given row needs.
 *
 * §3.3 used to read "no second line ever" — retired by the product rule that
 * unified workspaces and chats into one row model: a `branch` row that owns a
 * real (unlocked) workspace now draws a second line under its label with that
 * workspace's branch name and change counts (rule 6), the way the retired
 * workspace tree's own `WorkspaceRowLabel` did.
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
  inlineRenameDisabled,
}: SidebarRowProps) {
  // Read UNCONDITIONALLY, before either early return below — rules of hooks:
  // a row's `pending`/`removal` state can flip between renders of the SAME
  // component instance (a real row enters the removal tray and later leaves
  // it, without remounting — its id, and so its key, never changes), so a
  // hook called only on the branch that falls through past both checks would
  // vary the hook count render to render for that one instance. Harmless to
  // run early: the two returns below render an entirely different component,
  // which never reads this value at all.
  const isThisRowRenaming = useSidebarInlineRenameStore((s) => s.renamingRowId === row.id)
  // A create still in flight (pending-creates.ts) draws through this SAME row
  // shape rather than a separate placeholder component, at the exact slot
  // the finished create lands in — none of the interactive state below (open,
  // drag, rename, promote, fork/thread affordances) applies to a row with no
  // real chat/workspace behind it yet, so this returns before touching any
  // of it.
  if (row.pending) {
    return <PendingSidebarRow row={row} depth={depth} pending={row.pending} />
  }
  // A REAL row currently held in the removal tray (`removal-plan.ts`'s
  // `attachRemovalState`) draws its countdown/undo IN PLACE instead of the
  // row hiding while a separate tray shows the same thing elsewhere —
  // explicit product request. Same early-return shape as `row.pending`
  // above; the two never coexist (a pending row's id is a synthetic tempId
  // no removal entry could ever name).
  if (row.removal) {
    return <RemovingSidebarRow row={row} depth={depth} removal={row.removal} />
  }
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
  // `canFork !== false` too: a bubble's cwd walk "always terminates at a real
  // worktree ancestor" above is exactly untrue for a project-home chat — home
  // rides no repo at all, so there is no worktree for a promote to attach to
  // either (same reason Fork hides for one, canFork's own doc). Without this,
  // "Make workspace" on a home bubble was offered and silently did nothing —
  // `performPromoteChat` finds no owning repo for a chat id no repo's `chats`
  // ever lists, and returns before the request even goes out.
  const promotable =
    row.kind === 'chat' && !row.ownsWorktree && !row.working && row.canFork !== false
  // Double-click-to-rename (sidebar-tree-chrome.tsx's delegated `dblclick`
  // listener) starts this row's turn in `sidebar-inline-rename.ts`'s store —
  // real inline editing in place, matching `develop`, not the modal Task 4
  // wrongly opened. A narrow selector: this row only cares whether IT is the
  // one renaming, not who else might be (`isThisRowRenaming` itself, read
  // unconditionally at the top of this function — see that hook call's own
  // doc). `inlineRenameDisabled` (see its own doc above) keeps a second
  // same-id instance — Recents mirroring a live pane — from ALSO answering
  // yes and fighting the tree row for focus.
  const renaming = !inlineRenameDisabled && isThisRowRenaming
  // Rule 6: a `branch` row that owns a real, unlocked workspace draws its
  // OWNING CHAT's title on the label line now (`rows-from-repo.ts`'s own
  // `label`/`branchName` split), with the branch name and change counts moved
  // to a second line beneath it. A locked branch, the project-home row, and a
  // genuinely chat-less workspace (no owner resolved yet) all keep the single
  // branch/repo-name line instead (addendum rules 1-4: "Folder mechanism" for
  // the first two — unchanged) — every one of those is exactly the case where
  // `label` IS `branchName`, which is what the last check below catches
  // without a field of its own: a second line would just repeat the label.
  const showBranchSecondLine =
    row.kind === 'branch' &&
    !isProjectHome &&
    !row.locked &&
    !!row.branchName &&
    row.label !== row.branchName

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
              <RowGlyph row={row} large={false} expanded={expanded} />
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
              <RowGlyph row={row} large={isProjectHome} expanded={expanded} />
            )}
          </span>
        )}

        {renaming ? (
          <InlineRenameInput
            defaultValue={row.label}
            mono={row.kind === 'branch' && !showBranchSecondLine}
            onConfirm={(name) => {
              useSidebarInlineRenameStore.getState().stopRenaming()
              if (name !== row.label) void performRenameRow(row.id, name)
            }}
            onCancel={() => useSidebarInlineRenameStore.getState().stopRenaming()}
          />
        ) : showBranchSecondLine ? (
          // A flex COLUMN, not a row (matches the retired workspace tree's own
          // `WorkspaceRowLabel`) — the counts sit UNDER the title, not beside
          // it, so the title keeps the row's full width. ROW_BASE's `h-9` is a
          // fixed 36px and the two leadings below (16px + 13px) are sized to
          // fit inside it without growing the row.
          <span
            data-sidebar-row-label=""
            className={cn(
              'flex min-w-0 flex-1 flex-col justify-center',
              row.hasView && 'text-muted-foreground',
            )}
          >
            <span className={cn('truncate', row.labelProvisional && 'italic')}>{row.label}</span>
            <BranchSecondLine row={row} />
          </span>
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

        {/* Trailing cluster order, explicit product spec: Thread, Branch
            (Fork), remove (X), Dropdown (fold) — left to right.

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
            `folder` (when `ownsWorktree`) can fork. */}
        {onCreate && (
          <button
            type="button"
            data-control="thread"
            className={ROW_SUB_ACTION_HOVER}
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
          (row.kind === 'folder' ? row.ownsWorktree : row.kind === 'branch') && (
          <button
            type="button"
            data-control="fork"
            className={ROW_SUB_ACTION_HOVER}
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

        {/* Spec §9: "every row that owns something carries a trash: chats,
            workspaces, folders, repos, and the space header for the
            project." A locked branch and the repo's own project-home row
            are the two `handleTrash` itself refuses (space-content-actions.ts's
            own doc) — surfacing a toast rather than pretending to succeed —
            so those are excluded here rather than offered a dead click. Same
            token+glyph Recents' own close button uses (recents-band.tsx),
            not a hard-coded destructive-red trash icon. Calls the identical
            `handleTrash` the (removed) drag-to-trash gesture used to. */}
        {onTrash &&
          !isProjectHome &&
          (row.kind === 'chat' ||
            row.kind === 'folder' ||
            (row.kind === 'branch' && !row.locked)) && (
          <button
            type="button"
            data-control="remove"
            className={ROW_SUB_ACTION_HOVER}
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

        {onToggleFold && (
          <button
            type="button"
            data-control="fold"
            className={ROW_SUB_ACTION_HOVER}
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
      </div>
    </div>
  )
}

/**
 * A create still in flight, drawn at the exact tree slot the finished create
 * lands in (pending-creates.ts) — a naming input, a spinner, or an inline
 * error with a dismiss, matching the pre-migration tree's own
 * `PendingCreateRow` (workspace-tree-context.tsx/pending-create-row.tsx,
 * since deleted) in spirit: dimmed and non-interactive except while naming,
 * which needs a real focused input.
 */
function PendingSidebarRow({
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
        className={cn(ROW_BASE, 'border-transparent', !isNaming && 'pointer-events-none opacity-60')}
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
            <span className="text-xs text-destructive">failed</span>
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

/** Whole seconds left on a deadline, never below zero — same formula
 *  `removal-tray.tsx` draws its own figures from. */
function secondsLeft(deadlineAt: number | null): number {
  if (deadlineAt === null) return 0
  return Math.max(0, Math.ceil((deadlineAt - Date.now()) / 1000))
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
function RemovingSidebarRow({
  row,
  depth,
  removal,
}: {
  row: SidebarRowType
  depth: number
  removal: NonNullable<SidebarRowType['removal']>
}) {
  return (
    <div className={ROW_INDENT_TRANSITION} style={{ marginInlineStart: depth * ROW_INDENT_STEP }}>
      <div className={cn(ROW_BASE, ROW_INACTIVE, 'relative')}>
        <span className={ROW_GLYPH_BOX}>
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
          {secondsLeft(removal.deadlineAt)}
        </span>
        <button
          type="button"
          className={ROW_SUB_ACTION}
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

/**
 * A `branch` row's second line: `branchName -- +added -deleted` (rule 6).
 * Muted TOKEN throughout except the counts themselves, which keep the
 * green/red they've always had — see `ROW_SUBLABEL`'s own doc on why the line
 * is muted but the counts are not.
 */
function BranchSecondLine({ row }: { row: SidebarRowType }) {
  const added = row.added ?? 0
  const deleted = row.deleted ?? 0
  return (
    <span className={ROW_SUBLABEL}>
      {row.branchName}
      {(added > 0 || deleted > 0) && ' -- '}
      {added > 0 && <span className={ROW_SUBLABEL_ADD}>+{formatChangeCount(added)}</span>}
      {added > 0 && deleted > 0 && ' '}
      {deleted > 0 && <span className={ROW_SUBLABEL_DEL}>-{formatChangeCount(deleted)}</span>}
    </span>
  )
}

function RowGlyph({
  row,
  large,
  expanded,
}: {
  row: SidebarRowType
  large: boolean
  expanded: boolean
}) {
  const size = large ? 'size-5' : 'size-4'
  if (row.kind === 'folder') {
    return expanded ? (
      <FolderOpen aria-hidden="true" className={size} weight="duotone" />
    ) : (
      <Folder aria-hidden="true" className={size} weight="duotone" />
    )
  }
  // A locked/protected branch (the repo/project home, or any other locked
  // branch `rows-from-repo.ts`'s `walk()` mints) draws the Lock mark instead
  // of the plain GitBranch every other worktree-owning row gets.
  // `row.locked` is `rows-from-repo.ts`'s own `Workspace.status === 'locked'`
  // read straight onto the row — every workspace-owning row is now id'd from
  // its owning chat, locked or not, so the id/workspaceId mismatch this used
  // to read off is no longer a signal unique to the locked case.
  // `workspace-branch-icon.tsx`'s own `status === 'locked'` case renders the
  // same glyph for the same fact.
  if (row.kind === 'branch' && row.locked) {
    return <Lock aria-hidden="true" className={size} weight="fill" />
  }
  if (row.ownsWorktree) {
    return <GitBranch aria-hidden="true" className={size} weight="fill" />
  }
  return <ChatsCircle aria-hidden="true" className={size} weight="regular" />
}
