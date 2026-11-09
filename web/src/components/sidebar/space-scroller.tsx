import { lazy, Suspense, useEffect, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ActiveSessionsSection } from './active-sessions-section'
import { SidebarTree } from './sidebar-tree'
import { SpaceHostScope } from './space-host-scope'
import { TreeFoot } from './tree-foot'
import { UnreadNotice } from './unread-notice'
import { findScrollParent } from '@/components/layout/edge-scroll'
import { performCreateHomeFolder } from '@/components/sidebar/lib/row-actions'
import { useHomeWorkspaceState } from '@/features/workspace/lib/home-workspace-resolver'
import { handleCreateHomeThread } from '@/components/layout/home-actions'
import { toast } from '@/features/window/stores/toast-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { DropMode } from '@/components/tree-dnd/drop-core'
import type { SidebarPaneZone } from '@/components/sidebar/lib/sidebar-drag-hover'
import type { Project } from '@/lib/types'

// Lazy so the dialog and its folder pickers load after first paint, not with the boot bundle.
const AddRepositoryModal = lazy(() =>
  import('@/components/projects/add-repository-modal').then((m) => ({
    default: m.AddRepositoryModal,
  })),
)

interface SpaceScrollerProps {
  projects: Project[]
  /** Undefined on a route with no project yet; the same value SidebarFooter reads. */
  activeProjectId: string | undefined
  onActiveProjectChange: (id: string) => void
  /** Every row the project's panel draws (`deriveRows`). */
  rowsForProject: (projectId: string) => SidebarRow[]
  onOpen: (id: string) => void
  onTrash: (id: string) => void
  onCreate: (parentId: string, kind: 'workspace' | 'thread') => void
  onDrop: (subjects: SidebarRow[], target: SidebarRow, mode: DropMode) => void
  onPaneDrop: (subjects: SidebarRow[], paneId: string, zone: SidebarPaneZone) => void
  /** Spec §9's project-level trash behind the tree foot's Delete space; a prop, so this file
   *  stays presentational. */
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
  // The edge-scroll target must be the real overflow element (ScrollArea's Viewport), not the
  // content div a plain ref would land on.
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
      <Suspense fallback={null}>
        <AddRepositoryModal
          open={addRepoOpen}
          onOpenChange={setAddRepoOpen}
          projectId={project.id}
        />
      </Suspense>
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

/** One horizontal, x-mandatory-snap panel per project (spec §4), each one scroll region. */
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
  // The vertical chat list lives inside this carousel, so its wheel ticks bubble here too
  // (onWheel doesn't filter by axis). Only a horizontally dominant gesture is a swipe; anything
  // else arming this would let an unrelated scrollLeft nudge swap the active project.
  const armUserGestureFromWheel = (e: React.WheelEvent) => {
    if (Math.abs(e.deltaX) <= Math.abs(e.deltaY)) return
    isUserGesture.current = true
  }
  // The panel index a smooth programmatic scroll is travelling to. Its scroll events aren't
  // gestures, and re-aligning on them would cancel the animation — a snap instead of a glide.
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
      // A collapsed sidebar has zero width and no panel offset; the resize that reopens it
      // re-aligns.
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
    // Already there: scrollTo fires no scroll event, so arming would disarm re-align for good.
    if (el.scrollLeft === left) {
      settlingToIndex.current = null
      return
    }
    settlingToIndex.current = index
    el.scrollTo({ left, behavior: 'smooth' })
  }, [activeProjectId, projects])

  // A swipe picks the active project; any other scroll is held to the active project's panel.
  function handleScroll() {
    const el = containerRef.current
    if (!el || el.clientWidth === 0) return
    const index = Math.round(el.scrollLeft / el.clientWidth)
    if (!isUserGesture.current) {
      if (settlingToIndex.current !== null) {
        if (settlingToIndex.current === index) settlingToIndex.current = null
        return
      }
      // Not a swipe and nothing programmatic in flight (focus's scrollIntoView, edge-scroll, a
      // WebKit snap): mandatory snapping turns any of these into a whole panel, so bounce back
      // rather than switch to a project the user never asked for.
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
      // During a row drag a captured pointer past this edge would get WebKit to snap a whole
      // panel away; index.css pins the carousel via this attribute while `data-row-dragging`
      // is set (with `will-change`: without it the pin costs 4x the frame budget).
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
