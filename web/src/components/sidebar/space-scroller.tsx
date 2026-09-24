import { useEffect, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { useNavigate } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { SidebarTree } from './sidebar-tree'
import { SpaceHeader } from './space-header'
import { RecentsBand } from './recents-band'
import { CARD_BOTTOM_INSET_VAR } from '@/components/layout/sidebar-card-height'
import { findScrollParent } from '@/components/layout/edge-scroll'
import { performCreateHomeFolder } from '@/components/sidebar/lib/row-actions'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { rowRepoScope } from '@/components/sidebar/lib/rows-from-repo'
import { hideRowsForInFlightCreates } from '@/components/sidebar/lib/rows-from-pending'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'
import {
  ensureHomeWorkspaceResolved,
  useHomeWorkspaceState,
} from '@/features/workspace/lib/home-workspace-resolver'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { toast } from '@/features/window/stores/toast-store'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { usePendingCreatesStore } from '@/lib/store/pending-creates'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { attachRemovalState, renderedHiddenIds } from '@/components/layout/removal-plan'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { selectProjectViewIds } from '@/features/panes/lib/view-selectors'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { Project } from '@/lib/types'

interface SpaceScrollerProps {
  projects: Project[]
  /** Undefined on a route with no project in it yet (matches
   *  `SidebarFooter`'s own `activeProjectId?: string` — the two read
   *  the SAME value, ide-shell.tsx's `activeProjectIdFromRoute`). */
  activeProjectId: string | undefined
  onActiveProjectChange: (id: string) => void
  rowsForProject: (projectId: string) => SidebarRow[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  onFocusRecent: (viewId: string) => void
  onCloseRecent: (viewId: string) => void
  /** A group member's own ×: closes that one chat, never the group. */
  onCloseChatRecent: (chatId: string) => void
  onDrop: (subjects: SidebarRow[], target: SidebarRow, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void
  /** Spec §9's project-level trash, the one verb the space header's overflow
   *  carries. Threaded as a prop like every other verb here rather than
   *  called directly, so this file stays presentational. */
  onTrashProject: (projectId: string) => void
}

interface SpacePanelProps {
  project: Project
  rowsForProject: (projectId: string) => SidebarRow[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  onFocusRecent: (viewId: string) => void
  onCloseRecent: (viewId: string) => void
  onCloseChatRecent: (chatId: string) => void
  onDrop: (subjects: SidebarRow[], target: SidebarRow, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void
  onTrashProject: (projectId: string) => void
}

function SpacePanel({
  project,
  rowsForProject,
  onOpen,
  onTrash,
  onCreate,
  onFocusRecent,
  onCloseRecent,
  onCloseChatRecent,
  onDrop,
  onPaneDrop,
  onTrashProject,
}: SpacePanelProps) {
  const projectId = project.id
  const repoRows = rowsForProject(projectId)
  // Row id -> repo id, so a repo-scoped pending create (a branch import)
  // only ever suppresses a stray reseed within ITS OWN repo, never a
  // brand-new row anywhere else in this project (`hideRowsForInFlightCreates`'s
  // own doc, rows-from-pending.ts). Read off the canonical store, not
  // `repoRows` above, since scope only needs the id, never the rendered
  // (removal-filtered) shape.
  const allRepos = useSidebarStore((s) => s.repos)
  const rowRepoId = useMemo(
    () => rowRepoScope(allRepos.filter((r) => r.projectId === projectId)),
    [allRepos, projectId],
  )
  // The REAL project-home workspace — project-scoped, not repo-scoped
  // (home-workspace-resolver.ts: "home is a project-level concept, not a
  // repo workspace"). Target for the header's Thread button and the
  // add-menu's "Create a folder" item — NOT a repo's own home row (a
  // different workspace entirely; conflating the two was a real bug, a
  // "New thread on the project" button that silently created the thread
  // under a REPO instead, caught live).
  const { wsId: homeWorkspaceId, owningChatId: homeOwningChatId } = useHomeWorkspaceState(projectId)
  useEffect(() => {
    ensureHomeWorkspaceResolved(projectId)
  }, [projectId])
  // Every chat-scoped API call for this workspace (`setChatPlacement`,
  // `createChat`, `workspaceBase`'s own throw) needs its scope RECORDED
  // first (workspace-scope.ts) — `ide-shell.tsx` only ever records the
  // ACTIVE route's home workspace, but `SpacePanel` mounts one per VISIBLE
  // project, not just the active one. Without this, dragging/reordering (or
  // even just opening) a home chat on any project OTHER than the one
  // currently on screen throws "no project/repo scope recorded" instead of
  // working — `recordWorkspaceScope` (not `setWorkspaceScope`) is the one
  // that writes without also claiming this workspace as ACTIVE, exactly
  // matching how `lib/store/sidebar.ts` already records every repo
  // workspace's scope "as its data arrives," per that function's own doc.
  //
  // Recorded SYNCHRONOUSLY during render, not in a `useEffect` — mirrors
  // `ide-shell.tsx`'s own `setWorkspaceScope` call and its doc comment: a
  // `useEffect` here left a window, right after `homeWorkspaceId` first
  // resolves, where the header's already-enabled Thread button reads a real
  // `homeWorkspaceId` but the scope effect for THIS same value has not yet
  // committed — a click landing in that window (caught live) threw "no
  // project/repo scope recorded" from a workspace that plainly exists.
  // `recordWorkspaceScope`'s listener notify is `useSyncExternalStore`-safe
  // (see `useOwningChatId`, use-workspace-effects.ts) precisely so callers
  // can do this — call it mid-render — the same guarantee `setWorkspaceScope`
  // already relies on above.
  if (homeWorkspaceId) recordWorkspaceScope({ projectId, repoId: '', wsId: homeWorkspaceId })
  // This project's home chats/folders — the same two aggregates a repo's
  // OWN tree holds, kept in their own per-project store (home-tree.ts) since
  // project home rides no repo. Rendered as FLAT TOP-LEVEL rows, exactly
  // like a repo itself — explicit user correction: `rowsFromHome` first drew
  // a container "Home" row for these to nest under, mirroring a repo's own
  // home row, and it was rejected outright; there is no such container here.
  const homeTree = useHomeTreeStore((s) => s.trees[projectId])
  // `rowsFromHome` degrades gracefully (never throws) while its owning chat
  // has not resolved yet — same as `rowsFromRepo`'s own home row — so this
  // only needs to gate on the tree itself having seeded, the same way
  // `SidebarTreeSurface`'s `seededRepoIds` keeps a repo's rows from being
  // built before ITS seed has landed. Task 9 deleted the boot backfill that
  // used to make that resolution a real (if narrow) race — the owning chat
  // is minted chat-first, atomically, at this workspace's own creation.
  const homeSeeded = homeWorkspaceId !== null && homeTree !== undefined
  // A held home chat/folder's cascade descendants disappear the same way a
  // held repo row's do — `repoRows` above comes in already filtered AND
  // marked for the in-place transform (`SidebarTreeSurface`'s
  // `rowsForProjectFn`), but a home tree is never part of `repos` for that
  // projection to reach, so it is filtered/marked here instead. The PRIMARY
  // id stays (removal-plan.ts's `descendantHiddenIds` doc) — only its
  // cascade goes. Filtered against the tray's own `hiddenIds`, not an
  // `entries`-derived set — see `sidebar-tree-surface.tsx`'s identical
  // `renderedHiddenIds` swap for why: `entries` drops a row the instant its
  // commit fires, before the DELETE it just sent has resolved, and deriving
  // from `entries` alone let that row ghost back onto the tree for the
  // length of the round trip.
  const removalEntries = useRemovalTrayStore((s) => s.entries)
  const trayHiddenIds = useRemovalTrayStore((s) => s.hiddenIds)
  const hiddenIds = useMemo(
    () => renderedHiddenIds(trayHiddenIds, removalEntries),
    [trayHiddenIds, removalEntries],
  )
  // Read here (not only in `sidebar-tree-surface.tsx`) because `homeRows`
  // below is built off `useHomeTreeStore`, outside that merge entirely.
  const pendingEntries = usePendingCreatesStore((s) => s.entries)
  // A repo header row already in `repoRows` (rowsFromRepo's own push) carries
  // its own real `parentId`/`order` straight off the wire — Task 3 put a
  // repo's position on its own `Node` row, computed server-side against
  // these SAME real home chat/folder siblings, so it needs no correction
  // here any more: it interleaves into the same sibling sort as
  // `rowsFromHome`'s rows just by sitting in the same flat list, exactly the
  // way a chat or folder row already does.
  const homeRows =
    homeSeeded && homeTree
      ? attachRemovalState(
          rowsFromHome(
            homeWorkspaceId,
            homeTree.chats.filter((c) => !hiddenIds.has(c.id)),
            homeTree.folders.filter((f) => !hiddenIds.has(f.id)),
            homeOwningChatId ?? undefined,
          ),
          removalEntries,
        )
      : []
  // The pending row is the ONE stand-in for a create in flight — its real row
  // reseeds in (at root, before its placement write) long before the POST
  // answers, so it is hidden until the entry clears (rows-from-pending.ts).
  const rows = hideRowsForInFlightCreates(
    [...homeRows, ...repoRows],
    pendingEntries,
    projectId,
    rowRepoId,
  )
  const navigate = useNavigate()
  // The tree and Recents sit in ONE shared scroll region (spec §2) and both
  // take `useSidebarDrag` (Task 21) — each resolves its own edge-scroll
  // target off this ref, which points at the actual overflow element
  // (`ScrollAreaPrimitive.Viewport`), not the plain content div a naive ref
  // here would otherwise land on.
  const contentRef = useRef<HTMLDivElement>(null)
  const viewportRef = useRef<HTMLElement | null>(null)
  useEffect(() => {
    viewportRef.current = findScrollParent(contentRef.current)
  })
  const viewIds = useStore(windowPaneStore, (s) => selectProjectViewIds(s, projectId))

  // Spec §4: "clicking the header folds the space: the tree goes, Recents
  // stays. That is the point of folding — to see nothing but what is up in
  // this project. They share one scroller, so the fold hides the ROWS, not
  // the scroller." Local per-panel state, not persisted: nothing in the spec
  // asks a fold to survive a reload, and each project's panel is its own
  // component instance (keyed by project id), so "per space" is free.
  const [folded, setFolded] = useState(false)

  const [addRepoOpen, setAddRepoOpen] = useState(false)

  return (
    <div
      data-testid="space-panel"
      className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
    >
      {/* flex: none, above the scroller — spec §2's own layout diagram. */}
      <div className="shrink-0">
        <SpaceHeader
          project={project}
          folded={folded}
          onToggleFold={() => setFolded((f) => !f)}
          onCreateThread={() => {
            // Not `onCreate`/`handleCreate`: that pipe resolves parentId
            // against the REPO-scoped sidebar store (resolveRow), which has
            // no notion of the project-home workspace at all — this calls
            // the project-home-aware sibling instead, which also owns
            // opening/navigating to the new chat (mirrors handleCreate's own
            // open/navigate handling, just scoped to the home workspace).
            //
            // Reported live as "can't create a thread from the project
            // home": a freshly-opened (or just-created) project's home
            // workspace resolves asynchronously (`ensureHomeWorkspaceResolved`
            // above) — clicking during that window used to silently do
            // nothing, with no error and no visible reason, since
            // `homeWorkspaceId` was still null. A row's own refusals (a
            // locked branch, a working chat) all surface a toast instead —
            // this one now matches.
            if (!homeWorkspaceId) {
              toast.error("Can't start a new thread yet")
              return
            }
            void handleCreateHomeThread(projectId, homeWorkspaceId, navigate)
          }}
          // Same create as `onCreateThread` above, landed on Terminal — "start
          // THIS chat on the CLI" without flipping chatIsDefaultPresentation
          // (Settings → Chat) for every project-home thread after it. Same
          // not-yet-resolved refusal as the plain button, same reason.
          onCreateThreadTerminal={() => {
            if (!homeWorkspaceId) {
              toast.error("Can't start a new thread yet")
              return
            }
            void handleCreateHomeThread(projectId, homeWorkspaceId, navigate, 'terminal')
          }}
          // Imports another repo into this project — opens the same modal
          // the standalone Add-menu button used to.
          onImportRepo={() => setAddRepoOpen(true)}
          // Starts a folder on the project's OWN home workspace (the
          // backend's `/home/chats/folders` mount — folders were once
          // thought repo-internal only; they are not).
          onCreateFolder={() => void performCreateHomeFolder(projectId)}
          onDeleteSpace={() => onTrashProject(project.id)}
        />
      </div>
      <AddRepositoryModal open={addRepoOpen} onOpenChange={setAddRepoOpen} projectId={project.id} />
      <ScrollArea className="flex-1">
        <div ref={contentRef} data-testid="space-scroll-content">
          {!folded && (
            <SidebarTree
              rows={rows}
              onOpen={onOpen}
              onTrash={onTrash}
              onCreate={onCreate}
              scrollRef={viewportRef}
              onDrop={onDrop}
              onPaneDrop={onPaneDrop}
            />
          )}
          <RecentsBand
            viewIds={viewIds}
            onFocus={onFocusRecent}
            onClose={onCloseRecent}
            onCloseChat={onCloseChatRecent}
            scrollRef={viewportRef}
            onDrop={onDrop}
            onPaneDrop={onPaneDrop}
          />
        </div>
      </ScrollArea>
      {/* The clearance itself (spec §6: "the tree keeps a bottom inset the
          height of the card") — now an outer, non-scrolling spacer instead of
          padding on the scrollable content. Padding there counted straight
          toward `scrollHeight`: the card's default open height is a THIRD of
          the whole sidebar rail (`DEFAULT_CARD_HEIGHT_FRACTION`), so that
          padding alone could tip an otherwise short, non-overflowing list
          into "overflowing," showing a scrollbar nothing about the visible
          rows justified. A `shrink-0` sibling shrinks the ScrollArea's own
          box instead, so the browser's real overflow check only ever sees
          genuine row/Recents content — same visual clearance, correct
          overflow math. Reads `--card-bottom-inset` straight off the CSS
          cascade rather than a prop for the same reason the content div
          used to: that variable is written directly onto the shared rail
          ancestor by sidebar-carousel.tsx, including once per animation
          frame during a resize drag, and a prop here would re-render this
          panel (and every row in it) on every one of those frames. */}
      <div
        data-testid="space-scroll-bottom-spacer"
        className="shrink-0"
        style={{ height: `var(${CARD_BOTTOM_INSET_VAR}, 0px)` }}
      />
    </div>
  )
}

/**
 * One horizontal, x-mandatory-snap panel per project (spec §4). Copies
 * sidebar-carousel.tsx's scroll mechanics rather than importing it - that
 * carousel's scope is narrowing to Files/Git only (D.1), and mixing "which
 * project" with "which card tab" into one component would conflate two
 * different numbers.
 *
 * Each panel is ONE scroll region (spec §2's "the tree and Recents are one
 * scrolling group, not two stacked panels") holding a project's `SidebarTree`
 * followed by its own `RecentsBand`.
 */
export function SpaceScroller({
  projects,
  activeProjectId,
  onActiveProjectChange,
  rowsForProject,
  onOpen,
  onTrash,
  onCreate,
  onFocusRecent,
  onCloseRecent,
  onCloseChatRecent,
  onDrop,
  onPaneDrop,
  onTrashProject,
}: SpaceScrollerProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  // Armed only by an actual scroll gesture over the scroller. Everything else
  // that moves scrollLeft is reflow, not intent - see sidebar-carousel.tsx.
  const isUserGesture = useRef(false)
  const armUserGesture = () => {
    isUserGesture.current = true
  }
  // The vertical chat-list ScrollArea lives INSIDE this horizontal carousel's
  // subtree (SpacePanel), so a plain vertical wheel tick over the chat list
  // still bubbles up and reaches this handler — React's onWheel doesn't
  // filter by axis. Left unguarded, that armed this exactly like a real
  // horizontal swipe, and a later scrollLeft nudge for any unrelated reason
  // would silently swap the active project mid-scroll. Only a
  // horizontally-dominant gesture is a real swipe of THIS carousel (same
  // axis check Zen Browser's native arrowscrollbox patch uses on its own
  // wheel handler).
  const armUserGestureFromWheel = (e: React.WheelEvent) => {
    if (Math.abs(e.deltaX) <= Math.abs(e.deltaY)) return
    isUserGesture.current = true
  }
  // The panel index a smooth, programmatic scroll is currently travelling to.
  // That animation fires scroll events the whole way and none of them is a
  // user gesture, so the re-align below would cancel the animation it is
  // chasing by jumping straight to its destination — the switch would snap
  // instead of glide.
  const settlingToIndex = useRef<number | null>(null)

  // Re-align scroll when the container is resized. Each panel is min-w-full,
  // so scrollLeft must stay at projectIndex * containerWidth.
  useEffect(() => {
    const el = containerRef.current
    if (!el || typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(() => {
      isUserGesture.current = false
      const index = projects.findIndex((p) => p.id === activeProjectId)
      if (index === -1) return
      // A collapsed sidebar has zero width: no offset identifies a panel, and
      // the browser has already clamped scrollLeft to 0. Leave it - the
      // resize that reopens the sidebar re-aligns it.
      if (el.clientWidth === 0) return
      settlingToIndex.current = null
      el.scrollLeft = index * el.clientWidth
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [projects, activeProjectId])

  // Scroll to the correct panel when activeProjectId changes.
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const index = projects.findIndex((p) => p.id === activeProjectId)
    if (index === -1) return
    isUserGesture.current = false
    const left = index * el.clientWidth
    // Already there: scrollTo fires no scroll event, so arming would leave the
    // re-align disarmed for good.
    if (el.scrollLeft === left) {
      settlingToIndex.current = null
      return
    }
    settlingToIndex.current = index
    el.scrollTo({ left, behavior: 'smooth' })
  }, [activeProjectId, projects])

  // Sync activeProjectId when the user swipes/wheels — and hold the settled
  // panel and the active project together when anything else scrolls us.
  function handleScroll() {
    const el = containerRef.current
    if (!el || el.clientWidth === 0) return
    const index = Math.round(el.scrollLeft / el.clientWidth)
    if (!isUserGesture.current) {
      if (settlingToIndex.current !== null) {
        if (settlingToIndex.current === index) settlingToIndex.current = null
        return
      }
      // Not a swipe, and nothing programmatic is in flight: focus's own
      // scrollIntoView (every row holds tabbable buttons, in EVERY panel), an
      // edge-scroll, a WebKit snap. `overflow-x: scroll` + mandatory x
      // snapping + min-w-full panels turns any of them into a whole panel,
      // leaving the sidebar naming one space while the content area shows
      // another — the exact divergence project-scoped panes exists to kill.
      // Bounce back rather than switch a project the user never asked for.
      const active = projects.findIndex((p) => p.id === activeProjectId)
      if (active !== -1 && active !== index) el.scrollLeft = active * el.clientWidth
      return
    }
    const project = projects[index]
    if (project && project.id !== activeProjectId) {
      onActiveProjectChange(project.id)
    }
  }

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      onWheel={armUserGestureFromWheel}
      onTouchStart={armUserGesture}
      data-testid="space-scroll-region"
      // Same geometry, same failure mode, same fix as sidebar-carousel.tsx's
      // own `[data-sidebar-carousel]`: `overflow-x: scroll` + mandatory x
      // snapping + min-w-full panels means a captured pointer travelling
      // past this box's edge during a row drag gets scrolled a whole panel
      // away by WebKit — the exact gesture Part G exists for (dragging a row
      // rightward onto a pane). `index.css`'s `html[data-row-dragging]
      // [data-sidebar-carousel]` rule pins BOTH carousels by this one
      // attribute; see drag-carousel-pin.test.ts for its own measured
      // rationale (pin without `will-change` costs 4x the frame budget).
      data-sidebar-carousel=""
      className="flex flex-1 overflow-x-scroll overflow-y-hidden [scroll-snap-type:x_mandatory] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {projects.map((project) => (
        <SpacePanel
          key={project.id}
          project={project}
          rowsForProject={rowsForProject}
          onOpen={onOpen}
          onTrash={onTrash}
          onCreate={onCreate}
          onFocusRecent={onFocusRecent}
          onCloseRecent={onCloseRecent}
          onCloseChatRecent={onCloseChatRecent}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={onTrashProject}
        />
      ))}
    </div>
  )
}
