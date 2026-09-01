import { useRef } from 'react'
import { X } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { Separator } from '@/components/ui/separator'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { ROW_ACTIVE, ROW_SUB_ACTION_HOVER } from '@/components/layout/workspace-row-base'
import { DragGhost, DragGhostRows } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import {
  useSidebarDrag,
  type SidebarDrag,
  type SidebarPaneZone,
} from '@/components/sidebar/hooks/use-sidebar-drag'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import type { RecentsEntry, RecentsEntryState } from '@/features/panes/types/recents-entry'

// Re-exported for existing importers — the type itself lives in
// features/panes/types/recents-entry.ts so pane-slice.ts (a store) can hold
// `dormantArrangements: RecentsEntry[]` without importing from components/.
export type { RecentsEntry, RecentsEntryState }

/**
 * A `RecentsEntry` tagged with the workspace whose store its chats live in.
 *
 * The band's per-chat lookups used to go through `useWorkspaceStoreContext`
 * (an ambient `WorkspaceStoreContext.Provider`), which only exists inside
 * the mounted `WorkspaceView` subtree — NOT the sidebar, which sits outside
 * it entirely. That worked in this file's own tests only because they mock
 * the hook away; mounted for real (this task), a project's Recents can span
 * more than the active workspace (spec §4: "Recents is per space"), so there
 * is no single ambient store to read from anyway. `useWorkspaceStoreById`
 * (the same registry-by-id mechanism `merge-popover.tsx`/the git sidebar
 * already use for the identical "no per-workspace context mounted here"
 * problem) reads any workspace's store directly, keyed by this tag.
 */
export interface RecentsBandEntry extends RecentsEntry {
  workspaceId: string
  /**
   * The entry's id exactly as `deriveRecentsEntries` produced it, before
   * `recents-for-project.ts` workspace-qualifies `.id` for cross-workspace
   * uniqueness. Pane ids (`ROOT_PANE_ID`/`BOTTOM_PANE_ID`) are module-level
   * constants shared verbatim across EVERY workspace store, so `.id` alone
   * collides once a project's Recents spans more than one workspace
   * (`WorkspaceHost` keeps several retained at once) — `.id` stays globally
   * unique (React keys / `data-testid`), `localId` is what the OWNING
   * workspace store's own `dormantArrangements`/pane ids actually are, for
   * any call (e.g. `paneActions.forgetDormantArrangement`) that must match
   * against real stored state.
   */
  localId: string
}

interface RecentsBandProps {
  entries: RecentsBandEntry[]
  onFocus: (entry: RecentsBandEntry) => void
  /** No control renders for a 'working' entry — nothing calls this for one. */
  onClose: (entry: RecentsBandEntry) => void
  /** The panel's own scroll container — what an edge-held drag scrolls
   *  (Task 21). Shared with `SidebarTree`, since the two sit in one scroll
   *  region per space. */
  scrollRef: React.RefObject<HTMLElement | null>
  onDrop: (subjects: SidebarRowType[], target: SidebarRowType, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRowType[], paneId: string, zone: SidebarPaneZone) => void
}

// The close button sits OUTSIDE SidebarRow's own layout (absolute, over the
// row), so SidebarRow's `pr-2.5` (it has no trailing controls in this usage)
// leaves its `truncate` label free to render right up under it. Same problem
// `tab-bar-item.tsx` already solved for its own external close button —
// `Tab` reserves `pr-8` against a `!size-5` button at `right-1.5` (26px right
// extent, 6px of clearance). Ours is a 24px `ROW_SUB_ACTION_HOVER` button at
// `right-2.5` (34px right extent); `pr-10` (40px) keeps the same 6px margin.
const RECENTS_ROW_CLOSE_RESERVE = 'pr-10'

/**
 * §5: "what is up, and what is running." Every entry renders through
 * `SidebarRow` at `depth={0}` — no indent, no parentage, no chevron, no
 * second line (§5.1).
 *
 * Deliberately does NOT wire `onClose` into `SidebarRow`'s `onTrash` slot.
 * That slot hard-codes a Trash icon, destructive-red hover, and a
 * `Delete ${label}` aria-label — correct for the tree's destroy-the-chat verb,
 * but §5.4's "×" here means the opposite: end this view, never touch the
 * chat. Reusing `onTrash` would render a mislabelled delete affordance for a
 * non-destructive close. RecentsBand instead renders its own close control
 * beside the row, built from the same trailing-action tokens
 * (`ROW_SUB_ACTION_HOVER`) so it stays visually consistent without borrowing
 * the tree's destructive semantics.
 */
