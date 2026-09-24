import type { PendingCreateEntry } from '@/lib/store/pending-creates'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

/**
 * One pending create, as the row it stands in for — same shape every real
 * row renders through (`SidebarRow`, sidebar-row.tsx), so no separate
 * placeholder component is needed: `row.pending` alone tells the renderer to
 * draw a naming input, a spinner, or an inline error in its place. `working`
 * doubles as the existing spinner-glyph signal (sidebar-row.tsx already
 * swaps to `FlickerSpinner` on it) — 'naming' stays false so the glyph is a
 * plain one behind the input, matching every other not-yet-working row.
 */
export function rowFromPending(entry: PendingCreateEntry): SidebarRow {
  return {
    id: entry.tempId,
    kind: entry.kind,
    parentId: entry.parentId,
    order: entry.order,
    label: entry.label,
    ownsWorktree: entry.ownsWorktree,
    workspaceId: entry.workspaceId,
    working: entry.status === 'creating',
    hasView: false,
    pending: { tempId: entry.tempId, status: entry.status, error: entry.error },
  }
}

export function rowsFromPending(entries: readonly PendingCreateEntry[]): SidebarRow[] {
  return entries.map(rowFromPending)
}

const EMPTY_REPO_SCOPE: ReadonlyMap<string, string> = new Map()

/**
 * `projectId`'s panel rows minus the real row of every create still in
 * flight, so the pending row is the ONE stand-in until its entry clears.
 * Once the POST has answered, `realId` names the row to hide; before that
 * (the daemon's `created` frame reseeds it in long before the runner or
 * worktree finish — see `PendingCreateEntry.rowIdsAtClick`) any row the
 * panel did not hold at click time is hidden — wherever it reseeded, since
 * a mint lands at root before its placement write.
 *
 * `rowRepoId` (a row id -> repo id lookup, `rows-from-repo.ts`'s
 * `rowRepoScope`) SCOPES that suppression for an entry carrying its own
 * `repoId` (a branch import — `row-actions.ts`'s `startImportPendingRows`):
 * such an entry only ever hides a row this map resolves to that SAME repo,
 * never a row anywhere else in the project. Without it, one repo's import
 * would blank out every other repo's (and project home's) freshly-created
 * rows for its whole provisioning window — a worse regression than the
 * ghost row this mechanism exists to hide, live-reported. An entry with no
 * `repoId` (every fork/thread create, `create-actions.ts`) is
 * unaffected: it keeps hiding project-wide exactly as before, since its own
 * real row can land anywhere in the panel before its placement write
 * corrects it (this function's own doc above). `rowRepoId` omitted, or a row
 * absent from it, means a repo-scoped entry suppresses nothing there rather
 * than risk hiding project-wide.
 */
export function hideRowsForInFlightCreates(
  rows: readonly SidebarRow[],
  entries: readonly PendingCreateEntry[],
  projectId: string,
  rowRepoId: ReadonlyMap<string, string> = EMPTY_REPO_SCOPE,
): readonly SidebarRow[] {
  const realIds = new Set<string>()
  const knownAtClick: { repoId: string | undefined; ids: ReadonlySet<string> }[] = []
  for (const e of entries) {
    if (e.projectId !== projectId) continue
    if (e.realId) realIds.add(e.realId)
    else if (e.status === 'creating' && e.rowIdsAtClick)
      knownAtClick.push({ repoId: e.repoId, ids: new Set(e.rowIdsAtClick) })
  }
  if (realIds.size === 0 && knownAtClick.length === 0) return rows
  return rows.filter((r) => {
    if (r.pending !== undefined) return true
    if (realIds.has(r.id)) return false
    return knownAtClick.every(
      (known) =>
        (known.repoId !== undefined && known.repoId !== rowRepoId.get(r.id)) || known.ids.has(r.id),
    )
  })
}
