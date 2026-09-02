import { useEffect, useRef, useState } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useContextMenu } from '@/components/ui/context-menu'
import { SidebarTree } from './sidebar-tree'
import { SpaceHeader } from './space-header'
import { RecentsBand, type RecentsBandEntry } from './recents-band'
import { CARD_BOTTOM_INSET_VAR } from '@/components/layout/sidebar-card-height'
import { findScrollParent } from '@/components/layout/edge-scroll'
import { useSidebarStore } from '@/lib/store/sidebar'
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
    }
    unsubs.push(
      windowPaneStore.subscribe((state) => {
        if (state.panes === prevPaneSlice.panes && state.dormantArrangements === prevPaneSlice.dormant) {
          return
        }
        prevPaneSlice = { panes: state.panes, dormant: state.dormantArrangements }
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
  const rows = rowsForProject(projectId)
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

  // The overflow menu's anchor. `SpaceHeader.onOverflow` is a bare callback
  // with no event (its own reviewed signature), so the menu is positioned off
  // the header's own box rather than the pointer — which is also the more
  // correct anchor for a control that can be reached by keyboard.
  const headerRef = useRef<HTMLDivElement>(null)
  const menu = useContextMenu()

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
          onOverflow={() => {
            const rect = headerRef.current?.getBoundingClientRect()
            menu.openAt({ x: rect ? rect.right - 8 : 0, y: rect ? rect.bottom : 0 })
          }}
        />
      </div>
      {/* Addendum §4: "the dropdown never carries a Delete item" — deletion is
          reachable only through drag-to-trash (addendum §2) now. The project
          overflow currently has nothing else §4 names for it either
          (rename/lock/import are row verbs with a home already in
          row-context-menu.tsx), so there is no menu left to open here yet —
          `onOverflow`/`menu` stay wired for the next verb this surface gets. */}
      <ScrollArea className="flex-1">
        {/* Padding lives on the scrollable CONTENT, not the ScrollArea root —
            only that extends how far the region actually scrolls, which is
            the whole point of the inset (spec §6). Reads `--card-bottom-inset`
            straight off the CSS cascade rather than a prop: that variable is
            written directly onto the shared rail ancestor by
            sidebar-carousel.tsx (see ide-shell.tsx's `railRef`), including
            once per animation frame during a resize drag — routing that
            through a React prop here would re-render this panel (and every
            row in it) on every one of those frames. The `0px` fallback
            covers the one render before the card has measured anything. */}
        <div
          ref={contentRef}
          data-testid="space-scroll-content"
          style={{ paddingBottom: `var(${CARD_BOTTOM_INSET_VAR}, 0px)` }}
        >
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
      onWheel={armUserGesture}
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
