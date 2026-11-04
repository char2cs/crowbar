import { useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ActiveSessionsSection } from './active-sessions-section'
import { SidebarTree } from './sidebar-tree'
import { SpaceHostScope } from './space-host-scope'
import { TreeFoot } from './tree-foot'
import { UnreadNotice } from './unread-notice'
import { findScrollParent } from '@/components/layout/edge-scroll'
import { performCreateHomeFolder } from '@/components/sidebar/lib/row-actions'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { toast } from '@/features/window/stores/toast-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/lib/sidebar-drag-hover'
import type { Project } from '@/lib/types'

interface SpaceScrollerProps {
  projects: Project[]
  /** Undefined on a route with no project in it yet (matches
   *  `SidebarFooter`'s own `activeProjectId?: string` — the two read
   *  the SAME value, ide-shell.tsx's `activeProjectIdFromRoute`). */
  activeProjectId: string | undefined
  onActiveProjectChange: (id: string) => void
  /** Every row the project's panel draws (`deriveRows`). */
  rowsForProject: (projectId: string) => SidebarRow[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  onDrop: (subjects: SidebarRow[], target: SidebarRow, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void
  /** Spec §9's project-level trash, behind the tree foot's Delete space.
   *  Threaded as a prop like every other verb here rather than
   *  called directly, so this file stays presentational. */
  onTrashProject: (projectId: string) => void
}

interface SpacePanelProps {
  project: Project
  rowsForProject: (projectId: string) => SidebarRow[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
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
  onDrop,
  onPaneDrop,
  onTrashProject,
}: SpacePanelProps) {
  const projectId = project.id
  const rows = rowsForProject(projectId)
  // The project-home workspace: the tree foot's New thread and folder act on it, never a repo's home.
  const { wsId: homeWorkspaceId } = useHomeWorkspaceState(projectId)
  const navigate = useNavigate()
  // `useSidebarDrag` resolves its edge-scroll target off this ref, which
  // points at the actual overflow element (`ScrollAreaPrimitive.Viewport`),
  // not the plain content div a naive ref here would otherwise land on.
  const contentRef = useRef<HTMLDivElement>(null)
  const viewportRef = useRef<HTMLElement | null>(null)
  useEffect(() => {
    viewportRef.current = findScrollParent(contentRef.current)
  })

  const [addRepoOpen, setAddRepoOpen] = useState(false)
  // Not `onCreate`/`handleCreate`: that resolves against the repo-scoped
  // store, which has no project home. A click before home resolves says so.
  const createHomeThread = () => {
    if (!homeWorkspaceId) {
      toast.error("Can't start a new thread yet")
      return
    }
    void handleCreateHomeThread(projectId, homeWorkspaceId, navigate)
  }

  return (
    <div
      data-testid="space-panel"
      className="min-w-full [scroll-snap-align:start] flex flex-col overflow-hidden"
    >
      <AddRepositoryModal open={addRepoOpen} onOpenChange={setAddRepoOpen} projectId={project.id} />
      <ScrollArea className="flex-1">
        <div ref={contentRef} data-testid="space-scroll-content">
          <SpaceHostScope hostId={project.hostId}>
            <UnreadNotice projectId={projectId} />
            <ActiveSessionsSection
              rows={rows}
              onOpen={onOpen}
              onTrash={onTrash}
              onCreate={onCreate}
            />
            <SidebarTree
              rows={rows}
              onOpen={onOpen}
              onTrash={onTrash}
              onCreate={onCreate}
              scrollRef={viewportRef}
              onDrop={onDrop}
              onPaneDrop={onPaneDrop}
            />
            <TreeFoot
              onNewThread={createHomeThread}
              onImportRepo={() => setAddRepoOpen(true)}
              onCreateFolder={() => void performCreateHomeFolder(projectId)}
              onDeleteSpace={() => onTrashProject(projectId)}
            />
          </SpaceHostScope>
        </div>
      </ScrollArea>
    </div>
  )
}

/**
 * One horizontal, x-mandatory-snap panel per project (spec §4).
 *
 * Each panel is ONE scroll region holding a project's `SidebarTree`.
 */
export function SpaceScroller({
  projects,
  activeProjectId,
  onActiveProjectChange,
  rowsForProject,
  onOpen,
  onTrash,
  onCreate,
  onDrop,
  onPaneDrop,
  onTrashProject,
}: SpaceScrollerProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  // Armed only by an actual scroll gesture over the scroller. Everything else
  // that moves scrollLeft is reflow (a sidebar resize, a hide/show), not intent.
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
      // `overflow-x: scroll` + mandatory x
      // snapping + min-w-full panels means a captured pointer travelling
      // past this box's edge during a row drag gets scrolled a whole panel
      // away by WebKit — the exact gesture Part G exists for (dragging a row
      // rightward onto a pane). `index.css`'s `html[data-row-dragging]
      // [data-sidebar-carousel]` rule pins it by this
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
          onDrop={onDrop}
          onPaneDrop={onPaneDrop}
          onTrashProject={onTrashProject}
        />
      ))}
    </div>
  )
}
