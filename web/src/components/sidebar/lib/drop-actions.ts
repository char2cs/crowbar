import { toast } from '@/features/window/stores/toast-store'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { getPaneSplitDropOptions } from '@/features/panes/utils/pane-drop-zones'

/**
 * Committing a sidebar drop.
 *
 * `useSidebarDrag` (Task 21) is the drag ARM: hit-test, ghost, hairline, edge
 * scroll, the subtree cycle guard, pane-zone geometry — all real and wired
 * end to end, so a drag visibly tracks the pointer and resolves to a real
 * target. `performSidebarDrop` (row-to-row, via `onDrop`) stays a disclosed
 * placeholder — deliberately, not an oversight:
 *
 *   - A workspace/folder REORDER or RE-FILE has real, purpose-built store
 *     machinery already sitting ready for it (`SidebarPlacement`/
 *     `applyPlacement`/`capturePlacement` in `lib/store/sidebar.ts`, plus
 *     `placeWorkspace`/`placeFolder` in `lib/api/sidebar-placement.ts`) —
 *     but nesting a row 'into' another BRANCH row re-parents its FORK
 *     (`Workspace.parentId`), which that same file's own comment warns
 *     "silently breaks merge eligibility and the diff base" if written
 *     wrong. Getting the folder-vs-fork-edge precedence right needs reading
 *     `buildSidebarTree` (workspace-tree-utils.ts) in full and a real test
 *     suite against it — not a same-sitting guess.
 *   - A 'chat'/'workflow' row has NO sidebar placement endpoint at all yet
 *     (chat reordering has no live daemon route since the old, workspace-
 *     scoped Chats panel — and its `chat-tree-commit.ts` — was retired).
 *
 * This means "middle of a Recents entry" / "above·below a Recents entry"
 * (spec §8.1's other two targets) resolve mechanically today — a Recents
 * member row is a real droppable `SidebarRow` (`RecentsBand` already spreads
 * `drag.dragProps(row)` on it, Task 21), so a drop there DOES reach `onDrop`
 * with a real `mode: 'into' | 'before' | 'after'` — but committing "into"
 * as an open/merge and "before"/"after" as a reorder is exactly the write
 * this comment defers. `performSidebarPaneDrop` below is the part of spec
 * §8.1 this task (22) DOES build: the other two targets, "middle of a pane"
 * and "edge of a pane", which need no sidebar-placement endpoint at all —
 * only the pane store, which already has one.
 */
export function performSidebarDrop(
  subjects: SidebarRow[],
  target: SidebarRow,
  mode: DropMode,
): void {
  toast.info(
    `Reordering isn't wired to the daemon yet (${subjects.length} row(s) → ${target.id}, ${mode})`,
  )
}

/**
 * A row dropped onto a pane — spec §8.1/§8.2. Every drop here ADDS; nothing
 * this reaches for can remove a pane or evict a chat that is already showing
 * (the dwell-to-remove gesture this replaced is gone — Task 22).
 *
 * Scoped to `kind === 'chat'` subjects. A branch/folder/workflow row has no
 * "open into a pane" meaning in this app today — a branch NAVIGATES to a
 * different workspace route entirely (`space-content-actions.ts`'s
 * `handleOpen`), a folder only folds, and no 'workflow' row is produced
 * anywhere yet (`SidebarRowKind` carries it for a future feature). Dropping
 * one of those kinds onto a pane is silently a no-op rather than a guess at
 * behavior nothing in the codebase has defined.
 */
export function performSidebarPaneDrop(
  subjects: SidebarRow[],
  paneId: string,
  zone: SidebarPaneZone,
): void {
  for (const subject of subjects) {
    if (subject.kind === 'chat') openChatIntoPane(subject, paneId, zone)
  }
}

/**
 * One chat, dropped onto one pane.
 *
 * §8.2: "dropping a chat that is already up goes TO it... it never opens
 * twice." Checked FIRST, before any zone/merge logic, and against every
 * pane in the chat's own workspace — not just `paneId` — because the row
 * dragged onto pane B might already be showing, live, in pane A. The
 * established dedup pattern (`open-agent-chat.ts`'s `openAgentChat`):
 * `Object.values(panes).find(p => p.chatId === chatId)`, reveal via
 * `setActivePane`, never a second `setPaneChat`.
 */
function openChatIntoPane(subject: SidebarRow, paneId: string, zone: SidebarPaneZone): void {
  // A chat's owning workspace store is where its panes live — the same
  // resolution `openAgentChat`/`focusRecent`/`closeRecent` already use, and
  // the only one that works for a Recents row spanning a project's OTHER
  // workspaces (spec §4: "Recents is per space").
  if (!subject.workspaceId) return
  const store = getOrCreateWorkspaceStore(subject.workspaceId)
  const { panes, paneActions } = store.getState()
  const chatId = subject.id

  const existingPane = Object.values(panes).find((p) => p.chatId === chatId)
  if (existingPane) {
    paneActions.setActivePane(existingPane.id)
    return
  }

  const target = panes[paneId]
  if (!target) return

  // Middle of an EMPTY pane: a plain open, exactly where you dropped it.
  if (zone === 'center' && target.chatId === null) {
    paneActions.setPaneChat(paneId, chatId, null)
    paneActions.setActivePane(paneId)
    return
  }

  // Every other case is a MERGE — an edge always splits (spec §8.1: "into
  // this view, on that side"), and the middle of an already-occupied pane
  // can only ADD, never swap out what is already there (§8.2's rule 1 —
  // that silent swap is exactly the dwell-to-remove gesture's replacement),
  // so it falls back to the same split, defaulting to the right.
  const splitOptions = getPaneSplitDropOptions(zone === 'center' ? 'right' : zone)
  if (!splitOptions) return
  const newPaneId = paneActions.splitPane(paneId, splitOptions.direction, undefined, splitOptions.placement)
  if (!newPaneId) return
  paneActions.setPaneChat(newPaneId, chatId, null)
  paneActions.setActivePane(newPaneId)
  // "You asked for them side by side, so you get them side by side" (§8.2) —
  // only when there was something to merge WITH; splitting off an empty pane
  // opens one chat alone, nothing to group.
  if (target.chatId) paneActions.groupIntoArrangement([target.chatId, chatId])
}
