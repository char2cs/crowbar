import { useEffect, useState, type RefObject } from 'react'
import { SidebarRowContextMenu } from '@/components/sidebar/row-context-menu'
import { performImportBranches } from '@/components/sidebar/lib/row-actions'
import { useSidebarInlineRenameStore } from '@/lib/store/sidebar-inline-rename'
import { resolveRow } from './space-content-actions'
import type { Repo } from '@/lib/store/sidebar'
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
 * ONCE here rather than once per `SpaceScroller` panel — a second dialog
 * per project would duplicate the one the user is actually looking at.
 * Carries over `RepoImportDialog` and `SidebarRowContextMenu` verbatim;
 * none of their own logic changed, only where they mount. The "New Project"
 * entry point that used to live here too moved to a trailing `+` mark, now
 * in `SidebarFooter` (spec §4.1, relocated by task-10 of the
 * sidebar-restyle-recovery-batch2), with its modal state lifted to
 * `IDEShell` alongside it.
 *
 * `RemovalTray` no longer mounts here — a held row renders in place in the
 * tree now, and the tray (the commit clock) mounts ungated in the rail
 * itself, `ide-shell.tsx`. `RenameDialog` doesn't either any more:
 * the right-click menu's Rename item now starts the same inline editor
 * double-click does (`row-context-menu.tsx`), instead of a modal — this app
 * has exactly one rename gesture, not two.
 */
export function SidebarTreeChrome({ treeRef, rows, repos }: SidebarTreeChromeProps) {
  const [importRepoRowId, setImportRepoRowId] = useState<string | null>(null)
  const importRepo = importRepoRowId != null ? resolveRow(repos, importRepoRowId)?.repo : undefined

  // Double-click-to-rename, restored from the deleted tree's per-row inline
  // editors (git history: cf422bc5) — as a REAL inline `<input>` in place of
  // the row's label (sidebar-row.tsx), matching `develop`'s actual behavior.
  // This listener only starts the inline-rename store
  // (sidebar-inline-rename.ts); the row that reads `renamingRowId` off it and
  // actually draws the input lives several components away (SpaceScroller's
  // own tree), so the two can't share a prop chain — a store, not this
  // component's own state, is what lets both sides reach the same value. The
  // right-click menu's Rename item (`row-context-menu.tsx`) starts the exact
  // same store action, for the exact same reason.
  //
  // A native `dblclick` listener on the same `treeRef` ancestor
  // SidebarRowContextMenu's own `contextmenu` listener already uses, for the
  // same reason that component documents: reacting from state that lived
  // INSIDE the tree would re-render every row for an edit that only one of
  // them needs to draw. `[data-sidebar-row-label]` (sidebar-row.tsx) scopes
  // this to the label specifically — the trailing trash/create/fold controls
  // stop `click`/`pointerdown` from reaching the row, but not `dblclick`, so
  // without this a double-click on the trash icon would also start a rename.
  useEffect(() => {
    const tree = treeRef.current
    if (!tree) return
    const onDoubleClick = (e: MouseEvent) => {
      if (!(e.target instanceof HTMLElement)) return
      if (!e.target.closest('[data-sidebar-row-label]')) return
      const el = e.target.closest<HTMLElement>('[role="treeitem"]')
      const rowId = el?.getAttribute('data-sidebar-row-id')
      const row = rows.find((r) => r.id === rowId)
      if (!rowId || !row) return
      // Same refusal as the context menu's own Rename item: a locked branch's
      // rename write is silently dropped downstream (performRenameWorkspaceBranch),
      // so starting the inline editor here would look like it worked and didn't.
      if (row.kind === 'branch' && row.locked) return
      useSidebarInlineRenameStore.getState().startRenaming(rowId)
    }
    tree.addEventListener('dblclick', onDoubleClick)
    return () => tree.removeEventListener('dblclick', onDoubleClick)
  }, [treeRef, rows])

  return (
    <>
      <SidebarRowContextMenu treeRef={treeRef} rows={rows} onImport={setImportRepoRowId} />
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
