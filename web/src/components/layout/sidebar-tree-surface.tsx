import { useCallback, useMemo, useRef } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { SpaceScroller } from '@/components/sidebar/space-scroller'
import { rowsForProject } from '@/components/sidebar/lib/rows-for-project'
import { recentsForProject } from '@/components/sidebar/lib/recents-for-project'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { rowsFromHome } from '@/components/sidebar/lib/rows-from-home'
import { rowsFromPending } from '@/components/sidebar/lib/rows-from-pending'
import { usePendingCreatesStore } from '@/lib/store/pending-creates'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { getHomeWorkspaceId } from '@/features/workspace/lib/home-workspace-resolver'
import { focusRecent, closeRecent } from '@/components/sidebar/lib/recents-actions'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'
import {
  handleOpen as openSidebarRow,
  handleTrash,
  handleTrashProject as trashProject,
  handleCreate as createSidebarRow,
} from './space-content-actions'
import { applyPendingRemovals, attachRemovalState, descendantHiddenIds } from './removal-plan'
import { performSidebarDrop, performSidebarPaneDrop } from '@/components/sidebar/lib/drop-actions'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { toast } from '@/features/window/stores/toast-store'
import { SidebarTreeChrome } from './sidebar-tree-chrome'
import { CreateSpacePanel } from './create-space-panel'
import type { Project } from '@/lib/types'

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
 * SpaceScroller's real mount point, plus the chrome (RemovalTray,
 * RenameDialog, RepoImportDialog, SidebarRowContextMenu) it needs mounted
 * once alongside it — split out of `ide-shell.tsx` itself
 * rather than inlined there, on purpose: `ide-shell.tsx`'s OWN comment on
 * `sidebarWorkspacePath` (a few lines up from where this used to be wired)
 * already establishes why — "subscribing to the whole repos array made
 * every live status/count frame rebuild the complete IDE shell." The tree
 * genuinely needs the full `repos` array (there is no narrower shape that
 * still builds every project's rows), so that subscription has to live
 * somewhere; isolating it here means a git-status/PR tick re-renders this
 * component alone, not `IDEShell` and everything under it.
 */
export function SidebarTreeSurface({
  projects,
  activeProjectId,
  onActiveProjectChange,
  creatingSpace = false,
  onCreateSpace = () => {},
  onCancelCreateSpace = () => {},
}: SidebarTreeSurfaceProps) {
  const navigate = useNavigate()
  // Every project's rows come off the SAME removal-tray-filtered `repos` the
  // old SidebarTreePanel derived once for its single flat tree: a row held
  // for removal must disappear from whichever project's panel renders it,
  // exactly as it disappeared from the one flat tree before.
  const allRepos = useSidebarStore((s) => s.repos)
  const removalEntries = useRemovalTrayStore((s) => s.entries)
  // A held row's own PRIMARY id no longer needs hiding — it stays on screen,
  // transformed in place (sidebar-row.tsx's `RemovingSidebarRow`) — only its
  // cascade descendants still vanish outright (removal-plan.ts's own doc).
  const hiddenIds = useMemo(() => descendantHiddenIds(removalEntries), [removalEntries])
  const repos = useMemo(() => applyPendingRemovals(allRepos, hiddenIds), [allRepos, hiddenIds])
  // ROWS come only from repos whose tree has actually been read back.
  //
  // A row's identity is the chat that owns its workspace, and a repo's chats
  // arrive on their own reseed loop — independent of the repo and workspace
  // streams that put `repos` above into the store. Drawing during that window
  // would give every row an id that changes the moment the seed lands, and a
  // row id is the React key, the `collapsedWorkspaces` key and the selection
  // key: the tree would silently drop the user's folds and selection a beat
  // after painting. So a repo's rows appear once, with final ids. Everything
  // else here still reads the FULL `repos` — resolving, opening and dropping a
  // row are questions about the repo, not about its rows.
  const seededRepoIds = useFolderSignalStore((s) => s.seededRepoIds)
  const treeRepos = useMemo(
    () => repos.filter((r) => seededRepoIds.has(r.id)),
    [repos, seededRepoIds],
  )
  // Every row across every project — SidebarRowContextMenu/RenameDialog look
  // up a row by id regardless of which project's panel drew it (a row's id
  // is never ambiguous by project), so the chrome mounted once below needs
  // the whole set, not any one project's slice.
  //
  // Repo rows are not the whole story: a project-home chat or folder is a row
  // too (`space-scroller.tsx`'s own `SpacePanel` draws it via the identical
  // `rowsFromHome`), and until it is ALSO in this set, `SidebarRowContextMenu`
  // silently finds no row for one and returns before even opening — a home
  // folder never gets a right-click menu at all, caught live (no "New
  // folder", no "Rename", nothing — not a refusal, just no menu).
  const homeTrees = useHomeTreeStore((s) => s.trees)
  const homeRows = useMemo(
    () =>
      projects.flatMap((project) => {
        const homeWorkspaceId = getHomeWorkspaceId(project.id)
        const homeTree = homeTrees[project.id]
        // Mirrors `space-scroller.tsx`'s own `homeSeeded` guard: `rowsFromHome`
        // degrades gracefully (never throws) while its owning chat has not
        // resolved yet, so this only needs the tree itself to have seeded.
        if (!homeWorkspaceId || !homeTree) return []
        // A held home chat/folder's cascade descendants disappear the same
        // way a held repo row's do via `applyPendingRemovals` above —
        // filtered here rather than there, since a home tree is never part
        // of `repos` for that projection to reach at all. The PRIMARY id
        // stays (same `descendantHiddenIds` rule); `attachRemovalState`
        // below marks it for the in-place transform.
        return attachRemovalState(
          rowsFromHome(
            homeWorkspaceId,
            homeTree.chats.filter((c) => !hiddenIds.has(c.id)),
            homeTree.folders.filter((f) => !hiddenIds.has(f.id)),
          ),
          removalEntries,
        )
      }),
    [projects, homeTrees, hiddenIds, removalEntries],
  )
  // Every create in flight, drawn as a real row at the exact slot the
  // finished create lands in (pending-creates.ts) — merged in here, the one
  // place both `allRows` (chrome) and `rowsForProjectFn` (the actual render
  // path) already converge, so neither needs its own copy of this logic.
  const pendingEntries = usePendingCreatesStore((s) => s.entries)
  const pendingRows = useMemo(() => rowsFromPending(pendingEntries), [pendingEntries])
  // Marked for the SAME in-place transform as home rows above — computed
  // once here and reused by both `allRows` (chrome) and `rowsForProjectFn`
  // (the actual render path) rather than redone in each.
  const repoRows = useMemo(
    () => attachRemovalState(treeRepos.flatMap(rowsFromRepo), removalEntries),
    [treeRepos, removalEntries],
  )
  const allRows = useMemo(
    () => [...homeRows, ...repoRows, ...pendingRows],
    [homeRows, repoRows, pendingRows],
  )

  const rowsForProjectFn = useCallback(
    (projectId: string) => [
      ...attachRemovalState(rowsForProject(treeRepos, projectId), removalEntries),
      ...rowsFromPending(pendingEntries.filter((e) => e.projectId === projectId)),
    ],
    [treeRepos, removalEntries, pendingEntries],
  )
  const recentsForProjectFn = useCallback(
    (projectId: string) => recentsForProject(repos, projectId),
    [repos],
  )
  const openRow = useCallback(
    (id: string) => openSidebarRow(id, repos, navigate),
    [repos, navigate],
  )
  const focusRecentEntry = useCallback(
    (entry: RecentsBandEntry) => focusRecent(entry, repos, navigate),
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
            rowsForProject={rowsForProjectFn}
            recentsForProject={recentsForProjectFn}
            onOpen={openRow}
            onTrash={onTrash}
            onCreate={createSidebarRow}
            onFocusRecent={focusRecentEntry}
            onCloseRecent={closeRecent}
            onDrop={performSidebarDrop}
            onPaneDrop={performSidebarPaneDrop}
            onTrashProject={handleTrashProject}
          />
        )}
      </ErrorBoundary>
      <SidebarTreeChrome treeRef={treeRef} rows={allRows} repos={repos} />
    </div>
  )
}
