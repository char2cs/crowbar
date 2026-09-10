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
