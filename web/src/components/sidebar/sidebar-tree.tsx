import { useCallback, useSyncExternalStore } from 'react'
import { useStore } from 'zustand'
import { cn } from '@/lib/utils'
import {
  readChatWorking,
  subscribeChatWorking,
} from '@/features/workspace/stores/workspace-store-registry'
import { useSidebarStore } from '@/lib/store/sidebar'
import {
  ROW_BASE,
  ROW_INACTIVE,
  ROW_INDENT_STEP,
  ROW_INDENT_TRANSITION,
} from '@/components/layout/workspace-row-base'
import { DragGhost, DragGhostRows } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import { SidebarRow } from '@/components/sidebar/sidebar-row'
import { AffordanceRow } from '@/components/sidebar/affordance-row'
import { useSidebarDrag, type SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { selectChatHasView } from '@/features/panes/stores/slices/pane-slice'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarRow as SidebarRowType } from '@/components/sidebar/types/sidebar-row'

interface SidebarTreeProps {
  /** One project's rows, flat, parentId-linked. */
  rows: SidebarRowType[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  /** The panel's own scroll container — what an edge-held drag scrolls
   *  (Task 21). Shared with `RecentsBand`, since the two sit in one scroll
   *  region per space. */
  scrollRef: React.RefObject<HTMLElement | null>
  onDrop: (subjects: SidebarRowType[], target: SidebarRowType, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRowType[], paneId: string, zone: SidebarPaneZone) => void
}

const byOrder = (a: SidebarRowType, b: SidebarRowType) => a.order - b.order

/**
 * `renderRow` below is a plain recursive function called inside `.map()`/
 * recursion, not a component — calling a hook inside it would violate the
 * rules of hooks (a varying call count/order as the tree's shape changes
 * across renders). This is the real component `renderRow` defers to instead,
 * mirroring `recents-band.tsx`'s `RecentsMemberRow`: it subscribes this ONE
 * row to the two LIVE facts `rows-from-repo.ts` deliberately refuses to seed —
 * pane membership and turn state — and overrides the inert `hasView: false` /
 * `working: false` every row arrives with from that pure bridge.
 *
 * `working` is the newer of the two and it closes a real hole. `rows-from-repo`
 * has always seeded `working: false` on a chat row ("ALWAYS FALSE, AND NOT AN
 * OVERSIGHT" — a value seeded once latches the spinner on a chat whose turn
 * ended minutes ago) and a STATIC `Workspace.working` on a branch row, which is
 * a field off a seeded record rather than a per-turn signal. Both reasons are
 * still right; what was missing is the third option those notes point at —
 * "anything else that comes to need real turn state here must subscribe per row
 * the way Recents does". This is that subscription. Now that a workspace and
 * its conversation render as ONE prominent, glyph-swapping row, the missing
 * spinner is the most visible thing in the sidebar rather than a detail on a
 * bubble.
 *
 * NOT `useWorkspaceStoreById` (what `RecentsMemberRow` uses) and NOT
 * `isChatWorking`, and both refusals are load-bearing. `useWorkspaceStoreById`
 * goes through `getOrCreateWorkspaceStore`, which MINTS a store for any id
 * handed to it — fine for Recents, which only ever names workspaces that are
 * already open, and a per-session leak here, where the tree draws a row for
 * every workspace in the repo including the ones nobody has touched (see
 * `getWorkspaceStore`'s own doc on exactly that leak). `isChatWorking` returns
 * a boolean with nothing to subscribe to. `subscribeChatWorking` /
 * `readChatWorking` are the watch-don't-create pair: no store is created, and
 * the row re-binds to the real one the moment that workspace mounts.
 */
function SidebarTreeRow({
  row,
  ...rest
}: { row: SidebarRowType } & Omit<React.ComponentProps<typeof SidebarRow>, 'row'>) {
  const hasView = useStore(windowPaneStore, (s) => selectChatHasView(s, row.id))
  // The row's OWN id is the chat id — every row the tree draws that has a real
  // conversation behind it (a folded branch row and a bubble alike) is id'd by
  // it (`rows-from-repo.ts`). A folder names no chat and no workspace, so it
  // subscribes to nothing and reads a constant `false`.
  const wsId = row.kind === 'folder' ? '' : (row.workspaceId ?? '')
  const subscribe = useCallback(
    (onChange: () => void) => (wsId ? subscribeChatWorking(wsId, onChange) : () => {}),
    [wsId],
  )
  const getSnapshot = useCallback(
    () => (wsId ? readChatWorking(wsId, row.id) : false),
    [wsId, row.id],
  )
  const chatWorking = useSyncExternalStore(subscribe, getSnapshot)
  // TWO LIVE SIGNALS AT TWO GRAINS, UNIONED — not a fallback, and not two
  // sources disagreeing about one fact.
  //
  //   `chatWorking`  — THIS conversation is mid-turn. Exact, per-chat, and the
  //                    only one that can ever be true for a bubble. Requires
  //                    the workspace to be mounted (its `working` map is filled
  //                    by that workspace's own chats stream).
  //   `row.working`  — the WORKTREE has work in flight (`Workspace.working`,
  //                    pushed live on `worktree_state` frames over the repo-wide
  //                    chat feed, which stays open while the repo shows rows).
  //                    Coarser, but answers for a workspace nobody has opened.
  //
  // Neither subsumes the other, and both mean the same thing to a reader —
  // "this row is busy" — so the row spins if either says so. `row.working`
  // arrives as a hard `false` on every `chat`-kind row by construction
  // (`rows-from-repo.ts`), so for a bubble this is purely the chat signal.
  const working = chatWorking || row.working
  return <SidebarRow row={{ ...row, hasView, working }} {...rest} />
}

/**
 * Walks a flat, parentId-linked `SidebarRow[]` into the real nested tree —
 * spec §4: one project's rows render as siblings, with no rule between them
 * (the horizontal separators that used to divide projects are gone now that a
 * space holds exactly one).
 *
 * Every row is a container (spec §3.1: "a container can always be given
 * something"), so every row gets the fold chevron. A container currently
 * holding nothing renders the one affordance row (§3.5) in place of its
 * children, rather than nothing.
 *
 * Fold state reuses `collapsedChatRows` off the always-alive `useSidebarStore`
 * — the same set `agent-chats-panel.tsx` folds Chats-panel rows into today —
 * rather than a second fold-state store. It already survives what this tree
 * will too: a remount local component state would not.
 */
export function SidebarTree({
  rows,
  onOpen,
  onTrash,
  onCreate,
  scrollRef,
  onDrop,
  onPaneDrop,
}: SidebarTreeProps) {
  const collapsed = useSidebarStore((s) => s.collapsedChatRows)

  const ids = new Set(rows.map((r) => r.id))
  const childrenByParent = new Map<string, SidebarRowType[]>()
  const roots: SidebarRowType[] = []
  for (const row of rows) {
    // A row whose parent isn't in this project's own set is a root too —
    // defensive against a dangling edge rather than silently dropping it.
    const parentId = row.parentId !== null && ids.has(row.parentId) ? row.parentId : null
    if (parentId === null) {
      roots.push(row)
      continue
    }
    const siblings = childrenByParent.get(parentId)
    if (siblings) siblings.push(row)
    else childrenByParent.set(parentId, [row])
  }
  roots.sort(byOrder)
  for (const siblings of childrenByParent.values()) siblings.sort(byOrder)

  // Single-row drags only — the new unified tree has no multiselect yet
  // (Task 8 left `sidebar-selection.ts` orphaned), so a drag always carries
  // exactly the grabbed row.
  const drag = useSidebarDrag({
    scrollRef,
    subjectsFor: (rowId) => {
      const row = rows.find((r) => r.id === rowId)
      return row ? [row] : []
    },
    onDrop,
    onPaneDrop,
  })

  function renderRow(row: SidebarRowType, depth: number, path: string) {
    const folded = collapsed.has(row.id)
    const children = childrenByParent.get(row.id)
    const hasChildren = !!children && children.length > 0
    const childPath = `${path}${row.id}/`

    return (
      <div key={row.id}>
        <SidebarTreeRow
          row={row}
          depth={depth}
          onOpen={onOpen}
          onTrash={onTrash}
          onCreate={onCreate}
          onToggleFold={(id) => useSidebarStore.getState().toggleChatRow(id)}
          folded={folded}
          dragProps={drag.dragProps(row, { path: childPath, expanded: !folded, hasChildren })}
          isDragging={drag.draggingIds.has(row.id)}
          isNestTarget={drag.nestTargetId === row.id}
          onPointerDownDrag={(e) => drag.onPointerDownDrag(row, e)}
        />
        {!folded &&
          (hasChildren ? (
            children!.map((child) => renderRow(child, depth + 1, childPath))
          ) : row.kind === 'folder' ? (
            // Addendum §5: a folder has no owning chat of its own, so it is
            // the one container whose own row can't carry Fork/Thread — it
            // still needs this nested bootstrap row when childless. A
            // `branch` or `chat` row, by contrast, already got its own
            // always-present Fork/Thread buttons straight on the row
            // (sidebar-row.tsx) — rendering this underneath THOSE kinds too
            // was the redundant, unlabeled "empty" row users were seeing
            // under every real chat.
            <div
              className={ROW_INDENT_TRANSITION}
              style={{ marginInlineStart: (depth + 1) * ROW_INDENT_STEP }}
            >
              <div
                className={cn(ROW_BASE, ROW_INACTIVE, 'group cursor-default justify-end pr-2.5')}
              >
                <AffordanceRow
                  onCreateThread={() => onCreate(row.id, 'thread')}
                  onCreateWorkspace={
                    row.ownsWorktree ? () => onCreate(row.id, 'workspace') : undefined
                  }
                />
              </div>
            </div>
          ) : null)}
      </div>
    )
  }

  return (
    <>
      {roots.map((row) => renderRow(row, 0, '/'))}
      {drag.dragging && <DropIndicator ref={drag.attachDropLine} />}
      {drag.ghostRows && (
        <DragGhost ref={drag.ghostRef} origin={drag.ghostOrigin}>
          <DragGhostRows rows={drag.ghostRows} />
        </DragGhost>
      )}
    </>
  )
}
