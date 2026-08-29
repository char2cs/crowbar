import { useCallback, useState, type RefObject } from 'react'
import { Plus } from 'lucide-react'
import { cn } from '@/lib/utils'
import { SidebarRowContextMenu } from '@/components/sidebar/row-context-menu'
import { RenameDialog } from '@/components/sidebar/rename-dialog'
import { performRenameRow, performImportBranches } from '@/components/sidebar/lib/row-actions'
import { resolveRow } from './space-content-actions'
import type { Repo } from '@/lib/store/sidebar'
import { importProjectAndSync } from '@/lib/store/projects'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { RemovalTray } from './removal-tray'
import { RepoImportDialog } from './repo-import-dialog'
import { ROW_BASE } from './workspace-row-base'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { Project } from '@/lib/types'

interface SidebarTreeChromeProps {
  /** An ancestor of every space panel's DOM, for the context menu's
   *  delegated `contextmenu` listener — one listener catches a right-click
   *  in ANY project's panel, not just whichever one owns `treeRef` itself. */
  treeRef: RefObject<HTMLElement | null>
  /** Every row across every project — rename/context-menu lookups are not
   *  scoped to one space (a row's id is unambiguous regardless of which
   *  project it renders under). */
  rows: SidebarRow[]
  /** Every repo across every project — resolves the repo behind a branch
   *  import dialog's row id. */
  repos: readonly Repo[]
}

/**
 * The sidebar chrome that used to live inside `SidebarTreePanel`, mounted
 * ONCE here rather than once per `SpaceScroller` panel — a second tray or
 * dialog per project would duplicate the one the user is actually looking
 * at. Carries over `RemovalTray`, `RenameDialog`, `RepoImportDialog`,
 * `SidebarRowContextMenu` and the "New Project" entry point verbatim; none
 * of their own logic changed, only where they mount.
 */
export function SidebarTreeChrome({ treeRef, rows, repos }: SidebarTreeChromeProps) {
  // The tree's only entry point for a SECOND project — carried over verbatim
  // from the old workspace-tree.tsx, which was the app's only "New Project"
  // surface once past the zero-project /oobe screen.
  const [importProjectOpen, setImportProjectOpen] = useState(false)
  const handleImportProject = useCallback((project: Project) => {
    importProjectAndSync(project)
    setImportProjectOpen(false)
  }, [])

  // Rename and branch-import both need a dialog rather than the row-context-menu's
  // direct-fire actions (lock/new-folder) — one row id (or none) at a time.
  const [renamingRowId, setRenamingRowId] = useState<string | null>(null)
  const [importRepoRowId, setImportRepoRowId] = useState<string | null>(null)
  const renamingLabel = rows.find((r) => r.id === renamingRowId)?.label ?? ''
  const importRepo = importRepoRowId != null ? resolveRow(repos, importRepoRowId)?.repo : undefined

  return (
    <>
      {/* "New Project", carried over verbatim from workspace-tree.tsx /
          sidebar-tree-panel.tsx. Deliberately outside the tree/row surface:
          it is an action, not a row with a place in any one project's
          hierarchy, so it renders once below every space rather than once
          per panel. */}
      <div className="px-1.5">
        <button
          type="button"
          className={cn(
            ROW_BASE,
            'mx-0 w-full border-transparent text-muted-foreground hover:bg-accent hover:text-foreground',
          )}
          onClick={() => setImportProjectOpen(true)}
        >
          <span className="inline-flex size-5 shrink-0 items-center justify-center">
            <Plus className="size-3.5" />
          </span>
          <span className="min-w-0 flex-1 truncate text-left">New Project</span>
        </button>
      </div>

      <RemovalTray />

      <ImportProjectModal
        open={importProjectOpen}
        onOpenChange={setImportProjectOpen}
        onImport={handleImportProject}
      />
      <SidebarRowContextMenu
        treeRef={treeRef}
        rows={rows}
        onRename={setRenamingRowId}
        onImport={setImportRepoRowId}
      />
      <RenameDialog
        open={renamingRowId != null}
        initialValue={renamingLabel}
        onOpenChange={(open) => {
          if (!open) setRenamingRowId(null)
        }}
        onConfirm={(name) => {
          if (renamingRowId) void performRenameRow(renamingRowId, name)
        }}
      />
      <RepoImportDialog
        projectId={importRepo?.projectId ?? ''}
        repoId={importRepo?.id ?? ''}
        defaultBranch={importRepo?.defaultBranch ?? ''}
        open={importRepoRowId != null}
        onOpenChange={(open) => {
          if (!open) setImportRepoRowId(null)
        }}
        onImport={(branches) => {
          if (importRepo) void performImportBranches(importRepo.id, branches)
        }}
      />
    </>
  )
}
