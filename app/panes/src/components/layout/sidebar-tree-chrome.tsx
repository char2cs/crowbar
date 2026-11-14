import { useEffect, useState, type RefObject } from 'react'
import { SidebarRowContextMenu } from '@/components/sidebar/row-context-menu'
import { performImportBranches } from '@/components/sidebar/lib/import-branches'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import type { Repo } from '@/core/entities/repo-types'
import { RepoImportDialog } from './repo-import-dialog'
import { resolveRow } from './open-actions'
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

// Sidebar chrome mounted ONCE for every `SpaceScroller` panel (a dialog per project would
// duplicate the one on screen): the branch-import dialog, the row context menu, and the
// double-click rename listener.
export function SidebarTreeChrome({ treeRef, rows, repos }: SidebarTreeChromeProps) {
  const [importRepoRowId, setImportRepoRowId] = useState<string | null>(null)

  // Double-click starts the inline rename through the tree UI store (the row drawing the input is
  // far away; the context menu's Rename uses the same action). A native listener on the tree avoids
  // re-rendering rows; `[data-sidebar-row-label]` scopes it, since trailing controls pass dblclick.
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
      treeUiStore.getState().startRenaming(rowId)
    }
    tree.addEventListener('dblclick', onDoubleClick)
    return () => tree.removeEventListener('dblclick', onDoubleClick)
  }, [treeRef, rows])

  return (
    <>
      <SidebarRowContextMenu treeRef={treeRef} rows={rows} onImport={setImportRepoRowId} />
      <RowBranchImportDialog
        repoRowId={importRepoRowId}
        repos={repos}
        onClose={() => setImportRepoRowId(null)}
      />
    </>
  )
}

/** The import dialog for the project-home row `repoRowId` (null: closed), importing into its repo. */
export function RowBranchImportDialog({
  repoRowId,
  repos,
  onClose,
}: {
  repoRowId: string | null
  repos: readonly Repo[]
  onClose: () => void
}) {
  const repo = repoRowId != null ? resolveRow(repos, repoRowId)?.repo : undefined
  return (
    <RepoImportDialog
      projectId={repo?.projectId ?? ''}
      repoId={repo?.id ?? ''}
      defaultBranch={repo?.defaultBranch ?? ''}
      open={repoRowId != null}
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
      onImport={(branches, lockedBranches) => {
        if (repo) performImportBranches(repo.id, branches, lockedBranches)
      }}
    />
  )
}
