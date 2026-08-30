import { toast } from '@/features/window/stores/toast-store'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * Committing a sidebar drop — deliberately NOT built here.
 *
 * `useSidebarDrag` (Task 21) is the drag ARM: hit-test, ghost, hairline, edge
 * scroll, the subtree cycle guard, pane-zone geometry — all real and wired
 * end to end, so a drag visibly tracks the pointer and resolves to a real
 * target. What a RELEASE should WRITE is a distinct, higher-risk piece of
 * work this task's own "Produces" contract never asked for, and it is not
 * safe to improvise under this task's time budget:
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
 *     (chat reordering today goes through the separate, workspace-scoped
 *     `chat-tree-commit.ts`, built for the old Chats panel's own id space).
 *   - `onPaneDrop`'s real semantics (spec §8.1's four-target table — open
 *     into a view, merge into a live one, reorder a Recents entry) are
 *     explicitly Task 22's job ("every pane drop adds"): it deletes the old
 *     dwell-to-remove overlay and rewires whatever mounts a droppable pane
 *     onto this hook's `onPaneDrop`. No live element carries `PANE_DROP_ATTR`
 *     yet for it to act on.
 *
 * Both handlers below are therefore safe, disclosed placeholders — the drag
 * resolves and calls them, but nothing is written to the daemon. Follow-up:
 * a dedicated task (same shape as this plan's Tasks 29-32) should build the
 * real commit path with its own test coverage before this is load-bearing.
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

export function performSidebarPaneDrop(
  subjects: SidebarRow[],
  paneId: string,
  zone: SidebarPaneZone,
): void {
  toast.info(`Pane drops aren't wired yet (${subjects.length} row(s) → pane ${paneId}, ${zone})`)
}
