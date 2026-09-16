import { useRef } from 'react'
import { cn } from '@/lib/utils'
import { Separator } from '@/components/ui/separator'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { useWorkspaceStoreById } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { ROW_ACTIVE } from '@/components/layout/workspace-row-base'
import { DragGhost, DragGhostRows } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import {
  useSidebarDrag,
  type SidebarDrag,
  type SidebarPaneZone,
} from '@/components/sidebar/hooks/use-sidebar-drag'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'
import type { ChatIconFields } from '@/components/sidebar/lib/rows-from-repo'
import type { RecentsEntry, RecentsEntryState } from '@/features/panes/types/recents-entry'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'

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
  /**
   * The entry's OWN primary workspace — kept for callers that need exactly
   * one (`recents-actions.ts`'s `focusRecent`/its route), derived the same
   * way it always was, from the first chat. Rendering a SET's individual
   * members must NOT use this: a SET can span more than one workspace
   * within a project, and every member here used to be drawn against
   * whichever workspace the FIRST chat happened to own — see
   * `chatWorkspaces` below, which is what a member row actually reads.
   */
  workspaceId: string
  /** Each of `chatIds`, mapped to the workspace store that actually owns
   *  it — resolved per chat, never assumed uniform across the entry (a SET
   *  can span workspaces within one project). What `RecentsMemberRow`
   *  reads to find its own chat's data. Optional so a caller with only one
   *  chat (or one that never needed per-member resolution — existing test
   *  fixtures included) can still fall back to `workspaceId` above; the
   *  real producer (`recents-for-project.ts`) always populates it. */
  chatWorkspaces?: Record<string, string>
  /**
   * Icon-relevant `SidebarRow` fields (see `ChatIconFields`) for any of
   * `chatIds` that OWN a workspace — the tree's own branch/lock/PR-status
   * glyph, resolved from the SAME repo data `rows-from-repo.ts`'s
   * `chatIconIndex` walks. A chat absent here owns no workspace, and
   * `RecentsMemberRow` keeps its own default (`kind: 'chat', ownsWorktree:
   * false`, the plain bubble) for it — same optional-with-a-real-producer
   * shape as `chatWorkspaces` above; `recents-for-project.ts` always
   * populates every workspace-owning member.
   *
   * Fixes the bug where a chat that owns a workspace (and so draws the real
   * `WorkspaceBranchIcon`/Lock/GitBranch mark in the tree) rendered as a
   * generic `ChatsCircle` bubble here instead — `RecentsMemberRow` used to
   * hand-build its row with no ownership data to draw on at all.
   */
  chatIcons?: Record<string, ChatIconFields>
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
  /**
   * Feedback: "each group chat row, on hover, should have its closing
   * button [that] removes that one chat from [the group], but doesn't
   * dissolve the group." For a SET this is now the ONLY close control there
   * is — explicit product correction, asked repeatedly: no separate
   * "close everything" button on the shell any more. Closing every member
   * one at a time already gets there; the group dissolves for free once it
   * is down to one (`viewIdOf`'s "group of one" fallback, pane-views.ts),
   * at which point that lone row's own `onClose` (below) takes over. A solo
   * entry's one chat goes straight to `onClose` instead — it has no group to
   * leave a member of.
   */
  onCloseChat: (entry: RecentsBandEntry, chatId: string) => void
}

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
 * non-destructive close. Every row instead wires `SidebarRow`'s own `onClose`
 * prop (its own doc, sidebar-row.tsx) — the SAME trailing-action token every
 * other row control already uses, hidden and out of flow until hovered,
 * costing the row a reflow on hover rather than a permanently reserved
 * margin (the previous design's `pr-10` overlay reserved padding on BOTH the
 * shell and every member for a button only visible on hover, which crushed a
 * 2-up set's labels down to a couple of characters even at rest — reported
 * four times before this unification). There is no separate shell-level
 * "close everything" button any more either (reported three more times after
 * that fix, against the button itself rather than its layout cost) — a SET's
 * only close control is each member's own, same as a solo row's.
 */
