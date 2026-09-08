import { useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { FolderOpen, Folder as FolderIcon } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ContextMenu, useContextMenu } from '@/components/ui/context-menu'
import { SidebarTree } from './sidebar-tree'
import { SpaceHeader } from './space-header'
import { RecentsBand, type RecentsBandEntry } from './recents-band'
import { CARD_BOTTOM_INSET_VAR } from '@/components/layout/sidebar-card-height'
import { findScrollParent } from '@/components/layout/edge-scroll'
import { performCreateHomeFolder } from '@/components/sidebar/lib/row-actions'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'
import {
  ensureHomeWorkspaceResolved,
  useHomeWorkspaceState,
} from '@/features/workspace/lib/home-workspace-resolver'
import { handleCreateHomeThread } from '@/components/layout/space-content-actions'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { WorkspaceState } from '@/features/workspace/stores/workspace-store.types'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/hooks/use-sidebar-drag'
import type { Project } from '@/lib/types'

// Join delimiter for the id-list dependency keys below — same choice and
// rationale as workspace-host.tsx's own `ID_DELIM`: a workspace id can never
// contain NUL, but nothing stops one containing a space or `|`, which a
// naive join/split would then mis-parse.
const ID_DELIM = '\u0000'

interface SpaceScrollerProps {
  projects: Project[]
  /** Undefined on a route with no project in it yet (matches
   *  `SidebarFooter`'s own `activeProjectId?: string` — the two read
   *  the SAME value, ide-shell.tsx's `activeProjectIdFromRoute`). */
  activeProjectId: string | undefined
  onActiveProjectChange: (id: string) => void
  rowsForProject: (projectId: string) => SidebarRow[]
  recentsForProject: (projectId: string) => RecentsBandEntry[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  onFocusRecent: (entry: RecentsBandEntry) => void
  onCloseRecent: (entry: RecentsBandEntry) => void
  onDrop: (subjects: SidebarRow[], target: SidebarRow, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void
  /** Spec §9's project-level trash, the one verb the space header's overflow
   *  carries. Threaded as a prop like every other verb here rather than
   *  called directly, so this file stays presentational. */
  onTrashProject: (projectId: string) => void
}

/** The two `agentChats` fields Recents actually reads, compared by reference
 *  below (immer only replaces a slice's reference when that slice was
 *  actually mutated) - mirrors workspace-store-registry.ts's own
 *  persistence-subscribe idiom, so an unrelated store write (LSP, terminal
 *  output) never triggers a recompute. `panes`/`dormantArrangements` moved to
 *  the window-level pane store (Task 26) — see the separate subscription
 *  below, no longer one of these per-workspace fields. */
function recentsSlice(state: WorkspaceState) {
  return {
    working: state.agentChats.working,
    chats: state.agentChats.chats,
  }
}

/**
 * Re-renders the caller whenever a currently-active workspace's working
 * chats or chat list change, OR the one window-level pane store's panes/
 * dormant arrangements change, so a project's Recents band recomputes with
 * fresh data - a plain `.subscribe(listener)` fires on EVERY store mutation,
 * so the listener drops anything that did not touch one of these fields.
 *
 * `workspaceIds` need not already be filtered to active ones: subscribing is
 * skipped for an id with no live store rather than creating one (see
 * recents-for-project.ts for why calling `getOrCreateWorkspaceStore` on a
 * never-opened workspace would leak it).
 *
 * `refreshSignal` re-runs the subscription setup (re-scanning
 * `getAllActiveWorkspaceIds()`) whenever it changes, independent of
 * `workspaceIds` — the caller passes its own `workingSignal` so a tree
 * workspace flipping `working` (the most common trigger for "a workspace
 * just got a store") re-scans for newly-active stores this effect would
 * otherwise never notice. This is a partial mitigation, not a full fix: a
 * workspace whose store is created without `workingSignal` also changing
 * (e.g. a chat opened into a pane that never starts a turn) still is not
 * picked up until some OTHER re-render happens — see space-scroller's own
 * `SpacePanel` comment and task-30-report.md for the full disclosure.
 */
function useRecentsTick(workspaceIds: string[], refreshSignal: string): void {
  const idsKey = workspaceIds.slice().sort().join(ID_DELIM)
  const [, setTick] = useState(0)
  useEffect(() => {
    const ids = idsKey ? idsKey.split(ID_DELIM) : []
    const active = new Set(getAllActiveWorkspaceIds())
    const unsubs = ids
      .filter((id) => active.has(id))
      .map((id) => {
        const store = getOrCreateWorkspaceStore(id)
        let prevSlice = recentsSlice(store.getState())
        return store.subscribe((state) => {
          const nextSlice = recentsSlice(state)
          if (nextSlice.working === prevSlice.working && nextSlice.chats === prevSlice.chats) {
            return
          }
          prevSlice = nextSlice
          setTick((t) => t + 1)
        })
      })
    // One window-level pane store (Task 26) — panes/dormantArrangements no
    // longer need a per-workspace subscription loop; any project's Recents
    // could be affected by a pane change anywhere, so this fires on every
    // pane/dormant-arrangement mutation regardless of `workspaceIds`.
    let prevPaneSlice = {
      panes: windowPaneStore.getState().panes,
      dormant: windowPaneStore.getState().dormantArrangements,
      activeView: windowPaneStore.getState().activeViewId,
    }
    unsubs.push(
      windowPaneStore.subscribe((state) => {
        if (
          state.panes === prevPaneSlice.panes &&
          state.dormantArrangements === prevPaneSlice.dormant &&
          // Recents is the VIEW SWITCHER, so which view is on screen is one of
          // the facts it draws (`RecentsEntry.showing`) — and switching views
          // touches neither of the other two: the panes are all still there,
          // unchanged, just hung on a different tree. Without this the "you
          // are here" marker stayed on whichever row happened to be showing
          // when `panes` last changed, which is a switcher that cannot tell
          // you where you are.
          state.activeViewId === prevPaneSlice.activeView
        ) {
          return
        }
        prevPaneSlice = {
          panes: state.panes,
          dormant: state.dormantArrangements,
          activeView: state.activeViewId,
        }
        setTick((t) => t + 1)
      }),
    )
    return () => unsubs.forEach((u) => u())
  }, [idsKey, refreshSignal])
}

interface SpacePanelProps {
  project: Project
  rowsForProject: (projectId: string) => SidebarRow[]
  recentsForProject: (projectId: string) => RecentsBandEntry[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  onFocusRecent: (entry: RecentsBandEntry) => void
  onCloseRecent: (entry: RecentsBandEntry) => void
  onDrop: (subjects: SidebarRow[], target: SidebarRow, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void
  onTrashProject: (projectId: string) => void
}

function SpacePanel({
  project,
  rowsForProject,
  recentsForProject,
  onOpen,
  onTrash,
  onCreate,
  onFocusRecent,
  onCloseRecent,
  onDrop,
  onPaneDrop,
  // Not read here any more — addendum §4 removed the project overflow's
  // Delete item, and deletion's only path is drag-to-trash now. Kept in
  // `SpacePanelProps` (and threaded through by `SpaceScroller` below) since
  // `SidebarTreeSurface` still supplies it and a future overflow verb may
  // want the same anchor this component already owns.
}: SpacePanelProps) {
  const projectId = project.id
  const repoRows = rowsForProject(projectId)
  // The REAL project-home workspace — project-scoped, not repo-scoped
  // (home-workspace-resolver.ts: "home is a project-level concept, not a
  // repo workspace"). Target for the header's Thread button and the
  // add-menu's "Create a folder" item — NOT a repo's own home row (a
  // different workspace entirely; conflating the two was a real bug, a
  // "New thread on the project" button that silently created the thread
  // under a REPO instead, caught live).
  const { wsId: homeWorkspaceId } = useHomeWorkspaceState(projectId)
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
  useEffect(() => {
    if (homeWorkspaceId) recordWorkspaceScope({ projectId, repoId: '', wsId: homeWorkspaceId })
  }, [projectId, homeWorkspaceId])
  // This project's home chats/folders — the same two aggregates a repo's
  // OWN tree holds, kept in their own per-project store (home-tree.ts) since
  // project home rides no repo. Rendered as FLAT TOP-LEVEL rows, exactly
  // like a repo itself — explicit user correction: `rowsFromHome` first drew
  // a container "Home" row for these to nest under, mirroring a repo's own
  // home row, and it was rejected outright; there is no such container here.
  const homeTree = useHomeTreeStore((s) => s.trees[projectId])
  // `rowsFromHome` THROWS if its owning branch chat is missing (same
  // contract `rowsFromRepo` holds a repo's own home row to) — guarded here
  // rather than there, the same way `SidebarTreeSurface`'s `seededRepoIds`
  // keeps a repo's rows from being built before ITS seed has landed: the
  // backfill that mints project home's owning chat is a daemon-side race
  // against this store's own first GET, not a caller error.
  const homeSeeded =
    homeWorkspaceId !== null &&
    !!homeTree?.chats.some((c) => c.type === 'branch' && c.workspaceId === homeWorkspaceId)
  // A repo header row already in `repoRows` (rowsFromRepo's own push) carries
  // its own real `parentId`/`order` straight off the wire — Task 3 put a
  // repo's position on its own `Node` row, computed server-side against
  // these SAME real home chat/folder siblings, so it needs no correction
  // here any more: it interleaves into the same sibling sort as
  // `rowsFromHome`'s rows just by sitting in the same flat list, exactly the
  // way a chat or folder row already does.
  const homeRows =
    homeSeeded && homeTree ? rowsFromHome(homeWorkspaceId, homeTree.chats, homeTree.folders) : []
  const rows = [...homeRows, ...repoRows]
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
  // Narrow, project-scoped selector (not the whole `repos` array) that
  // changes whenever a tree workspace under this project starts/stops
  // working. Read for its own re-render (a fresh string forces this
  // component to re-render when it changes) AND handed to useRecentsTick
  // below to re-scan `getAllActiveWorkspaceIds()` — a workspace's store
  // being created for the first time this session most commonly coincides
  // with its first chat starting to work, so this is what usually catches
  // it. It is a partial mitigation, not a guarantee: a workspace whose
  // store appears WITHOUT `working` also flipping (e.g. a pane opened onto
  // an already-idle/dormant chat) is still missed until some unrelated
  // re-render happens — there is no general cross-store live-aggregation
  // primitive in this codebase to close that gap fully (see
  // task-30-report.md).
  const workingSignal = useSidebarStore((s) =>
    s.repos
      .filter((r) => r.projectId === projectId)
      .flatMap((r) => r.workspaces.map((w) => `${w.id}${ID_DELIM}${w.working ? 1 : 0}`))
      .join(ID_DELIM),
  )
  const workspaceIds = Array.from(
    new Set(rows.map((r) => r.workspaceId).filter((id): id is string => id != null)),
  )
  useRecentsTick(workspaceIds, workingSignal)
  const entries = recentsForProject(projectId)

  // Spec §4: "clicking the header folds the space: the tree goes, Recents
  // stays. That is the point of folding — to see nothing but what is up in
  // this project. They share one scroller, so the fold hides the ROWS, not
  // the scroller." Local per-panel state, not persisted: nothing in the spec
  // asks a fold to survive a reload, and each project's panel is its own
  // component instance (keyed by project id), so "per space" is free.
  const [folded, setFolded] = useState(false)

  // The add-menu's anchor. `SpaceHeader.onOpenAddMenu` is a bare callback
  // with no event (its own reviewed signature), so the menu is positioned off
  // the header's own box rather than the pointer — which is also the more
  // correct anchor for a control that can be reached by keyboard.
  const headerRef = useRef<HTMLDivElement>(null)
  const menu = useContextMenu()
  const [addRepoOpen, setAddRepoOpen] = useState(false)

  return (
    <div
      data-testid="space-panel"
      className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
    >
      {/* flex: none, above the scroller — spec §2's own layout diagram. */}
      <div ref={headerRef} className="shrink-0">
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
            if (!homeWorkspaceId) return
            void handleCreateHomeThread(projectId, homeWorkspaceId, navigate)
          }}
          onOpenAddMenu={() => {
            const rect = headerRef.current?.getBoundingClientRect()
            menu.openAt({ x: rect ? rect.right - 8 : 0, y: rect ? rect.bottom : 0 })
          }}
        />
      </div>
      {/* Addendum §4's "the dropdown never carries a Delete item" still holds
          — deletion is reachable only through drag-to-trash (addendum §2).
          This is the verb that empty menu was left wired for: import another
          repo into this project, or start a folder on the project's OWN home
          workspace (the backend's `/home/chats/folders` mount — folders were
          once thought repo-internal only; they are not). */}
      {menu.isOpen && (
        <ContextMenu
          isOpen
          items={[
            {
              id: 'import-repo',
              label: 'Import a repo',
              icon: <FolderOpen />,
              onClick: () => setAddRepoOpen(true),
            },
            {
              id: 'new-folder',
              label: 'Create a folder',
              icon: <FolderIcon />,
              onClick: () => void performCreateHomeFolder(projectId),
            },
          ]}
          position={menu.position}
          onClose={menu.close}
        />
      )}
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
            entries={entries}
            onFocus={onFocusRecent}
            onClose={onCloseRecent}
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
  recentsForProject,
  onOpen,
  onTrash,
  onCreate,
  onFocusRecent,
  onCloseRecent,
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
    el.scrollTo({ left: index * el.clientWidth, behavior: 'smooth' })
  }, [activeProjectId, projects])

  // Sync activeProjectId when the user swipes/wheels.
  function handleScroll() {
    if (!isUserGesture.current) return
    const el = containerRef.current
    if (!el || el.clientWidth === 0) return
    const index = Math.round(el.scrollLeft / el.clientWidth)
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
          recentsForProject={recentsForProject}
          onOpen={onOpen}
          onTrash={onTrash}
          onCreate={onCreate}
          onFocusRecent={onFocusRecent}
          onCloseRecent={onCloseRecent}
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={onTrashProject}
        />
      ))}
    </div>
  )
}