export function RecentsBand({
  entries,
  onFocus,
  onClose,
  scrollRef,
  onDrop,
  onPaneDrop,
}: RecentsBandProps) {
  // Every member row constructs its own `SidebarRow` from live chat state at
  // render time (RecentsMemberRow, below) — this is where each one lands so
  // `subjectsFor` below can hand a drag the real thing it grabbed rather than
  // re-deriving it from a chat id.
  const rowsRef = useRef(new Map<string, SidebarRowType>())
  const registerRow = (row: SidebarRowType) => {
    rowsRef.current.set(row.id, row)
  }
  // A depth-0 leaf renderer: no tree structure to publish a real ancestor
  // path from, so the subtree cycle guard is a no-op for a Recents-rendered
  // TARGET (a real gap, narrow and disclosed — see use-sidebar-drag.ts).
  const drag = useSidebarDrag({
    scrollRef,
    subjectsFor: (rowId) => {
      const row = rowsRef.current.get(rowId)
      return row ? [row] : []
    },
    onDrop,
    onPaneDrop,
  })

  // §5.7: no band until something is open — the empty state teaches the
  // tree/Recents pairing for free. After the hook above: hooks run every
  // render regardless of `entries.length`, so this has to follow them.
  if (entries.length === 0) return null

  return (
    <div data-testid="recents-band">
      <div className="flex h-[22px] items-center gap-1.5 px-1.5">
        <Separator className="flex-1" />
        <span className="shrink-0 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          Recents
        </span>
      </div>
      {entries.map((entry) => (
        <RecentsEntryRow
          key={entry.id}
          entry={entry}
          onFocus={onFocus}
          onClose={onClose}
          drag={drag}
          registerRow={registerRow}
        />
      ))}
      {drag.dragging && <DropIndicator ref={drag.attachDropLine} />}
      {drag.ghostRows && (
        <DragGhost ref={drag.ghostRef} origin={drag.ghostOrigin}>
          <DragGhostRows rows={drag.ghostRows} />
        </DragGhost>
      )}
    </div>
  )
}

function RecentsEntryRow({
  entry,
  onFocus,
  onClose,
  drag,
  registerRow,
}: {
  entry: RecentsBandEntry
  onFocus: (entry: RecentsBandEntry) => void
  onClose: (entry: RecentsBandEntry) => void
  drag: SidebarDrag
  registerRow: (row: SidebarRowType) => void
}) {
  // §5.3/§5.6: chatIds.length decides the SHAPE — a shell around 2+ rows, or
  // a bare row for one. `state === 'live'` decides whether that shape is the
  // arrangement on screen right now. A literal 'set' with no 'live' is a
  // remembered (dormant) multi-chat view: the shell still draws — its own
  // ground, unlit — per §5.3 "at rest the shell and every member are empty".
  const isSet = entry.chatIds.length >= 2
  const isLive = entry.state === 'live'
  // §5.4: every row has a close control except the working one. There is
  // nothing left to close, and that absence is the "still running" signal.
  const canClose = entry.state !== 'working'
  // A lone live entry has no one to be grouped WITH — it doesn't need a set's
  // shell (§5.3's padded "ground" exists to hold multiple pills apart), it
  // just needs to BE the active row, pixel-for-pixel the tree's own footprint
  // (§5.2: "exactly as in the tree"). `SidebarRow`'s own `ROW_BASE` already
  // carries the row's real `mx-1.5 my-0.5 h-9` — mirroring the same classes
  // on THIS wrapper too used to stack a second, visible margin/padding on top
  // of it (a live entry rendered ~8px taller and ~8px narrower than a tree
  // row — the bug this file was patched for). The member below cancels
  // `ROW_BASE`'s own margin with an equal negative one exactly when this
  // wrapper is the one taking over that spacing, so the net is applied once.
  const soloActive = !isSet && isLive

  return (
    <div
      className={cn(
        'group relative',
        // A SET's shell is a real container (§5.3): its own ground, radius,
        // and 2px of padding around member rows that each keep their own
        // margin — that's what separates one member's pill from the next.
        // Unlike a solo-active entry (below), the shell div here IS the
        // painted box — there's no separate unstyled wrapper the way
        // SidebarRow's own outer div is for a solo row — so its `mx-1.5
        // my-0.5` is its ONLY source of external gutter, not a redundant
        // copy of anything. Vertical margins between this shell and its
        // siblings collapse regardless of whether the shell states its own
        // `my-0.5` (so height was never at stake either way), but horizontal
        // margins never collapse — dropping `mx-1.5` here (an earlier,
        // wrong pass at this fix) deleted the shell's only left/right
        // gutter and rendered it flush against the sidebar's edges.
        isSet && 'mx-1.5 my-0.5 rounded-xl p-0.5',
        isSet && (isLive ? ROW_ACTIVE : 'bg-sidebar-element-idle'),
        soloActive && cn('mx-1.5 my-0.5 rounded-lg', ROW_ACTIVE),
      )}
      data-testid={isSet ? `recents-set-${entry.id}` : undefined}
    >
      {entry.chatIds.map((chatId) => (
        <RecentsMemberRow
          key={chatId}
          workspaceId={entry.workspaceId}
          chatId={chatId}
          hasView={isLive}
          reserveClose={canClose}
          cancelOwnMargin={soloActive}
          onOpen={() => onFocus(entry)}
          drag={drag}
          registerRow={registerRow}
        />
      ))}
      {canClose && (
        <button
          type="button"
          data-control="close"
          data-testid={`recents-close-${entry.id}`}
          aria-label="Close view"
          className={cn(ROW_SUB_ACTION_HOVER, 'absolute right-2.5 top-1/2 -translate-y-1/2')}
          onClick={(e) => {
            e.stopPropagation()
            onClose(entry)
          }}
          onPointerDown={(e) => e.stopPropagation()}
        >
          <X aria-hidden="true" className="size-3" weight="bold" />
        </button>
      )}
    </div>
  )
}

