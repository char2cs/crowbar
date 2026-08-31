import { useCallback, useMemo, useRef, useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { ErrorBoundary } from '@/components/error-boundary'
import { SpaceScroller } from '@/components/sidebar/space-scroller'
import { rowsForProject } from '@/components/sidebar/lib/rows-for-project'
import { recentsForProject } from '@/components/sidebar/lib/recents-for-project'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { focusRecent, closeRecent } from '@/components/sidebar/lib/recents-actions'
import { DeleteConfirmDialog } from '@/components/sidebar/delete-confirm-dialog'
import type { RecentsBandEntry } from '@/components/sidebar/recents-band'
import {
  resolveRow,
  resolveChatRow,
  handleOpen as openSidebarRow,
  handleTrash as trashSidebarRow,
  handleTrashProject as trashProject,
  handleCreate as createSidebarRow,
} from './space-content-actions'
import { applyPendingRemovals } from './removal-plan'
import { performSidebarDrop, performSidebarPaneDrop } from '@/components/sidebar/lib/drop-actions'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useSidebarStore } from '@/lib/store/sidebar'
import { toast } from '@/features/window/stores/toast-store'
import { SidebarTreeChrome } from './sidebar-tree-chrome'
import type { Project } from '@/lib/types'

interface SidebarTreeSurfaceProps {
  projects: Project[]
  activeProjectId: string | undefined
  onActiveProjectChange: (id: string) => void
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
}: SidebarTreeSurfaceProps) {
  const navigate = useNavigate()
  // Every project's rows come off the SAME removal-tray-filtered `repos` the
  // old SidebarTreePanel derived once for its single flat tree: a row held
  // for removal must disappear from whichever project's panel renders it,
  // exactly as it disappeared from the one flat tree before.
  const allRepos = useSidebarStore((s) => s.repos)
  const hiddenIds = useRemovalTrayStore((s) => s.hiddenIds)
  const repos = useMemo(() => applyPendingRemovals(allRepos, hiddenIds), [allRepos, hiddenIds])
  // Every row across every project — SidebarRowContextMenu/RenameDialog look
  // up a row by id regardless of which project's panel drew it (a row's id
  // is never ambiguous by project), so the chrome mounted once below needs
  // the whole set, not any one project's slice.
  const allRows = useMemo(() => repos.flatMap(rowsFromRepo), [repos])

  const rowsForProjectFn = useCallback(
    (projectId: string) => rowsForProject(repos, projectId),
    [repos],
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
  // Spec §9's project-level trash, reached from the space header's overflow.
  // Says so rather than doing nothing when the tray refuses to hold the
  // project (a project id no loaded project claims) — the same posture the
  // row trash's own confirm takes.
  const handleTrashProject = useCallback((projectId: string) => {
    if (trashProject(projectId)) return
    toast.error("Can't delete this project — it may already be gone")
  }, [])
  // The right-click menu listens on an ancestor of every project's panel DOM
  // (via native `contextmenu` bubbling), not on any one panel — a single
  // listener here catches a right-click in ANY project's rows.
  const treeRef = useRef<HTMLDivElement>(null)

  // A trash click no longer deletes directly (Task 8/22's `handleTrash`
  // still does the real work, now only from this dialog's onConfirm): spec
  // §9 requires an idle delete to confirm and a working one to refuse
  // outright, so the click just names the pending row and
  // `DeleteConfirmDialog` decides which of those it gets.
  const [deletingRowId, setDeletingRowId] = useState<string | null>(null)
  const deletingRow = allRows.find((r) => r.id === deletingRowId) ?? null
  // `resolveRow` cannot see a chat (`resolveChatRow`'s own doc: "callers must
  // consult THIS FIRST" — same order `handleOpen` above already uses), so a
  // chat's delete-preview would otherwise get no `projectId` and degrade to
  // the fallback copy instead of the real file/chat counts.
  const deletingRepo =
    deletingRowId == null
      ? undefined
      : (resolveChatRow(repos, deletingRowId)?.repo ?? resolveRow(repos, deletingRowId)?.repo)

  return (
    <div ref={treeRef} className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <ErrorBoundary>
        <SpaceScroller
          projects={projects}
          activeProjectId={activeProjectId}
          onActiveProjectChange={onActiveProjectChange}
          rowsForProject={rowsForProjectFn}
          recentsForProject={recentsForProjectFn}
          onOpen={openRow}
          onTrash={setDeletingRowId}
          onCreate={createSidebarRow}
          onFocusRecent={focusRecentEntry}
          onCloseRecent={closeRecent}
          onDrop={performSidebarDrop}
          onPaneDrop={performSidebarPaneDrop}
          onTrashProject={handleTrashProject}
        />
      </ErrorBoundary>
      <SidebarTreeChrome treeRef={treeRef} rows={allRows} repos={repos} />
      <DeleteConfirmDialog
        open={deletingRow != null}
        label={deletingRow?.label ?? ''}
        working={deletingRow?.working ?? false}
        projectId={deletingRepo?.projectId}
        repoId={deletingRepo?.id ?? ''}
        chatId={deletingRowId ?? ''}
        onOpenChange={(open) => {
          if (!open) setDeletingRowId(null)
        }}
        onConfirm={() => {
          if (!deletingRowId) return
          // A locked (non-home) workspace's own trash button still shows —
          // Task 25 review round 1's Important finding — so a confirm can
          // walk the user through a real preview and then find zero drafts
          // to hold (`handleTrash`'s own doc comment). That can no longer be
          // silent now that the click already implied a real delete was
          // about to happen.
          const held = trashSidebarRow(deletingRowId)
          if (!held) {
            toast.error(`Can't delete ${deletingRow?.label ?? 'this row'} — it may be locked`)
          }
        }}
      />
    </div>
  )
}
