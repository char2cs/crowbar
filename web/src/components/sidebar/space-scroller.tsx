import { useEffect, useRef, useState } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { SidebarTree } from './sidebar-tree'
import { RecentsBand, type RecentsBandEntry } from './recents-band'
import { useSidebarStore } from '@/lib/store/sidebar'
import {
  getAllActiveWorkspaceIds,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { WorkspaceState } from '@/features/workspace/stores/workspace-store.types'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { Project } from '@/lib/types'

interface SpaceScrollerProps {
  projects: Project[]
  /** Undefined on a route with no project in it yet (matches
   *  `SidebarProjectHeader`'s own `activeProjectId?: string` — the two read
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
}

/** The four `WorkspaceState` fields Recents actually reads, compared by
 *  reference below (immer only replaces a slice's reference when that slice
 *  was actually mutated) - mirrors workspace-store-registry.ts's own
 *  persistence-subscribe idiom, so an unrelated store write (editor buffers,
 *  terminal output) never triggers a recompute. */
function recentsSlice(state: WorkspaceState) {
  return {
    panes: state.panes,
    working: state.agentChats.working,
    chats: state.agentChats.chats,
    dormant: state.dormantArrangements,
  }
}

/**
 * Re-renders the caller whenever a currently-active workspace's panes,
 * working chats, chat list or dormant arrangements change, so a project's
 * Recents band recomputes with fresh data - a plain `.subscribe(listener)`
 * fires on EVERY store mutation, so the listener drops anything that did not
 * touch one of the four fields above.
 *
 * `workspaceIds` need not already be filtered to active ones: subscribing is
 * skipped for an id with no live store rather than creating one (see
 * recents-for-project.ts for why calling `getOrCreateWorkspaceStore` on a
 * never-opened workspace would leak it).
 */
function useRecentsTick(workspaceIds: string[]): void {
  const idsKey = workspaceIds.slice().sort().join(' ')
  const [, setTick] = useState(0)
  useEffect(() => {
    const ids = idsKey ? idsKey.split(' ') : []
    const active = new Set(getAllActiveWorkspaceIds())
    const unsubs = ids
      .filter((id) => active.has(id))
      .map((id) => {
        const store = getOrCreateWorkspaceStore(id)
        let prevSlice = recentsSlice(store.getState())
        return store.subscribe((state) => {
          const nextSlice = recentsSlice(state)
          if (
            nextSlice.panes === prevSlice.panes &&
            nextSlice.working === prevSlice.working &&
            nextSlice.chats === prevSlice.chats &&
            nextSlice.dormant === prevSlice.dormant
          ) {
            return
          }
          prevSlice = nextSlice
          setTick((t) => t + 1)
        })
      })
    return () => unsubs.forEach((u) => u())
  }, [idsKey])
}

interface SpacePanelProps {
  projectId: string
  rowsForProject: (projectId: string) => SidebarRow[]
  recentsForProject: (projectId: string) => RecentsBandEntry[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  onFocusRecent: (entry: RecentsBandEntry) => void
  onCloseRecent: (entry: RecentsBandEntry) => void
}

function SpacePanel({
  projectId,
  rowsForProject,
  recentsForProject,
  onOpen,
  onTrash,
  onCreate,
  onFocusRecent,
  onCloseRecent,
}: SpacePanelProps) {
  const rows = rowsForProject(projectId)
  // Re-scanning "which workspace under this project is working" on every
  // backend push (already narrow, project-scoped - not the whole `repos`
  // array) is what catches a workspace's store being created for the FIRST
  // time this session while its panel is not otherwise re-rendering: the
  // transition to "working" is what makes THIS project panel re-render and
  // pick the new store up.
  const workingSignal = useSidebarStore((s) =>
    s.repos
      .filter((r) => r.projectId === projectId)
      .flatMap((r) => r.workspaces.map((w) => `${w.id}:${w.working ? 1 : 0}`))
      .join('|'),
  )
  const workspaceIds = Array.from(
    new Set(rows.map((r) => r.workspaceId).filter((id): id is string => id != null)),
  )
  // `workingSignal` is read only to force a re-render on change (see comment
  // above); this line exists to keep eslint's exhaustive-deps happy without
  // pretending the value itself is used.
  void workingSignal
  useRecentsTick(workspaceIds)
  const entries = recentsForProject(projectId)

  return (
    <div
      data-testid="space-panel"
      className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
    >
      <ScrollArea className="flex-1">
        <SidebarTree rows={rows} onOpen={onOpen} onTrash={onTrash} onCreate={onCreate} />
        <RecentsBand entries={entries} onFocus={onFocusRecent} onClose={onCloseRecent} />
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
      className="flex flex-1 overflow-x-scroll overflow-y-hidden [scroll-snap-type:x_mandatory] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {projects.map((project) => (
        <SpacePanel
          key={project.id}
          projectId={project.id}
          rowsForProject={rowsForProject}
          recentsForProject={recentsForProject}
          onOpen={onOpen}
          onTrash={onTrash}
          onCreate={onCreate}
          onFocusRecent={onFocusRecent}
          onCloseRecent={onCloseRecent}
        />
      ))}
    </div>
  )
}
