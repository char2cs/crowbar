import { useEffect, useState, type RefObject } from 'react'
import { SidebarRowContextMenu } from '@/components/sidebar/row-context-menu'
import { RenameDialog } from '@/components/sidebar/rename-dialog'
import { performRenameRow, performImportBranches } from '@/components/sidebar/lib/row-actions'
import { resolveRow } from './space-content-actions'
import type { Repo } from '@/lib/store/sidebar'
import { RemovalTray } from './removal-tray'
import { RepoImportDialog } from './repo-import-dialog'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

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
 * at. Carries over `RemovalTray`, `RenameDialog`, `RepoImportDialog` and
 * `SidebarRowContextMenu` verbatim; none of their own logic changed, only
 * where they mount. The "New Project" entry point that used to live here
 * too moved to a trailing `+` mark in `SidebarProjectHeader` (spec §4.1),
 * with its modal state lifted to `IDEShell` alongside it.
 */
export function SidebarTreeChrome({ treeRef, rows, repos }: SidebarTreeChromeProps) {
  // Rename and branch-import both need a dialog rather than the row-context-menu's
  // direct-fire actions (lock/new-folder) — one row id (or none) at a time.
  const [renamingRowId, setRenamingRowId] = useState<string | null>(null)
  const [importRepoRowId, setImportRepoRowId] = useState<string | null>(null)
  const renamingLabel = rows.find((r) => r.id === renamingRowId)?.label ?? ''
  const importRepo = importRepoRowId != null ? resolveRow(repos, importRepoRowId)?.repo : undefined

  // Double-click-to-rename, restored from the deleted tree's per-row inline
  // editors (git history: cf422bc5). A native `dblclick` listener on the same
  // `treeRef` ancestor SidebarRowContextMenu's own `contextmenu` listener
  // already uses, for the same reason that component documents: opening the
  // dialog from state that lived INSIDE the tree would re-render every row to
  // draw a dialog that isn't part of the tree at all. `[data-sidebar-row-label]`
  // (sidebar-row.tsx) scopes this to the label specifically — the trailing
  // trash/create/fold controls stop `click`/`pointerdown` from reaching the
  // row, but not `dblclick`, so without this a double-click on the trash icon
  // would also open the rename dialog.
  useEffect(() => {
    const tree = treeRef.current
    if (!tree) return
    const onDoubleClick = (e: MouseEvent) => {
      if (!(e.target instanceof HTMLElement)) return
      if (!e.target.closest('[data-sidebar-row-label]')) return
      const el = e.target.closest<HTMLElement>('[role="treeitem"]')
      const rowId = el?.getAttribute('data-sidebar-row-id')
      if (!rowId || !rows.some((r) => r.id === rowId)) return
      setRenamingRowId(rowId)
    }
    tree.addEventListener('dblclick', onDoubleClick)
    return () => tree.removeEventListener('dblclick', onDoubleClick)
  }, [treeRef, rows])

  return (
    <>
      <RemovalTray />

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
        onImport={(branches, lockedBranches) => {
          if (importRepo) void performImportBranches(importRepo.id, branches, lockedBranches)
        }}
      />
    </>
  )
}
