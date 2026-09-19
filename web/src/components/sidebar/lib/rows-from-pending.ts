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

/**
 * `projectId`'s panel rows minus the real row of every create still in
 * flight, so the pending row is the ONE stand-in until its entry clears.
 * Once the POST has answered, `realId` names the row to hide; before that
 * (the daemon's `created` frame reseeds it in long before the runner or
 * worktree finish — see `PendingCreateEntry.rowIdsAtClick`) any row the
 * panel did not hold at click time is hidden — wherever it reseeded, since
 * a mint lands at root before its placement write.
 */
export function hideRowsForInFlightCreates(
  rows: readonly SidebarRow[],
  entries: readonly PendingCreateEntry[],
  projectId: string,
): readonly SidebarRow[] {
  const realIds = new Set<string>()
  const knownAtClick: ReadonlySet<string>[] = []
  for (const e of entries) {
    if (e.projectId !== projectId) continue
    if (e.realId) realIds.add(e.realId)
    else if (e.status === 'creating' && e.rowIdsAtClick) knownAtClick.push(new Set(e.rowIdsAtClick))
  }
  if (realIds.size === 0 && knownAtClick.length === 0) return rows
  return rows.filter(
    (r) =>
      r.pending !== undefined ||
      (!realIds.has(r.id) && knownAtClick.every((known) => known.has(r.id))),
  )
}