function RecentsMemberRow({
  workspaceId,
  chatId,
  hasView,
  reserveClose,
  cancelOwnMargin,
  onOpen,
  drag,
  registerRow,
}: {
  /** Which workspace's store owns this chat — see `RecentsBandEntry`. */
  workspaceId: string
  chatId: string
  hasView: boolean
  /** Whether this entry's row(s) sit under an overlaid close control — reserve
   *  room for it (see RECENTS_ROW_CLOSE_RESERVE) so a long title truncates
   *  before it, not before SidebarRow's own narrower built-in inset. */
  reserveClose: boolean
  /** Set only for a lone live entry (`RecentsEntryRow`'s `soloActive`), whose
   *  OWN wrapper takes over `SidebarRow`'s `mx-1.5 my-0.5` to become the row's
   *  one active surface. `SidebarRow` always carries that margin itself too
   *  (shared with the tree, can't opt out per-caller) — left uncancelled here
   *  it would stack a second copy on top of the wrapper's, rendering the row
   *  visibly bigger/narrower than a tree row. A SET's members deliberately
   *  keep their own margin (uncancelled) — that's what separates one
   *  member's pill from the next inside the shell (§5.3). */
  cancelOwnMargin?: boolean
  onOpen: () => void
  drag: SidebarDrag
  registerRow: (row: SidebarRowType) => void
}) {
  const chat = useWorkspaceStoreById(workspaceId, (s) =>
    s.agentChats.chats.find((c) => c.id === chatId),
  )
  // Per-chat, narrow selector (copied verbatim from the tree's own pattern) —
  // the spinner rides the member wherever it lands (§5.6), independent of
  // which of the four band states its entry carries.
  const working = useWorkspaceStoreById(workspaceId, (s) => s.agentChats.working[chatId] ?? false)

  if (!chat) return null

  const row: SidebarRowType = {
    id: chat.id,
    kind: 'chat',
    parentId: null,
    order: 0,
    label: chat.title,
    ownsWorktree: false,
    workspaceId: chat.workspaceId,
    working,
    hasView,
  }
  // So `subjectsFor` can hand a real, freshly-rendered row back to a drag
  // that grabs it — see RecentsBand's own `rowsRef` note above.
  registerRow(row)

  return (
    <div
      data-testid={`recents-row-${chat.id}`}
      className={cn(reserveClose && RECENTS_ROW_CLOSE_RESERVE, cancelOwnMargin && '-mx-1.5 -my-0.5')}
    >
      <SidebarRow
        row={row}
        depth={0}
        onOpen={onOpen}
        dragProps={drag.dragProps(row)}
        isDragging={drag.draggingIds.has(row.id)}
        isNestTarget={drag.nestTargetId === row.id}
        onPointerDownDrag={(e) => drag.onPointerDownDrag(row, e)}
        // A live chat (hasView) renders here AND as its own tree row, sharing
        // one id — see `inlineRenameDisabled`'s own doc in sidebar-row.tsx.
        // Without this, double-click-to-rename flips both instances into
        // rename mode at once; the second one's mount-time focus() steals
        // focus from the first and its unhandled blur commits the (unchanged)
        // value, cancelling the rename before it's ever visible.
        inlineRenameDisabled
      />
    </div>
  )
}
