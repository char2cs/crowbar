import { memo, useCallback, useMemo, useRef } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { SpaceScroller } from '@/components/sidebar/space-scroller'
import { deriveRows } from '@/features/workspace/tree/rows'
import { useTreeInput } from '@/features/workspace/tree/input'
import { focusRecent, closeRecent, closeRecentChat } from '@/components/sidebar/lib/recents-actions'
import { handleOpen as openSidebarRow } from './open-actions'
import { handleTrash, handleTrashProject as trashProject } from './trash-actions'
import { handleCreate as createSidebarRow } from './create-actions'
import { applyPendingRemovals, renderedHiddenIds } from './removal-plan'
import { performSidebarDrop } from '@/components/sidebar/lib/drop-actions'
import { performSidebarPaneDrop } from '@/components/sidebar/lib/pane-drop-actions'
import { useTreeUi } from '@/features/workspace/tree/tree-ui-store'
import { useSidebarStore } from '@/lib/store/sidebar'
import { toast } from '@/features/window/stores/toast-store'
import { SidebarTreeChrome } from './sidebar-tree-chrome'
import { CreateSpacePanel } from './create-space-panel'
import type { Project } from '@/lib/types'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

const NO_ROWS: SidebarRow[] = []

interface SidebarTreeSurfaceProps {
  projects: Project[]
  activeProjectId: string | undefined
  onActiveProjectChange: (id: string) => void
  /** The `+` mark's own panel (create-space-panel.tsx) replaces SpaceScroller
   *  entirely while true — not a panel inside its scroll-snap track — so the
   *  touchpad swipe between real spaces can never land on it. Optional,
   *  default false: only `ide-shell.tsx`'s real mount cares, every other
   *  caller (tests) just wants the ordinary scroller. */
  creatingSpace?: boolean
  onCreateSpace?: (project: Project) => void
  onCancelCreateSpace?: () => void
}

/**
 * SpaceScroller's mount plus the chrome it needs once. Its own component so the store
 * subscriptions every row needs re-render it alone, never `IDEShell`; memoized for the reverse.
 */
export const SidebarTreeSurface = memo(function SidebarTreeSurface({
  projects,
  activeProjectId,
  onActiveProjectChange,
  creatingSpace = false,
  onCreateSpace = () => {},
  onCancelCreateSpace = () => {},
}: SidebarTreeSurfaceProps) {
  const navigate = useNavigate()
  // Opening, resolving and focusing a row ask about the repo the user sees: the
  // tray's held rows taken out, its live primary rows kept (`renderedHiddenIds`).
  const allRepos = useSidebarStore((s) => s.repos)
  const removalEntries = useTreeUi((s) => s.removals)
  const trayHiddenIds = useTreeUi((s) => s.hiddenIds)
  const repos = useMemo(
    () => applyPendingRemovals(allRepos, renderedHiddenIds(trayHiddenIds, removalEntries)),
    [allRepos, trayHiddenIds, removalEntries],
  )
  const inputFor = useTreeInput()
  const rowsByProject = useMemo(
    () => new Map(projects.map((p) => [p.id, deriveRows(inputFor(p.id))])),
    [projects, inputFor],
  )
  const rowsForProject = useCallback(
    (projectId: string) => rowsByProject.get(projectId) ?? NO_ROWS,
    [rowsByProject],
  )
  // The chrome (context menu, trash label) looks a row up by id whichever panel drew it.
  const allRows = useMemo(() => [...rowsByProject.values()].flat(), [rowsByProject])
  const openRow = useCallback(
    (id: string) => openSidebarRow(id, repos, navigate),
    [repos, navigate],
  )
  const createRow = useCallback(
    (parentId: string, kind: 'workspace' | 'thread') => createSidebarRow(parentId, kind, navigate),
    [navigate],
  )
  const focusRecentEntry = useCallback(
    (viewId: string) => focusRecent(viewId, repos, navigate),
    [repos, navigate],
  )
  // The row's own X control (sidebar-row.tsx) — the same `handleTrash` the
  // drag-to-trash gesture already calls, just reachable without a drag now.
  // Refuses (a locked branch, a repo home, a workspace the live store no
  // longer recognises) get the identical toast the drag path already shows,
  // read off `allRows` since `handleTrash` only answers true/false, never a
  // reason.
  const onTrash = useCallback(
    (id: string) => {
      if (handleTrash(id)) return
      const label = allRows.find((r) => r.id === id)?.label
      toast.error(`Can't delete ${label || 'this row'} yet`)
    },
    [allRows],
  )
  // Spec §9's project-level trash, reached from the space header's overflow.
  // Says so rather than doing nothing when the tray refuses to hold the
  // project (a project id no loaded project claims) — the same posture the
  // row trash's own confirm takes.
  const handleTrashProject = useCallback((projectId: string) => {
    if (trashProject(projectId)) return
    toast.error("Can't delete this space — it may already be gone")
  }, [])
  // The right-click menu listens on an ancestor of every project's panel DOM
  // (via native `contextmenu` bubbling), not on any one panel — a single
  // listener here catches a right-click in ANY project's rows.
  const treeRef = useRef<HTMLDivElement>(null)

  return (
    <div ref={treeRef} className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <ErrorBoundary>
        {creatingSpace ? (
          <CreateSpacePanel onCreate={onCreateSpace} onCancel={onCancelCreateSpace} />
        ) : (
          <SpaceScroller
            projects={projects}
            activeProjectId={activeProjectId}
            onActiveProjectChange={onActiveProjectChange}
            rowsForProject={rowsForProject}
            onOpen={openRow}
            onTrash={onTrash}
            onCreate={createRow}
            onFocusRecent={focusRecentEntry}
            onCloseRecent={closeRecent}
            onCloseChatRecent={closeRecentChat}
            onDrop={performSidebarDrop}
            onPaneDrop={performSidebarPaneDrop}
            onTrashProject={handleTrashProject}
          />
        )}
      </ErrorBoundary>
      <SidebarTreeChrome treeRef={treeRef} rows={allRows} repos={repos} />
    </div>
  )
})