export function RecentsBand({
  entries,
  onFocus,
  onClose,
  scrollRef,
  onDrop,
  onPaneDrop,
  onCloseChat,
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
        <Separator className="flex-1 bg-border" />
        <span className="shrink-0 font-mono text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
          Recents
        </span>
      </div>
      {entries.map((entry) => (
        <RecentsEntryRow
          key={entry.id}
          entry={entry}
          onFocus={onFocus}
          onClose={onClose}
          onCloseChat={onCloseChat}
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
  onCloseChat,
  drag,
  registerRow,
}: {
  entry: RecentsBandEntry
  onFocus: (entry: RecentsBandEntry) => void
  onClose: (entry: RecentsBandEntry) => void
  onCloseChat: (entry: RecentsBandEntry, chatId: string) => void
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
  // THE SWITCHER'S "you are here". Several views can be live at once and only
  // one occupies the screen, so `state: 'live'` no longer decides the lit
  // treatment on its own — three steps now: showing (lit), open but off
  // screen (a filled, unlit ground), remembered (nothing). Reusing the SET
  // shell's own idle token for the middle step rather than inventing a
  // fourth surface.
  const isShowing = isLive && entry.showing === true
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
  const soloActive = !isSet && isShowing

  return (
    <div
      data-view-showing={isShowing || undefined}
      className={cn(
        'group relative',
        // A SET's shell is a real container (§5.3): its own ground and radius
        // around member rows that share it via a small `gap` (see below), not
        // their own margin. Unlike a solo-active entry (below), the shell div
        // here IS the painted box — there's no separate unstyled wrapper the
        // way SidebarRow's own outer div is for a solo row — so its `mx-1.5
        // my-0.5` is its ONLY source of external gutter, not a redundant copy
        // of anything. Vertical margins between this shell and its siblings
        // collapse regardless of whether the shell states its own `my-0.5`
        // (so height was never at stake either way), but horizontal margins
        // never collapse — dropping `mx-1.5` here (an earlier, wrong pass at
        // this fix) deleted the shell's only left/right gutter and rendered
        // it flush against the sidebar's edges.
        //
        // `p-0.5 gap-0.5`: the same 2px on every side AND between members —
        // reported live as visibly mismatched when the inter-member gap came
        // from each member's own uncancelled `mx-1.5` instead (12px between
        // members against 2px from the shell's own edge to a member, with a
        // THIRD value, 0, on the vertical edge). One shared value, in one
        // place, is what makes the three actually equal. `gap-x-0.5` is the
        // same 2px the tab strip already uses between its own pills
        // (tabs.tsx) — small, but a real, visible seam, not zero.
        //
        // `rounded-lg`, matching ROW_BASE's own radius (every ordinary and
        // member row) — `rounded-xl` here read as a visibly different corner
        // treatment between a plain row and a grouped one, reported live.
        //
        // `flex` (feedback: "grouped in a single line, not in multiple rows")
        // lays its member rows out SIDE BY SIDE instead of the vertical stack
        // a plain block div gave them.
        //
        // `ROW_ACTIVE`'s own edge is an `outline`, not a `border` — it never
        // participates in layout, so it paints the real 1px CossUI edge here
        // without adding to this shell's own auto height the way a `border`
        // width would (workspace-row-base.ts's own doc on `ROW_ACTIVE`). The
        // `p-0.5` this shell wraps each `h-8` (not the usual `h-9`) member in
        // is what actually keeps the total at a plain row's own height —
        // each member gives up exactly what this padding adds back
        // (SidebarRow's own `compactHeight` doc) — rather than shrinking the
        // padding itself, which was tuned live to match on every side (see
        // above) and would go uneven again if only its vertical half were
        // cut to make room.
        isSet && 'mx-1.5 my-0.5 flex items-center gap-0.5 rounded-lg p-0.5',
        // A SET no longer paints a ground of its own once it stops SHOWING —
        // reported live: an off-screen/dormant set still showed a filled
        // background at rest, with no hover and nothing to justify it. Now it
        // matches the solo row below exactly: bare until it's the one on
        // screen, `hasView` alone carrying the "still open" signal via each
        // member's own greyed label.
        isSet && isShowing && ROW_ACTIVE,
        // A NOT-showing SET's own hover — one shared surface across the whole
        // shell, not each member's own box (user correction, with Zen
        // browser's grouped-tab capsule as the explicit reference: "when
        // hovering a group, the whole group should receive the hover
        // signal"). `group-hover:` fires whenever the pointer is anywhere
        // inside this shell (CSS `:hover` matches an ancestor for the whole
        // time the pointer is over any descendant), so hovering ONE member
        // lights the entire capsule together — each member's own local hover
        // is silenced for exactly this reason (RecentsMemberRow's own doc).
        // Carries the CossUI top-highlight too, by explicit request ("add the
        // CossUI treatment and we're basically there") — the one place a
        // Recents surface gets it besides the showing row itself (ROW_ACTIVE,
        // above): an ordinary tree row's plain hover never does (workspace-
        // row-base.ts's own doc on why that first, too-broad pass was wrong).
        // Skipped while showing: ROW_ACTIVE already owns the shell's fill and
        // its own top-highlight, and layering a second one on top of it would
        // fight rather than add to it.
        // `0_1px` — production's (develop) own plain `--elevated-highlight`
        // offset, unchanged: this surface (`bg-sidebar-element-hover`) is
        // ambient-colored, never inverted, so it never had ROW_ACTIVE's
        // problem (see that token's own doc in theme.css) and needs no
        // departure from the same recipe every button and tab already uses.
        isSet &&
          !isShowing &&
          'group-hover:bg-sidebar-element-hover group-hover:shadow-xs ' +
            'group-hover:shadow-black/10 group-hover:inset-shadow-[0_1px_var(--elevated-highlight)]',
        // `h-9`, matching `ROW_BASE` exactly, rather than an auto height: the
        // member's own margin is suppressed at the source now
        // (`SidebarRow`'s `suppressOwnMargin`, passed below), so this shell
        // IS the row's one real box — sizing it explicitly is what lets
        // `ROW_ACTIVE`'s border (added for CossUI parity, workspace-row-
        // base.ts) sit inside the same 36px every other row has instead of
        // adding to an auto height that only this state renders (the actual
        // "grows in height" bug: an EARLIER version tried to zero this out
        // with dueling +/- margins across three nested boxes instead, which
        // silently collapsed to 0 rather than to the intended net value —
        // CSS collapses adjoining margins to `max(positives) + min
        // (negatives)`, not a running sum).
        soloActive && cn('mx-1.5 my-0.5 flex h-9 items-center rounded-lg', ROW_ACTIVE),
        // NOTE — a solo view that is OPEN BUT OFF SCREEN gets no ground of its
        // own here, deliberately (and, as of the fix above, neither does an
        // off-screen SET). `ROW_ACTIVE` reads as "selected" precisely because
        // the rows around it are bare, which is the same relationship it has
        // in the tree. What separates open-off-screen from remembered is
        // already said on the row itself: `hasView` greys its label (§3.2,
        // "a row with a view is grey"), and it is passed for every live entry
        // regardless of which one is showing.
      )}
      data-testid={isSet ? `recents-set-${entry.id}` : undefined}
    >
      {entry.chatIds.map((chatId) => (
        <RecentsMemberRow
          key={chatId}
          // Per-chat, never the entry-wide `workspaceId` — a SET's members
          // can each belong to a different workspace within the project.
          workspaceId={entry.chatWorkspaces?.[chatId] ?? entry.workspaceId}
          chatId={chatId}
          // Open is open: a view sitting off screen still HAS a view, and the
          // tree's grey "already open" marker must not flicker off every time
          // the user looks at something else — EXCEPT for the one member
          // sitting on the shell's own ROW_ACTIVE ground right now
          // (`isShowing`): that surface already says "you are here" louder
          // than any row in the list, and greying the label on TOP of it
          // read as washed out/under-saturated (caught live) rather than the
          // full-strength text every other ROW_ACTIVE surface in the app
          // gets. Off screen (`isLive && !isShowing`) still greys, same as
          // ever — there is no ground of its own there to conflict with.
          hasView={isLive && !isShowing}
          isSet={isSet}
          suppressOwnMargin={soloActive}
          isShowingGround={soloActive}
          // Unlike `isShowingGround` (solo-only — it neutralizes the row's
          // OWN hover, which stays intentionally different for a set's
          // members), the shell's fill is ROW_ACTIVE for EVERY member of a
          // showing SET alike, not just a showing solo row — a set has no
          // notion of "which member is on screen," the whole shell either
          // shows or doesn't (`isSet && isShowing && ROW_ACTIVE` above).
          // `activeGround` is exactly that broader "is this row's body
          // painted on an inverted ROW_ACTIVE fill" fact, so it's `isShowing`
          // unconditionally, not `soloActive`.
          activeGround={isShowing}
          icon={entry.chatIcons?.[chatId]}
          onOpen={() => onFocus(entry)}
          // A SET member's own close removes just THIS chat from the group,
          // never the whole thing — there is no separate "close everything"
          // control any more (explicit product correction, asked repeatedly:
          // closing every member one at a time is the whole of it, and the
          // group dissolves for free once it is down to one — see
          // `viewIdOf`'s own "group of one" fallback, pane-views.ts). A solo
          // entry's one row IS the whole view, so its close still ends it.
          onClose={
            canClose ? (isSet ? () => onCloseChat(entry, chatId) : () => onClose(entry)) : undefined
          }
          drag={drag}
          registerRow={registerRow}
        />
      ))}
    </div>
  )
}

function RecentsMemberRow({
  workspaceId,
  chatId,
  hasView,
  isSet,
  suppressOwnMargin,
  isShowingGround,
  activeGround,
  icon,
  onOpen,
  onClose,
  drag,
  registerRow,
}: {
  /** Which workspace's store owns this chat — see `RecentsBandEntry`. */
  workspaceId: string
  chatId: string
  hasView: boolean
  /** `RecentsEntryRow`'s own `isSet` — whether this member is one of 2+ chats
   *  sharing a horizontal shell (see that shell's own `flex` doc). Shrinks
   *  this member to share the row (`flex-1 min-w-0`) and cancels the vertical
   *  half of its own `my-0.5` (see the className's own doc below). */
  isSet: boolean
  /** Set only for a lone live entry (`RecentsEntryRow`'s `soloActive`), whose
   *  OWN wrapper takes over `SidebarRow`'s `mx-1.5 my-0.5` to become the row's
   *  one active surface. Forwarded straight to `SidebarRow`'s own
   *  `suppressOwnMargin` — see that prop's doc for why this is dropped at the
   *  source rather than cancelled with an equal-and-opposite margin on a
   *  wrapper here (the margin-collapse bug that fix had). */
  suppressOwnMargin?: boolean
  /** Same condition as `suppressOwnMargin` (`RecentsEntryRow`'s `soloActive`,
   *  deliberately NOT `isShowing` alone — see `activeGround` below for the
   *  broader fact). `SidebarRow`'s shared `ROW_INACTIVE` token still carries
   *  its own `hover:bg-accent` regardless of caller, which — layered on top
   *  of ROW_ACTIVE's own bold inverted surface — read as the row's
   *  background visibly changing shape on hover (caught live: "we should
   *  only display the row with no hover animation apart from showing the
   *  close button"). Neutralised locally (below) rather than in
   *  sidebar-row.tsx: every OTHER caller (the tree, a merely-open-off-screen
   *  solo row, a SET's members — which keep their OWN distinct hover, see
   *  the `isSet` hover class below) still wants a hover of their own, so
   *  this is Recents' own, narrower override for the one case (a solo row
   *  that IS the whole view) where hovering should reveal only the close
   *  button and change nothing else. */
  isShowingGround?: boolean
  /** This member's row body sits directly on an inverted `ROW_ACTIVE` ground
   *  — `RecentsEntryRow`'s own `isShowing`, unconditional on solo vs. set
   *  (unlike `isShowingGround` above): a showing SET's shell is ROW_ACTIVE
   *  for every one of its members alike, since a set has no notion of
   *  "which member is on screen" — the whole shell either shows or doesn't.
   *  Forwarded to `SidebarRow` so it can swap its own (and its branch
   *  second-line's) ambient text-color tokens for the inverted pair that
   *  actually reads against that ground — see `SidebarRowProps.activeGround`'s
   *  own doc for the live-verified bug this fixes. */
  activeGround?: boolean
  /** This chat's tree-equivalent icon fields (`RecentsBandEntry.chatIcons`) —
   *  undefined for a chat that owns no workspace, which keeps this row's
   *  default `kind: 'chat', ownsWorktree: false` bubble below. */
  icon?: ChatIconFields
  onOpen: () => void
  /** This row's own close, resolved by `RecentsEntryRow` to whichever action
   *  fits: for a SET member, removes just THIS chat from the group (never
   *  the whole thing — the shell's own separate close does that); for a
   *  solo entry's one row, ends the whole view. Undefined for the one
   *  state with no close control at all (`working`). Forwarded straight to
   *  `SidebarRow`'s own `onClose` — see that prop's doc for why this no
   *  longer needs a reserved-padding overlay of its own. */
  onClose?: () => void
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

  const row: SidebarRowType | null = chat
    ? {
        id: chat.id,
        parentId: null,
        order: 0,
        // Same fallback the tree's own row builder uses (rows-from-repo.ts) —
        // the tree row and this one render the same chat, so they must agree
        // on what an unnamed one is called, not one showing a placeholder and
        // the other showing nothing at all.
        label: chat.title || UNTITLED_CHAT_LABEL,
        labelProvisional: !chat.title,
        workspaceId: chat.workspaceId,
        working,
        hasView,
        // `icon` (see its own doc) carries the SAME ownership fold the tree's
        // own row builder uses — a chat that owns a workspace draws the real
        // branch/lock/PR-status glyph here too instead of always falling back
        // to the generic bubble. Spread AFTER the defaults below so an owning
        // chat's `kind`/`ownsWorktree` override them; a bubble (no `icon`)
        // keeps exactly the old defaults.
        kind: 'chat',
        ownsWorktree: false,
        ...icon,
      }
    : null

  // `row` is built from `chat` above, so this also narrows `row` to
  // non-null for everything below (TS can't see the two are correlated
  // through the ternary alone).
  if (!chat || !row) return null

  return (
    <div
      data-testid={`recents-row-${chat.id}`}
      // So `subjectsFor` can hand a real, freshly-rendered row back to a drag
      // that grabs it — see RecentsBand's own `rowsRef` note above. A ref
      // callback, not a plain call in the render body: render must stay pure
      // (React can replay or discard it without committing), so writing
      // into the parent's `rowsRef` map has to wait until this element has
      // actually committed — guarded on `el` so the unmount call (React
      // invokes the OLD callback with `null` first) never re-registers a
      // stale row. React 19 re-invokes a ref callback whenever its own
      // identity changes — a fresh arrow every render, closing over the
      // current `row` — so this still registers on every render that
      // produces one, exactly like the call it replaces.
      ref={(el) => {
        if (el) registerRow(row)
      }}
      className={cn(
        // Unconditional, not just `isSet &&`: a no-op outside a flex parent
        // (a resting solo row's wrapper is a plain, non-flex `group relative`
        // div, where `flex-1`/`min-w-0` do nothing), but load-bearing the
        // moment ONE exists — the SOLO-showing wrapper (`soloActive`,
        // RecentsEntryRow's own doc) is ALSO `flex` now (for the margin-
        // collapse fix below), which makes this div a flex ITEM too. A flex
        // item's default `flex: 0 1 auto` shrink-wraps to its own content
        // instead of filling the row's real width — SidebarRow's own
        // trailing buttons sit on `flex-1` INSIDE this div, so a div that's
        // itself shrunk to content leaves them nothing to push against,
        // landing right next to the label instead of the row's far edge
        // (live-reported, alongside the same fix's own height regression).
        'min-w-0 flex-1',
        // SidebarRow's own inner div (via ROW_BASE) always carries `mx-1.5
        // my-0.5` too — inconsequential in the TREE, where each row is its
        // own block sibling and adjoining margins collapse/stack the usual
        // single-row gap. A SET's members are FLEX items on one line now
        // (feedback #2), where margins never collapse and never share space
        // with a sibling `gap` the way padding-box sizing expects — left
        // uncancelled, each member's own margin used to both inflate the
        // shell taller than a single row (measured live: 44px against 40px,
        // the vertical half) AND make the gap BETWEEN two members (two
        // touching `mx-1.5`s) read as visibly wider than the gap from the
        // shell's own edge to a member (live-reported: "the gap between
        // siblings ... shouldn't be that large"). Cancelling BOTH here and
        // letting the shell's own `gap-0.5`/`p-0.5` (RecentsEntryRow's own
        // doc) be the ONLY source of spacing is what makes every gap —
        // member-to-member, and shell-edge-to-member, on both axes — the
        // same 2px instead of three different values.
        isSet && '-mx-1.5 -my-0.5',
        // Neutralizes this member's own individual hover — `[role="treeitem"]`
        // (not `.group`, which several ancestors share) targets exactly
        // SidebarRow's own inner div, the one element `ROW_INACTIVE`'s
        // `hover:bg-accent` actually paints. Two callers, two reasons: a lone
        // SHOWING row (`isShowingGround`) already sits on its own bold
        // ROW_ACTIVE ground and needs nothing more from hover. A SET member
        // (`isSet`) needs its OWN hover silenced for the opposite reason —
        // user correction, live: "when hovering a group, the whole group
        // should receive the hover signal, not only the item being hovered"
        // (Zen browser's own grouped-tab capsule was named as the reference).
        // Highlighting one member's own box while its siblings stayed bare
        // read as "this one specific chat is special," not "this is one
        // group." The shell's own `group-hover` (RecentsEntryRow's own doc)
        // is what actually answers hover now — one shared surface across
        // every member, never a per-member one.
        (isShowingGround || isSet) && '[&_[role="treeitem"]]:hover:bg-transparent',
      )}
    >
      <SidebarRow
        row={row}
        depth={0}
        onOpen={onOpen}
        // Published so a hit test can tell this row apart from a tree row
        // wearing the same `parentId: null` — see `RowDragExtra.inRecents`.
        dragProps={drag.dragProps(row, { inRecents: true })}
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
        activeGround={activeGround}
        suppressOwnMargin={suppressOwnMargin}
        compactHeight={isSet}
        onClose={onClose}
      />
    </div>
  )
}
